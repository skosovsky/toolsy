package toolsy

import (
	"context"
	"sync"
	"sync/atomic"
)

// execCounterCtxKey is the context key for execution step/retry counting.
type execCounterCtxKey struct{}

type execCounter struct {
	count    atomic.Int64
	retries  atomic.Int64
	mu       sync.Mutex
	lastTool string // name of the last executed tool
}

// WithExecutionCounter attaches a shared execution counter to ctx.
// Use the returned context across related tool executions to enforce Registry WithMaxSteps and WithMaxRetries.
func WithExecutionCounter(ctx context.Context) context.Context {
	var c execCounter
	return context.WithValue(ctx, execCounterCtxKey{}, &c)
}

// ExecutionCount returns the number of executions recorded in ctx by Registry.Execute.
func ExecutionCount(ctx context.Context) int64 {
	if c, ok := ctx.Value(execCounterCtxKey{}).(*execCounter); ok {
		return c.count.Load()
	}
	return 0
}

// RetriesCount returns the number of repeated calls for the last tool.
// Returns 0 if there's no counter or no calls have been made.
func RetriesCount(ctx context.Context) int64 {
	if c, ok := ctx.Value(execCounterCtxKey{}).(*execCounter); ok {
		return c.retries.Load()
	}
	return 0
}
