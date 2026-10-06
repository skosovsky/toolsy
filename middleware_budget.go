package toolsy

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// BudgetTracker admits each wrapped call, including calls that later replay.
// Input is a raw-call snapshot before binding; host owns pricing and accounting.
type BudgetTracker interface {
	Allow(ctx context.Context, manifest ToolManifest, input ToolInput) (allowed bool, reason string, err error)
}

// ErrBudgetConfiguration identifies a missing or malformed required budget gate.
var ErrBudgetConfiguration = errors.New("toolsy: invalid budget configuration")

// WithBudget requires a valid [BudgetTracker] at [DepKeyBudget] on [RunEnv].
func WithBudget() Middleware {
	return budgetMiddleware(false)
}

// WithOptionalBudget bypasses only an absent dependency. A supplied invalid
// tracker still fails closed. Use only for an intentional host no-budget mode.
func WithOptionalBudget() Middleware { return budgetMiddleware(true) }

func budgetMiddleware(optional bool) Middleware {
	return func(next Tool) Tool { return &budgetTool{next: next, optional: optional} }
}

type budgetTool struct {
	toolBase

	optional bool
}

func (t *budgetTool) Execute(
	ctx context.Context,
	env *RunEnv,
	input ToolInput,
	yield func(Chunk) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	budgetTracker, present, lookupErr := lookupBudgetTracker(env)
	if lookupErr != nil {
		return NewInternalError(lookupErr)
	}
	if !present {
		if t.optional {
			return t.next.Execute(ctx, env, input, yield)
		}
		return NewInternalError(fmt.Errorf("%w: tracker is missing", ErrBudgetConfiguration))
	}
	allowed, reason, err := budgetTracker.Allow(ctx, cloneManifestForPolicy(t.next.Manifest()), input.Clone())
	if canceled := ctx.Err(); canceled != nil {
		return canceled
	}
	if err != nil {
		return NewInternalError(fmt.Errorf("toolsy: budget allow check failed: %w", err))
	}
	if allowed {
		return t.next.Execute(ctx, env, input, yield)
	}

	msg := strings.TrimSpace(reason)
	if msg == "" {
		msg = "budget exceeded"
	}
	chunk := NewErrorChunkFromErr(NewBudgetExceededError(msg))
	prepared, chunkErr := prepareChunk(chunk)
	if chunkErr != nil {
		return chunkErr
	}
	if yieldErr := yield(prepared); yieldErr != nil {
		return wrapYieldError(yieldErr)
	}
	return nil
}

// Capture only under the store lock; host callback runs outside it. A nil value
// deliberately supplied under the key is different from an absent dependency.
func lookupBudgetTracker(env *RunEnv) (BudgetTracker, bool, error) {
	if env == nil || env.store == nil {
		return nil, false, nil
	}
	env.store.mu.RLock()
	raw, present := env.store.deps[DepKeyBudget]
	env.store.mu.RUnlock()
	if !present {
		return nil, false, nil
	}
	tracker, ok := raw.(BudgetTracker)
	if !ok || isNilValue(tracker) {
		return nil, true, fmt.Errorf("%w: tracker has invalid type or nil value", ErrBudgetConfiguration)
	}
	return tracker, true, nil
}
