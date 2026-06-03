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

// WithBudget enforces optional budget checks via [DepKeyBudget] on [RunEnv].
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
	env *RunEnv,
	input ToolInput,
	yield func(Chunk) error,
) error {
	budgetTracker, ok := Lookup[BudgetTracker](env, DepKeyBudget)
	if !ok || budgetTracker == nil {
		return t.next.Execute(ctx, env, input, yield)
	}

	allowed, reason, err := budgetTracker.Allow(ctx, t.next.Manifest(), input)
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
	chunk := newErrorChunk(msg)
	if chunkErr := validateChunk(chunk); chunkErr != nil {
		return chunkErr
	}
	if yieldErr := yield(chunk); yieldErr != nil {
		return wrapYieldError(yieldErr)
	}
	return nil
}
