package toolsy

import (
	"context"
	"log/slog"
	"time"
)

// Middleware wraps a Tool with cross-cutting behavior (logging, recovery, timeout).
type Middleware func(Tool) Tool

// WithLogging returns a middleware that logs start, end, duration, and errors.
func WithLogging(logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next Tool) Tool {
		return &middlewareTool{toolBase: toolBase{next: next}, logger: logger}
	}
}

// WithRecovery returns a middleware that recovers panics and returns SystemError.
func WithRecovery() Middleware {
	return func(next Tool) Tool {
		return &recoveryTool{toolBase{next: next}}
	}
}

// WithTimeoutMiddleware returns a middleware that enforces a per-tool timeout (overrides registry default for this tool).
// Named with "Middleware" suffix to avoid collision with ToolOption WithTimeout. When both registry default timeout
// and this middleware apply, the effective timeout is the minimum of the two (inner context cancels first).
func WithTimeoutMiddleware(d time.Duration) Middleware {
	return func(next Tool) Tool {
		return &timeoutTool{toolBase: toolBase{next: next}, timeout: d}
	}
}

// toolBase delegates Tool and ToolMetadata to the wrapped Tool; used by middleware wrappers.
type toolBase struct{ next Tool }

func (b *toolBase) Name() string               { return b.next.Name() }
func (b *toolBase) Description() string        { return b.next.Description() }
func (b *toolBase) Parameters() map[string]any { return b.next.Parameters() }

func (b *toolBase) Timeout() time.Duration {
	if tm, ok := b.next.(ToolMetadata); ok {
		return tm.Timeout()
	}
	return 0
}
func (b *toolBase) Tags() []string {
	if tm, ok := b.next.(ToolMetadata); ok {
		return tm.Tags()
	}
	return nil
}
func (b *toolBase) Version() string {
	if tm, ok := b.next.(ToolMetadata); ok {
		return tm.Version()
	}
	return ""
}
func (b *toolBase) IsDangerous() bool {
	if tm, ok := b.next.(ToolMetadata); ok {
		return tm.IsDangerous()
	}
	return false
}

type middlewareTool struct {
	toolBase
	logger *slog.Logger
}

func (m *middlewareTool) Execute(ctx context.Context, args []byte) ([]byte, error) {
	m.logger.Info("tool start", "tool", m.next.Name())
	start := time.Now()
	res, err := m.next.Execute(ctx, args)
	dur := time.Since(start)
	if err != nil {
		m.logger.Error("tool error", "tool", m.next.Name(), "duration", dur, "error", err)
		return nil, err
	}
	m.logger.Info("tool end", "tool", m.next.Name(), "duration", dur)
	return res, nil
}

type recoveryTool struct{ toolBase }

func (r *recoveryTool) Execute(ctx context.Context, args []byte) (res []byte, err error) {
	defer func() {
		if p := recover(); p != nil {
			res = nil
			err = &SystemError{Err: &panicError{p: p}}
		}
	}()
	return r.next.Execute(ctx, args)
}

type timeoutTool struct {
	toolBase
	timeout time.Duration
}

func (t *timeoutTool) Timeout() time.Duration {
	if t.timeout > 0 {
		return t.timeout
	}
	return t.toolBase.Timeout()
}

func (t *timeoutTool) Execute(ctx context.Context, args []byte) ([]byte, error) {
	if t.timeout <= 0 {
		return t.next.Execute(ctx, args)
	}
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	return t.next.Execute(ctx, args)
}

// Use stores the given middlewares and reapplies them from scratch to all registered tools (onion order:
// first middleware is outermost). Tools registered after Use will also get these middlewares applied.
// Calling Use multiple times replaces the middleware chain and rewraps from raw tools, avoiding double-wrapping.
func (r *Registry) Use(middlewares ...Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middlewares = middlewares
	for name, raw := range r.rawTools {
		t := raw
		for i := len(middlewares) - 1; i >= 0; i-- {
			t = middlewares[i](t)
		}
		r.tools[name] = t
	}
}
