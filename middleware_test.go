package toolsy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMiddlewareMinTool(
	name string,
	execute func(context.Context, RunContext, ToolInput, func(Chunk) error) error,
) *minTool {
	return &minTool{
		manifest: ToolManifest{Name: name, Parameters: map[string]any{"type": "object"}},
		execute:  execute,
	}
}

func TestWithLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	inner := newMiddlewareMinTool(
		"log_me",
		func(_ context.Context, _ RunContext, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{"ok":true}`), MimeType: MimeTypeJSON})
		},
	)
	wrapped := WithLogging(logger)(inner)

	var out []byte
	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(c Chunk) error {
		out = c.Data
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []byte(`{"ok":true}`), out)

	logStr := buf.String()
	assert.Contains(t, logStr, "tool start")
	assert.Contains(t, logStr, "tool end")
	assert.Contains(t, logStr, "log_me")
}

func TestWithLogging_SoftErrorChunkLogsToolError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	inner := newMiddlewareMinTool(
		"log_soft_error",
		func(_ context.Context, _ RunContext, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{
				Event:    EventResult,
				Data:     []byte("budget exceeded"),
				MimeType: MimeTypeText,
				IsError:  true,
			})
		},
	)
	wrapped := WithLogging(logger)(inner)

	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error {
		return nil
	})
	require.NoError(t, err)

	logStr := buf.String()
	assert.Contains(t, logStr, "tool start")
	assert.Contains(t, logStr, "tool error")
	assert.Contains(t, logStr, "error_chunks=1")
	assert.Contains(t, logStr, "last_error_text=\"budget exceeded\"")
}

func TestWithRecovery(t *testing.T) {
	inner := newMiddlewareMinTool(
		"panic_me",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			panic("test panic")
		},
	)
	wrapped := WithRecovery()(inner)

	err := wrapped.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	var sysErr *SystemError
	require.ErrorAs(t, err, &sysErr)
	assert.Contains(t, sysErr.Err.Error(), "panic")
}

func TestRegistryBuilderUse(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	tool, err := NewTool("wrap_me", "desc", func(_ context.Context, _ RunContext, a A) (R, error) {
		return R{Y: a.X + 1}, nil
	})
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Use(WithLogging(logger)).Add(tool).Build()
	require.NoError(t, err)

	args, _ := json.Marshal(A{X: 2})
	var r R
	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "wrap_me", Input: ToolInput{CallID: "1", ArgsJSON: args}},
		func(c Chunk) error { return json.Unmarshal(c.Data, &r) },
	)
	require.NoError(t, err)
	assert.Equal(t, 3, r.Y)
	assert.Equal(t, 1, strings.Count(buf.String(), "tool start"))
}

func TestMiddlewareShortCircuitSkipsInnerTool(t *testing.T) {
	var called atomic.Bool
	inner := newMiddlewareMinTool(
		"blocked",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			called.Store(true)
			return nil
		},
	)

	errRateLimit := errors.New("rate limit exceeded")
	shortCircuit := func(next Tool) Tool {
		return newMiddlewareMinTool(
			next.Manifest().Name,
			func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
				return errRateLimit
			},
		)
	}

	reg, err := NewRegistryBuilder().Use(shortCircuit).Add(inner).Build()
	require.NoError(t, err)

	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "blocked", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, errRateLimit)
	assert.False(t, called.Load())
}
