package toolsy

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	inner := &minTool{name: "log_me", desc: "desc", params: map[string]any{}}
	inner.execute = func(_ context.Context, _ []byte, yield func(Chunk) error) error {
		return yield(Chunk{Data: []byte(`{"ok":true}`)})
	}
	wrapped := WithLogging(logger)(inner)
	var out []byte
	err := wrapped.Execute(context.Background(), []byte(`{}`), func(c Chunk) error {
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

func TestWithRecovery(t *testing.T) {
	inner := &minTool{name: "panic_me", desc: "desc", params: map[string]any{}}
	inner.execute = func(_ context.Context, _ []byte, _ func(Chunk) error) error {
		panic("test panic")
	}
	wrapped := WithRecovery()(inner)
	err := wrapped.Execute(context.Background(), []byte(`{}`), func(Chunk) error { return nil })
	require.Error(t, err)
	var sysErr *SystemError
	require.ErrorAs(t, err, &sysErr)
	// SystemError hides message; unwrapped error contains "panic"
	assert.Contains(t, sysErr.Err.Error(), "panic")
}

func TestWithTimeoutMiddleware(t *testing.T) {
	inner := &minTool{name: "slow", desc: "desc", params: map[string]any{}}
	inner.execute = func(ctx context.Context, _ []byte, _ func(Chunk) error) error {
		<-ctx.Done()
		return ctx.Err()
	}
	wrapped := WithTimeoutMiddleware(5 * time.Millisecond)(inner)
	ctx := context.Background()
	err := wrapped.Execute(ctx, []byte(`{}`), func(Chunk) error { return nil })
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRegistry_Use(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("wrap_me", "desc", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X + 1}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry()
	reg.Register(tool)
	reg.Use(WithRecovery(), WithLogging(slog.Default()))
	args, _ := json.Marshal(A{X: 2})
	var result []byte
	err = reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "wrap_me", Args: json.RawMessage(args)}, func(c Chunk) error {
		result = c.Data
		return nil
	})
	require.NoError(t, err)
	var r R
	require.NoError(t, json.Unmarshal(result, &r))
	assert.Equal(t, 3, r.Y)
}

// TestRegistry_Use_NoDoubleWrap verifies that calling Use() twice rewraps from raw tools,
// so middlewares are not applied twice.
func TestRegistry_Use_NoDoubleWrap(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("double", "desc", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry()
	reg.Register(tool)
	reg.Use(WithRecovery())
	reg.Use(WithLogging(logger))
	var result []byte
	err = reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "double", Args: []byte(`{"x":3}`)}, func(c Chunk) error {
		result = c.Data
		return nil
	})
	require.NoError(t, err)
	logStr := buf.String()
	// With double-wrap we would see "tool start" twice (Logging(Logging(tool))). With rewrap-from-raw we see once.
	require.Equal(t, 1, strings.Count(logStr, "tool start"))
	var r R
	require.NoError(t, json.Unmarshal(result, &r))
	assert.Equal(t, 6, r.Y)
}
