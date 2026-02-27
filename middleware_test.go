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
	inner.execute = func(_ context.Context, _ []byte) ([]byte, error) {
		return []byte(`{"ok":true}`), nil
	}
	wrapped := WithLogging(logger)(inner)
	out, err := wrapped.Execute(context.Background(), []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, []byte(`{"ok":true}`), out)
	logStr := buf.String()
	assert.Contains(t, logStr, "tool start")
	assert.Contains(t, logStr, "tool end")
	assert.Contains(t, logStr, "log_me")
}

func TestWithRecovery(t *testing.T) {
	inner := &minTool{name: "panic_me", desc: "desc", params: map[string]any{}}
	inner.execute = func(_ context.Context, _ []byte) ([]byte, error) {
		panic("test panic")
	}
	wrapped := WithRecovery()(inner)
	res, err := wrapped.Execute(context.Background(), []byte(`{}`))
	require.Error(t, err)
	assert.Nil(t, res)
	var sysErr *SystemError
	require.ErrorAs(t, err, &sysErr)
	// SystemError hides message; unwrapped error contains "panic"
	assert.Contains(t, sysErr.Err.Error(), "panic")
}

func TestWithTimeoutMiddleware(t *testing.T) {
	inner := &minTool{name: "slow", desc: "desc", params: map[string]any{}}
	inner.execute = func(ctx context.Context, _ []byte) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	wrapped := WithTimeoutMiddleware(5 * time.Millisecond)(inner)
	ctx := context.Background()
	res, err := wrapped.Execute(ctx, []byte(`{}`))
	require.Error(t, err)
	assert.Nil(t, res)
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
	result := reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "wrap_me", Args: json.RawMessage(args)})
	require.NoError(t, result.Error)
	var r R
	require.NoError(t, json.Unmarshal(result.Result, &r))
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
	result := reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "double", Args: []byte(`{"x":3}`)})
	require.NoError(t, result.Error)
	logStr := buf.String()
	// With double-wrap we would see "tool start" twice (Logging(Logging(tool))). With rewrap-from-raw we see once.
	require.Equal(t, 1, strings.Count(logStr, "tool start"))
	var r R
	require.NoError(t, json.Unmarshal(result.Result, &r))
	assert.Equal(t, 6, r.Y)
}
