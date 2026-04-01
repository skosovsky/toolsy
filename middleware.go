package toolsy

import (
	"context"
	"log/slog"
	"time"
	"unicode/utf8"
)

// Middleware wraps a Tool with cross-cutting behavior (logging, recovery).
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
//
// Deprecated: [Registry] is built with panic recovery enabled by default ([NewRegistryBuilder] sets
// recoverPanics). Using WithRecovery in Use() recovers panics before the registry's recovery and
// onAfter hooks can observe them. Prefer relying on registry recovery only; this middleware remains
// for rare direct Tool.Execute paths without a registry.
func WithRecovery() Middleware {
	return func(next Tool) Tool {
		return &recoveryTool{toolBase{next: next}}
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
	var chunks, totalBytes, errorChunks int64
	var lastErrorText string
	yieldWrapped := func(c Chunk) error {
		if c.IsError {
			errorChunks++
			if c.MimeType == MimeTypeText && utf8.Valid(c.Data) {
				lastErrorText = string(c.Data)
			}
			return yield(c)
		}
		if !c.IsError {
			chunks++
			totalBytes += int64(len(c.Data))
		}
		return yield(c)
	}
	var err error
	defer func() {
		dur := time.Since(start)
		if err != nil || errorChunks > 0 {
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
				"error_chunks",
				errorChunks,
				"last_error_text",
				lastErrorText,
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
