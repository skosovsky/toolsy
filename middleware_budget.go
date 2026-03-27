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

// WithBudget enforces optional budget checks via run.Services.Get("budget").
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
	if run.Services == nil {
		return t.next.Execute(ctx, run, input, yield)
	}

	rawBudget, ok := run.Services.Get("budget")
	if !ok || rawBudget == nil {
		return t.next.Execute(ctx, run, input, yield)
	}

	budgetTracker, ok := rawBudget.(BudgetTracker)
	if !ok {
		return &SystemError{Err: fmt.Errorf("toolsy: budget service has unexpected type %T", rawBudget)}
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
