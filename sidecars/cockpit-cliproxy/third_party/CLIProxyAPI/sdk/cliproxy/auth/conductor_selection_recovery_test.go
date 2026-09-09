package auth

import (
	"context"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

type recoveryTestSelector struct {
	Selector
	calls   int
	recover func([]*Auth) bool
}

func (s *recoveryTestSelector) RecoverAuthSelectionFailure(_ context.Context, _, _ string, _ cliproxyexecutor.Options, candidates []*Auth) bool {
	s.calls++
	return s.recover(candidates)
}

func TestSelectionRecoveryBeforeSelectorRuns(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "mixed"}[mixed], func(t *testing.T) {
			selector := &recoveryTestSelector{Selector: &RoundRobinSelector{}}
			manager := NewManager(nil, selector, nil)
			manager.RegisterExecutor(schedulerTestExecutor{provider: "codex"})
			auth, err := manager.Register(context.Background(), &Auth{
				ID: "recovery-test", Provider: "codex", Status: StatusError,
				Unavailable: true, NextRetryAfter: time.Now().Add(time.Hour),
				LastError: &Error{HTTPStatus: 401, Message: "expired bearer"},
				Metadata:  map[string]any{"access_token": "old", "refresh_owner": "cockpit_token_authority", "cockpit_token_generation": 1},
			})
			if err != nil {
				t.Fatal(err)
			}
			registerSchedulerModels(t, "codex", "recovery-model", auth.ID)
			selector.recover = func(candidates []*Auth) bool {
				if len(candidates) != 1 {
					t.Fatalf("candidates = %d", len(candidates))
				}
				updated, err := manager.RecoverAuthState(context.Background(), candidates[0], "old", map[string]any{"access_token": "new", "cockpit_token_generation": 2})
				if err != nil {
					t.Fatal(err)
				}
				return updated != nil
			}
			if mixed {
				_, err = manager.Execute(context.Background(), []string{"codex"}, cliproxyexecutor.Request{Model: "recovery-model"}, cliproxyexecutor.Options{})
			} else {
				var selected *Auth
				selected, err = manager.SelectAuth(context.Background(), "codex", "recovery-model", cliproxyexecutor.Options{})
				if err == nil && selected.Metadata["access_token"] != "new" {
					t.Fatal("selected stale bearer")
				}
			}
			if err != nil || selector.calls != 1 {
				t.Fatalf("err=%v recovery calls=%d", err, selector.calls)
			}
		})
	}
}

func TestSelectionRecoveryBudgetAndCancellation(t *testing.T) {
	errUnavailable := &Error{Code: "auth_unavailable"}
	auths := []*Auth{{ID: "one"}}
	for _, mode := range []string{"once", "dispatched", "tried", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			selector := &recoveryTestSelector{recover: func([]*Auth) bool { return true }}
			ctx, cancel := context.WithCancel(withSelectionRecovery(context.Background()))
			defer cancel()
			var tried map[string]struct{}
			switch mode {
			case "dispatched":
				markSelectionRecoveryDispatched(ctx)
			case "tried":
				tried = map[string]struct{}{"one": {}}
			case "cancelled":
				cancel()
			}
			for i := 0; i < 2; i++ {
				tryRecoverAuthSelection(ctx, selector, "codex", "m", cliproxyexecutor.Options{}, auths, tried, errUnavailable)
			}
			want := 0
			if mode == "once" {
				want = 1
			}
			if selector.calls != want {
				t.Fatalf("calls=%d want=%d", selector.calls, want)
			}
		})
	}
}

func TestHostRecoveryPreservesConcurrentStateAndRestrictions(t *testing.T) {
	for _, state := range []string{"quota", "model_quota", "disabled", "model_disabled", "forbidden", "rate_limit", "future_transient", "reauth", "generation", "same_token", "stale_token_generation"} {
		t.Run(state, func(t *testing.T) {
			manager := NewManager(nil, nil, nil)
			auth, err := manager.Register(context.Background(), &Auth{ID: "guard", Provider: "codex", Status: StatusActive,
				Metadata: map[string]any{"access_token": "old", "refresh_owner": "cockpit_token_authority", "cockpit_token_generation": 2}})
			if err != nil {
				t.Fatal(err)
			}
			current := auth.Clone()
			switch state {
			case "quota":
				current.Quota.Exceeded = true
			case "model_quota":
				current.ModelStates = map[string]*ModelState{"another-model": {Quota: QuotaState{Exceeded: true}}}
			case "disabled":
				current.Disabled = true
			case "model_disabled":
				current.ModelStates = map[string]*ModelState{"another-model": {Status: StatusDisabled}}
			case "forbidden":
				current.LastError = &Error{HTTPStatus: 403}
			case "rate_limit":
				current.LastError = &Error{HTTPStatus: 429}
			case "future_transient":
				current.NextRetryAfter = time.Now().Add(time.Hour)
			case "reauth":
				current.LastError = &Error{HTTPStatus: 401, Code: "invalid_grant"}
			case "same_token":
				current.LastError = &Error{HTTPStatus: 401}
			}
			current, err = manager.Update(context.Background(), current)
			if err != nil {
				t.Fatal(err)
			}
			expected := current
			if state == "generation" {
				expected = auth
			}
			credentials := map[string]any{"access_token": "new", "cockpit_token_generation": 3}
			if state == "same_token" {
				credentials["access_token"] = "old"
			}
			if state == "stale_token_generation" {
				credentials["cockpit_token_generation"] = 1
			}
			updated, err := manager.RecoverAuthState(context.Background(), expected, "old", credentials)
			if err != nil || updated != nil {
				t.Fatalf("restriction lost: updated=%v err=%v", updated != nil, err)
			}
			latest, _ := manager.GetByID(auth.ID)
			if latest.Metadata["access_token"] != "old" {
				t.Fatal("rejected recovery changed credentials")
			}
		})
	}
}

func TestHostRecoveryAllowsWatcherRefreshAndIgnoresOnlyLate401(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	auth, err := manager.Register(context.Background(), &Auth{ID: "watcher-recovery", Provider: "codex", Unavailable: true,
		LastError: &Error{HTTPStatus: 401}, Metadata: map[string]any{"access_token": "new-from-watcher", "refresh_owner": "cockpit_token_authority", "cockpit_token_generation": 2}})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Second)
	updated, err := manager.RecoverAuthState(context.Background(), auth, "old-rejected", map[string]any{"access_token": "new-from-watcher", "cockpit_token_generation": 2})
	if err != nil || updated == nil {
		t.Fatalf("watcher recovery failed: %v", err)
	}
	manager.MarkResult(context.Background(), Result{AuthID: auth.ID, AttemptStartedAt: started, Error: &Error{HTTPStatus: 401}})
	current, _ := manager.GetByID(auth.ID)
	if current.Unavailable || current.LastError != nil {
		t.Fatal("late 401 poisoned refreshed bearer")
	}
	manager.MarkResult(context.Background(), Result{AuthID: auth.ID, AttemptStartedAt: started, Error: &Error{HTTPStatus: 429}})
	current, _ = manager.GetByID(auth.ID)
	if !current.Quota.Exceeded {
		t.Fatal("late quota result was discarded")
	}
}

func TestHostRecoveryDoesNotWriteBackOrPersistStaleBearer(t *testing.T) {
	store := &countingStore{}
	manager := NewManager(store, nil, nil)
	auth, err := manager.Register(WithSkipPersist(context.Background()), &Auth{ID: "host-owned", Provider: "codex",
		Metadata: map[string]any{"access_token": "old", "refresh_owner": "cockpit_token_authority", "cockpit_token_generation": 1}})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := manager.RecoverAuthState(context.Background(), auth, "old", map[string]any{"access_token": "new", "cockpit_token_generation": 2})
	if err != nil || updated == nil {
		t.Fatalf("recovery failed: %v", err)
	}
	if err := manager.persist(context.Background(), auth); err != nil {
		t.Fatal(err)
	}
	if store.saveCount.Load() != 0 {
		t.Fatal("host bearer was overwritten by sidecar persistence")
	}
}

type recoveryStreamExecutor struct {
	schedulerTestExecutor
	calls int
}

func (e *recoveryStreamExecutor) ExecuteStream(_ context.Context, auth *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	e.calls++
	chunks := make(chan cliproxyexecutor.StreamChunk, 2)
	chunks <- cliproxyexecutor.StreamChunk{Payload: []byte("data: " + auth.Metadata["access_token"].(string) + "\n\n")}
	chunks <- cliproxyexecutor.StreamChunk{Err: &Error{HTTPStatus: 401, Message: "stream failed"}}
	close(chunks)
	return &cliproxyexecutor.StreamResult{Chunks: chunks}, nil
}

func TestSelectionRecoveryStreamDoesNotReplayAfterFirstChunk(t *testing.T) {
	selector := &recoveryTestSelector{Selector: &RoundRobinSelector{}}
	manager := NewManager(nil, selector, nil)
	executor := &recoveryStreamExecutor{schedulerTestExecutor: schedulerTestExecutor{provider: "codex"}}
	manager.RegisterExecutor(executor)
	auth, err := manager.Register(context.Background(), &Auth{ID: "recovery-stream", Provider: "codex", Unavailable: true,
		NextRetryAfter: time.Now().Add(time.Hour), LastError: &Error{HTTPStatus: 401},
		Metadata: map[string]any{"access_token": "old", "refresh_owner": "cockpit_token_authority", "cockpit_token_generation": 1}})
	if err != nil {
		t.Fatal(err)
	}
	registerSchedulerModels(t, "codex", "stream-model", auth.ID)
	selector.recover = func(candidates []*Auth) bool {
		updated, err := manager.RecoverAuthState(context.Background(), candidates[0], "old", map[string]any{"access_token": "fresh", "cockpit_token_generation": 2})
		return err == nil && updated != nil
	}
	result, err := manager.ExecuteStream(context.Background(), []string{"codex"}, cliproxyexecutor.Request{Model: "stream-model"}, cliproxyexecutor.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var payload []byte
	failed := false
	for chunk := range result.Chunks {
		payload = append(payload, chunk.Payload...)
		failed = failed || chunk.Err != nil
	}
	if string(payload) != "data: fresh\n\n" || !failed || executor.calls != 1 || selector.calls != 1 {
		t.Fatalf("payload=%q failed=%v executions=%d recoveries=%d", payload, failed, executor.calls, selector.calls)
	}
}
