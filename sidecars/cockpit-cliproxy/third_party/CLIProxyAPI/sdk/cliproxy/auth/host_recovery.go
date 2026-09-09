package auth

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CanRecoverAuthState excludes explicit policy failures, real quota exhaustion
// and unexpired server cooldowns. A confirmed credential refresh may repair 401.
func CanRecoverAuthState(auth *Auth, now time.Time) bool {
	if auth == nil || auth.Disabled || auth.Status == StatusDisabled ||
		!recoverableState(auth.Status, auth.Quota, auth.NextRetryAfter, auth.LastError, now) {
		return false
	}
	for _, state := range auth.ModelStates {
		if state != nil && !recoverableState(state.Status, state.Quota, state.NextRetryAfter, state.LastError, now) {
			return false
		}
	}
	return true
}

func recoverableState(status Status, quota QuotaState, retry time.Time, last *Error, now time.Time) bool {
	if status == StatusDisabled || (quota.Exceeded && (quota.NextRecoverAt.IsZero() || quota.NextRecoverAt.After(now))) || quota.NextRecoverAt.After(now) {
		return false
	}
	unauthorized := last != nil && last.HTTPStatus == 401
	if last != nil {
		if last.HTTPStatus == 0 && !last.Retryable {
			return false
		}
		if last.HTTPStatus != 0 && last.HTTPStatus != 401 && last.HTTPStatus != 408 && last.HTTPStatus < 500 {
			return false
		}
		for _, permanent := range []string{"invalid_grant", "refresh_token_reused", "account_deactivated", "reauth", "permission_denied"} {
			if strings.Contains(strings.ToLower(last.Code+" "+last.Message), permanent) {
				return false
			}
		}
	}
	return !retry.After(now) || unauthorized
}

// RecoverAuthState applies a host-acknowledged bearer snapshot and clears only
// recoverable state. The generation check prevents a concurrent 429, disable or
// credential update from being lost while the host was refreshing.
func (m *Manager) RecoverAuthState(ctx context.Context, expected *Auth, rejectedAccessToken string, credentials map[string]any) (*Auth, error) {
	if expected == nil {
		return nil, nil
	}
	if token, ok := credentials["access_token"].(string); !ok || strings.TrimSpace(token) == "" {
		return nil, nil
	}
	updated, _, err := m.resetQuota(withSkipPersistWithoutWatermark(ctx), expected.ID, func(current *Auth) bool {
		if current.Generation != expected.Generation || current.RegistrationEpoch != expected.RegistrationEpoch || !CanRecoverAuthState(current, time.Now()) {
			return false
		}
		if current.Metadata["refresh_owner"] != "cockpit_token_authority" {
			return false
		}
		candidate := current.Clone()
		for _, key := range []string{"access_token", "id_token", "expired", "last_refresh", "cockpit_token_generation"} {
			if value, ok := credentials[key]; ok {
				candidate.Metadata[key] = value
			}
		}
		if !candidate.HasValidAccessToken(time.Now()) || hostTokenGeneration(candidate) < hostTokenGeneration(current) {
			return false
		}
		// Reusing the rejected bearer cannot establish that a 401 was repaired.
		if authHasUnauthorizedFailure(current) && candidate.Metadata["access_token"] == rejectedAccessToken {
			return false
		}
		candidate.Metadata["refresh_token"] = ""
		current.Metadata = candidate.Metadata
		current.LastRefreshedAt = time.Now()
		current.NextRefreshAfter = time.Time{}
		if m.hostAuthRecoveryBarriers == nil {
			m.hostAuthRecoveryBarriers = make(map[string]time.Time)
		}
		m.hostAuthRecoveryBarriers[current.ID] = time.Now()
		return true
	})
	if err == nil && updated != nil {
		// The host already saved this bearer. Advance the persistence watermark
		// outside the manager lock so queued older snapshots cannot overwrite it.
		err = m.persist(WithSkipPersist(ctx), updated)
	}
	return updated, err
}

func hostTokenGeneration(auth *Auth) uint64 {
	var generation uint64
	if auth != nil {
		_, _ = fmt.Sscan(fmt.Sprint(auth.Metadata["cockpit_token_generation"]), &generation)
	}
	return generation
}

func authHasUnauthorizedFailure(auth *Auth) bool {
	if auth.LastError != nil && auth.LastError.HTTPStatus == 401 {
		return true
	}
	for _, state := range auth.ModelStates {
		if state != nil && state.LastError != nil && state.LastError.HTTPStatus == 401 {
			return true
		}
	}
	return false
}
