package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

type authRecoveryRequest struct {
	Type               string `json:"type"`
	RecoveryID         string `json:"recoveryId"`
	AccountID          string `json:"accountId"`
	Reason             string `json:"reason"`
	ObservedGeneration uint64 `json:"observedGeneration"`
}

// Credentials travel only on the private parent-to-child stdin pipe, never in
// HTTP responses, stdout diagnostics or a public recovery endpoint.
type authRecoveryReply struct {
	Type        string         `json:"type"`
	RecoveryID  string         `json:"recoveryId"`
	AccountID   string         `json:"accountId"`
	Success     bool           `json:"success"`
	ErrorCode   string         `json:"errorCode,omitempty"`
	Credentials map[string]any `json:"credentials"`
}

type authRecoveryFlight struct {
	done    chan struct{}
	reply   chan authRecoveryReply
	request authRecoveryRequest
	success bool
}

type authRecoveryRetry struct {
	after    time.Time
	failures int
}

type authRecoveryCoordinator struct {
	ctx         context.Context
	emit        func(any)
	mu          sync.Mutex
	flights     map[string]*authRecoveryFlight
	retries     map[string]authRecoveryRetry
	hostDone    chan struct{}
	hostClosed  sync.Once
	wait        time.Duration
	minInterval time.Duration
}

func newAuthRecoveryCoordinator(ctx context.Context, emit func(any)) *authRecoveryCoordinator {
	return &authRecoveryCoordinator{
		ctx: ctx, emit: emit, flights: make(map[string]*authRecoveryFlight),
		retries: make(map[string]authRecoveryRetry), hostDone: make(chan struct{}),
		wait: 10 * time.Second, minInterval: time.Minute,
	}
}

func (c *authRecoveryCoordinator) readReplies(reader io.Reader) {
	defer c.hostClosed.Do(func() { close(c.hostDone) })
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 256*1024)
	for scanner.Scan() {
		var reply authRecoveryReply
		if json.Unmarshal(scanner.Bytes(), &reply) == nil {
			c.acceptReply(reply)
		}
	}
}

func (c *authRecoveryCoordinator) acceptReply(reply authRecoveryReply) {
	if reply.Type != "auth_recovery_result" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	flight := c.flights[reply.AccountID]
	if flight == nil || reply.RecoveryID != flight.request.RecoveryID {
		return
	}
	select {
	case flight.reply <- reply:
	default:
	}
}

func (c *authRecoveryCoordinator) recover(ctx context.Context, request authRecoveryRequest, apply func(context.Context, authRecoveryReply) bool) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case <-c.hostDone:
		return false
	case <-c.ctx.Done():
		return false
	default:
	}
	c.mu.Lock()
	flight := c.flights[request.AccountID]
	if flight == nil {
		if retry := c.retries[request.AccountID]; time.Now().Before(retry.after) {
			c.mu.Unlock()
			// A caller may hold a failed snapshot from just before another
			// flight completed. Reselect once without refreshing again.
			return retry.failures == 0
		}
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			c.mu.Unlock()
			return false
		}
		request.Type, request.RecoveryID = "auth_recovery_required", hex.EncodeToString(nonce[:])
		flight = &authRecoveryFlight{done: make(chan struct{}), reply: make(chan authRecoveryReply, 1), request: request}
		c.flights[request.AccountID] = flight
		// The flight belongs to the service, so cancelling one caller does not
		// cancel the refresh that other requests are waiting for.
		go c.run(flight, apply)
	}
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return false
	case <-flight.done:
		return flight.success
	}
}

func (c *authRecoveryCoordinator) run(flight *authRecoveryFlight, apply func(context.Context, authRecoveryReply) bool) {
	ctx, cancel := context.WithTimeout(c.ctx, c.wait)
	defer cancel()
	c.emit(flight.request)
	errorCode := ""
	select {
	case reply := <-flight.reply:
		flight.success = reply.Success && ctx.Err() == nil && apply(ctx, reply)
		if !flight.success {
			errorCode = reply.ErrorCode
			if errorCode == "" {
				errorCode = "state_changed_or_credentials_unconfirmed"
			}
		}
	case <-ctx.Done():
		errorCode = "recovery_timeout"
	case <-c.hostDone:
		errorCode = "host_unavailable"
	}
	c.mu.Lock()
	retry := c.retries[flight.request.AccountID]
	if flight.success {
		retry.failures = 0
	} else if retry.failures < 4 {
		retry.failures++
	}
	delay := c.minInterval
	if retry.failures > 1 {
		delay *= time.Duration(1 << (retry.failures - 1))
	}
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	retry.after = time.Now().Add(delay)
	c.retries[flight.request.AccountID] = retry
	delete(c.flights, flight.request.AccountID)
	close(flight.done)
	c.mu.Unlock()
	c.emit(map[string]any{"type": "auth_recovery", "accountId": flight.request.AccountID,
		"reason": flight.request.Reason, "success": flight.success, "errorCode": errorCode, "retryAfterMs": delay.Milliseconds()})
}

// This is the outermost selector so failures in the manager's availability
// pass and in any Cockpit policy selector share the same recovery entry point.
type authRecoverySelector struct {
	fallback coreauth.Selector
	manifest *manifest
	quota    *quotaReserveStateStore
}

func (s *authRecoverySelector) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*coreauth.Auth) (*coreauth.Auth, error) {
	return s.fallback.Pick(ctx, provider, model, opts, auths)
}

func (s *authRecoverySelector) Stop() {
	if stoppable, ok := s.fallback.(coreauth.StoppableSelector); ok {
		stoppable.Stop()
	}
}

func (s *authRecoverySelector) ReportAuthSelectionFailure(ctx context.Context, provider, model string, auths []*coreauth.Auth, err error) error {
	if reporter, ok := s.fallback.(coreauth.AuthSelectionFailureReporter); ok {
		return reporter.ReportAuthSelectionFailure(ctx, provider, model, auths, err)
	}
	return err
}

func (s *authRecoverySelector) recoverable(ctx context.Context, auth *coreauth.Auth, model string) bool {
	if !coreauth.CanRecoverAuthState(auth, time.Now()) || auth.Metadata["refresh_owner"] != "cockpit_token_authority" {
		return false
	}
	account := accountForAuthInManifest(s.manifest, auth)
	if account == nil || account.AuthKind != "oauth" || account.AccessTokenOnly || accountQuotaExhausted(s.manifest, account, time.Now()) || authModelExcluded(s.manifest, auth, model) {
		return false
	}
	requestKind, _ := ctx.Value(requestKindContextKey).(string)
	if isImageRequestKind(requestKind) && !imageGenerationAllowedForAccount(account) {
		return false
	}
	if quotaReserveBlockReasonWithState(account, s.quota, time.Now()) != "" {
		return false
	}
	selector := &cockpitSelector{manifest: s.manifest}
	return len(selector.filterAuthsForAPIKeyScope(ctx, []*coreauth.Auth{auth})) == 1
}

func (s *authRecoverySelector) RecoverAuthSelectionFailure(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*coreauth.Auth) bool {
	m := s.manifest
	if m == nil || m.authManager == nil || m.authRecovery == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, m.authRecovery.wait)
	defer cancel()
	for _, candidate := range auths {
		if !s.recoverable(ctx, candidate, model) {
			continue
		}
		account := accountForAuthInManifest(m, candidate)
		request := authRecoveryRequest{AccountID: account.ID, Reason: "transient"}
		_, _ = fmt.Sscan(fmt.Sprint(candidate.Metadata["cockpit_token_generation"]), &request.ObservedGeneration)
		if !candidate.HasValidAccessToken(time.Now()) || (candidate.LastError != nil && candidate.LastError.HTTPStatus == 401) {
			request.Reason = "credentials"
		}
		for _, state := range candidate.ModelStates {
			if state != nil && state.LastError != nil && state.LastError.HTTPStatus == 401 {
				request.Reason = "credentials"
			}
		}
		if m.authRecovery.recover(ctx, request, func(recoveryCtx context.Context, reply authRecoveryReply) bool {
			current, ok := m.authManager.GetByID(candidate.ID)
			if !ok || current.RegistrationEpoch != candidate.RegistrationEpoch || !s.recoverable(ctx, current, model) {
				return false
			}
			updated, err := m.authManager.RecoverAuthState(recoveryCtx, current, fmt.Sprint(candidate.Metadata["access_token"]), reply.Credentials)
			return err == nil && updated != nil
		}) {
			return true
		}
		if ctx.Err() != nil {
			return false
		}
	}
	return false
}
