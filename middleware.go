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

// toolBase delegates Tool to the wrapped Tool; used by middleware wrappers.
type toolBase struct{ next Tool }

func (b *toolBase) Manifest() ToolManifest { return b.next.Manifest() }

type middlewareTool struct {
	toolBase

	logger *slog.Logger
}

func (m *middlewareTool) Execute(ctx context.Context, run RunContext, input ToolInput, yield func(Chunk) error) error {
	toolName := m.next.Manifest().Name
	m.logger.InfoContext(ctx, "tool start", "tool", toolName)
	start := time.Now()
	var chunks, totalBytes int64
	yieldWrapped := func(c Chunk) error {
		if !c.IsError {
			chunks++
			totalBytes += int64(len(c.Data))
		}
		return yield(c)
	}
	var err error
	defer func() {
		dur := time.Since(start)
		if err != nil {
			m.logger.Error(
				"tool error",
				"tool",
				toolName,
				"duration",
				dur,
				"chunks",
				chunks,
				"bytes",
				totalBytes,
				"error",
				err,
			)
		} else {
			m.logger.Info("tool end", "tool", toolName, "duration", dur, "chunks", chunks, "bytes", totalBytes)
		}
	}()
	err = m.next.Execute(ctx, run, input, yieldWrapped)
	return err
}

type recoveryTool struct{ toolBase }

func (r *recoveryTool) Execute(
	ctx context.Context,
	run RunContext,
	input ToolInput,
	yield func(Chunk) error,
) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = &SystemError{Err: &panicError{p: p}}
		}
	}()
	return r.next.Execute(ctx, run, input, yield)
}

type timeoutTool struct {
	toolBase

	timeout time.Duration
}

func (t *timeoutTool) Manifest() ToolManifest {
	manifest := t.toolBase.Manifest()
	if t.timeout > 0 {
		manifest.Timeout = t.timeout
	}
	return manifest
}

func (t *timeoutTool) Execute(ctx context.Context, run RunContext, input ToolInput, yield func(Chunk) error) error {
	if t.timeout <= 0 {
		return t.next.Execute(ctx, run, input, yield)
	}
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	return t.next.Execute(ctx, run, input, yield)
}
