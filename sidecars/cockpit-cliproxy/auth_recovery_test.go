package main

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func TestAuthRecoverySharesFlightAndSurvivesCallerCancellation(t *testing.T) {
	requests := make(chan authRecoveryRequest, 100)
	c := newAuthRecoveryCoordinator(context.Background(), func(event any) {
		if request, ok := event.(authRecoveryRequest); ok {
			requests <- request
		}
	})
	var applied atomic.Int32
	apply := func(context.Context, authRecoveryReply) bool { applied.Add(1); return true }
	ctx, cancel := context.WithCancel(context.Background())
	leader := make(chan bool, 1)
	go func() { leader <- c.recover(ctx, authRecoveryRequest{AccountID: "a"}, apply) }()
	request := <-requests
	cancel()
	if <-leader {
		t.Fatal("cancelled caller succeeded")
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !c.recover(context.Background(), authRecoveryRequest{AccountID: "a"}, apply) {
				t.Error("shared recovery failed")
			}
		}()
	}
	c.acceptReply(authRecoveryReply{Type: "auth_recovery_result", AccountID: "a", RecoveryID: request.RecoveryID, Success: true})
	wg.Wait()
	if applied.Load() != 1 || len(requests) != 0 {
		t.Fatalf("duplicate refresh: applied=%d extra=%d", applied.Load(), len(requests))
	}
}

func TestAuthRecoveryTimeoutCorrelationBackoffAndHostExit(t *testing.T) {
	requests := make(chan authRecoveryRequest, 10)
	c := newAuthRecoveryCoordinator(context.Background(), func(event any) {
		if request, ok := event.(authRecoveryRequest); ok {
			requests <- request
		}
	})
	c.wait = 30 * time.Millisecond
	result := make(chan bool, 1)
	go func() {
		result <- c.recover(context.Background(), authRecoveryRequest{AccountID: "a"}, func(context.Context, authRecoveryReply) bool { return true })
	}()
	request := <-requests
	c.acceptReply(authRecoveryReply{Type: "auth_recovery_result", AccountID: "a", RecoveryID: "wrong", Success: true})
	c.acceptReply(authRecoveryReply{Type: "auth_recovery_result", AccountID: "b", RecoveryID: request.RecoveryID, Success: true})
	if <-result {
		t.Fatal("uncorrelated reply recovered account")
	}
	if c.recover(context.Background(), authRecoveryRequest{AccountID: "a"}, nil) || len(requests) != 0 {
		t.Fatal("failed refresh did not back off")
	}
	go func() { result <- c.recover(context.Background(), authRecoveryRequest{AccountID: "b"}, nil) }()
	<-requests
	c.readReplies(strings.NewReader(""))
	if <-result {
		t.Fatal("host exit reported recovery success")
	}
	if c.recover(context.Background(), authRecoveryRequest{AccountID: "c"}, nil) {
		t.Fatal("recovery allowed after EOF")
	}
}

type recoveryBearerExecutor struct {
	coreauth.ProviderExecutor
	calls int
}

func (*recoveryBearerExecutor) Identifier() string { return "codex" }
func (e *recoveryBearerExecutor) Execute(_ context.Context, auth *coreauth.Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	e.calls++
	return cliproxyexecutor.Response{Payload: []byte(auth.Metadata["access_token"].(string))}, nil
}

func recoveryTestManifest() (*manifest, *accountSpec, *coreauth.Auth) {
	account := &accountSpec{ID: "recover-account", AuthID: "recover-auth", AuthKind: "oauth", PlanType: "plus"}
	m := &manifest{Accounts: []accountSpec{*account}, accountByID: map[string]*accountSpec{account.ID: account}, accountByAuthID: map[string]*accountSpec{account.AuthID: account}}
	auth := &coreauth.Auth{ID: account.AuthID, Provider: "codex", Status: coreauth.StatusError, Unavailable: true,
		NextRetryAfter: time.Now().Add(time.Hour), LastError: &coreauth.Error{HTTPStatus: 401},
		Metadata: map[string]any{"access_token": "old", "refresh_owner": "cockpit_token_authority", "cockpit_token_generation": 1}}
	return m, account, auth
}

func TestAuthRecoveryProductionSelectorRefreshesBeforeDispatch(t *testing.T) {
	m, _, auth := recoveryTestManifest()
	manager := buildCoreAuthManager(&config.Config{}, &cockpitSelector{manifest: m}, nil, m, nil, newRequestUsageTracker())
	manager.SetStore(nil)
	executor := &recoveryBearerExecutor{}
	manager.RegisterExecutor(executor)
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatal(err)
	}
	reg := registry.GetGlobalRegistry()
	reg.RegisterClient(auth.ID, "codex", []*registry.ModelInfo{{ID: "gpt-5.5"}})
	t.Cleanup(func() { reg.UnregisterClient(auth.ID) })
	var requests atomic.Int32
	m.authRecovery = newAuthRecoveryCoordinator(context.Background(), func(event any) {
		if request, ok := event.(authRecoveryRequest); ok {
			requests.Add(1)
			if request.Reason != "credentials" || request.ObservedGeneration != 1 {
				t.Error("missing credential refresh reason/generation")
			}
			m.authRecovery.acceptReply(authRecoveryReply{Type: "auth_recovery_result", AccountID: request.AccountID, RecoveryID: request.RecoveryID, Success: true,
				Credentials: map[string]any{"access_token": "fresh", "cockpit_token_generation": 2}})
		}
	})
	response, err := manager.Execute(context.Background(), []string{"codex"}, cliproxyexecutor.Request{Model: "gpt-5.5"}, cliproxyexecutor.Options{})
	if err != nil || string(response.Payload) != "fresh" || executor.calls != 1 || requests.Load() != 1 {
		t.Fatalf("response=%q err=%v dispatches=%d refreshes=%d", response.Payload, err, executor.calls, requests.Load())
	}
}

func TestAuthRecoveryRespectsPoolPolicies(t *testing.T) {
	for _, policy := range []string{"allowed", "disabled", "quota", "scope", "image", "access_token", "model", "reserve"} {
		t.Run(policy, func(t *testing.T) {
			m, account, auth := recoveryTestManifest()
			ctx := context.Background()
			switch policy {
			case "disabled":
				auth.Disabled = true
			case "quota":
				account.QuotaCooldown = &quotaCooldownState{Exhausted: true}
			case "scope":
				ctx = context.WithValue(ctx, clientAPIKeyContextKey, &apiKeySpec{AccountIDs: []string{"another-account"}})
			case "image":
				account.ImageGenerationPolicy = "disabled"
				ctx = context.WithValue(ctx, requestKindContextKey, "image_generation")
			case "access_token":
				account.AccessTokenOnly = true
			case "model":
				auth.Metadata["excluded_models"] = []any{"gpt-5.5"}
			case "reserve":
				threshold, remaining, timestamp := 20, 10, time.Now().Unix()
				account.QuotaReserve = &quotaReserveSpec{HourlyThresholdPercent: &threshold, HourlyRemainingPercent: &remaining, SnapshotUpdatedAtUnixSeconds: &timestamp}
			}
			selector := &authRecoverySelector{manifest: m}
			if got := selector.recoverable(ctx, auth, "gpt-5.5"); got != (policy == "allowed") {
				t.Fatalf("recoverable=%v", got)
			}
		})
	}
}
