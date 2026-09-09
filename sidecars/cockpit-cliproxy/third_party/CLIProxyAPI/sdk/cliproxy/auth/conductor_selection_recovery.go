package auth

import (
	"context"
	"errors"
	"sync/atomic"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

// AuthSelectionFailureRecoverer may repair host-owned credentials before the
// first auth is selected. It runs without the manager lock; candidates are
// snapshots. A successful recovery causes one new selection through all policy
// checks, never a replay of an upstream request or an existing stream.
type AuthSelectionFailureRecoverer interface {
	RecoverAuthSelectionFailure(context.Context, string, string, cliproxyexecutor.Options, []*Auth) bool
}

type selectionRecoveryContextKey struct{}

type selectionRecoveryState struct {
	attempted atomic.Bool
	selected  atomic.Bool
}

func withSelectionRecovery(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(selectionRecoveryContextKey{}).(*selectionRecoveryState); ok {
		return ctx
	}
	return context.WithValue(ctx, selectionRecoveryContextKey{}, &selectionRecoveryState{})
}

func markSelectionRecoveryDispatched(ctx context.Context) {
	if state, ok := ctx.Value(selectionRecoveryContextKey{}).(*selectionRecoveryState); ok {
		state.selected.Store(true)
	}
}

func tryRecoverAuthSelection(ctx context.Context, selector Selector, provider, model string, opts cliproxyexecutor.Options, candidates []*Auth, tried map[string]struct{}, err error) bool {
	if ctx.Err() != nil || len(tried) != 0 || len(candidates) == 0 {
		return false
	}
	var authErr *Error
	var cooldownErr *modelCooldownError
	selectionFailed := errors.As(err, &authErr) && authErr != nil && (authErr.Code == "auth_unavailable" || authErr.Code == "auth_not_found")
	if !selectionFailed && !errors.As(err, &cooldownErr) {
		return false
	}
	recoverer, ok := selector.(AuthSelectionFailureRecoverer)
	if !ok {
		return false
	}
	state, ok := ctx.Value(selectionRecoveryContextKey{}).(*selectionRecoveryState)
	if !ok || state.selected.Load() || !state.attempted.CompareAndSwap(false, true) {
		return false
	}
	return recoverer.RecoverAuthSelectionFailure(ctx, provider, model, opts, candidates) && ctx.Err() == nil
}
