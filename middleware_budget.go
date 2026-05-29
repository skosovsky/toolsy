package toolsy

import (
	"context"
	"fmt"
	"strings"
)

// BudgetTracker authorizes each physical tool execution attempt.
type BudgetTracker interface {
	Allow(ctx context.Context, manifest ToolManifest, input ToolInput) (allowed bool, reason string, err error)
}

// BudgetEnv binds a [BudgetTracker] for [WithBudget] via [BindEnv].
type BudgetEnv struct {
	Budget BudgetTracker
}

// WithBudget enforces optional budget checks via [BudgetEnv] on the execution context.
func WithBudget() Middleware {
	return func(next Tool) Tool {
		return &budgetTool{
			toolBase: toolBase{next: next},
		}
	}
}

type budgetTool struct {
	toolBase
}

func (t *budgetTool) Execute(
	ctx context.Context,
	run RunContext,
	input ToolInput,
	yield func(Chunk) error,
) error {
	var budgetTracker BudgetTracker
	if env, ok := EnvFromContext[BudgetEnv](ctx); ok && env.Budget != nil {
		budgetTracker = env.Budget
	}
	if budgetTracker == nil {
		return t.next.Execute(ctx, run, input, yield)
	}

	allowed, reason, err := budgetTracker.Allow(ctx, t.next.Manifest(), input)
	if err != nil {
		return &SystemError{Err: fmt.Errorf("toolsy: budget allow check failed: %w", err)}
	}
	if allowed {
		return t.next.Execute(ctx, run, input, yield)
	}

	msg := strings.TrimSpace(reason)
	if msg == "" {
		msg = "budget exceeded"
	}
	chunk := newErrorChunk(msg)
	if chunkErr := validateChunk(chunk); chunkErr != nil {
		return chunkErr
	}
	if yieldErr := yield(chunk); yieldErr != nil {
		return wrapYieldError(yieldErr)
	}
	return nil
}
