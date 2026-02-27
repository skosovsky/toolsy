package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func raw(s string) json.RawMessage { return []byte(s) }

func TestRegistry_Register_Execute(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("double", "Double x", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry(WithDefaultTimeout(time.Second), WithRecoverPanics(true))
	reg.Register(tool)
	all := reg.GetAllTools()
	require.Len(t, all, 1)
	res := reg.Execute(context.Background(), ToolCall{
		ID: "1", ToolName: "double", Args: raw(`{"x": 7}`),
	})
	require.NoError(t, res.Error)
	require.NotNil(t, res.Result)
	var out R
	require.NoError(t, json.Unmarshal(res.Result, &out))
	assert.Equal(t, 14, out.Y)
}

func TestRegistry_GetTool(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("double", "Double x", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry()
	reg.Register(tool)
	got, ok := reg.GetTool("double")
	require.True(t, ok)
	require.Same(t, tool, got)
	_, ok = reg.GetTool("missing")
	require.False(t, ok)
}

func TestRegistry_Execute_ToolNotFound(t *testing.T) {
	reg := NewRegistry()
	res := reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "missing", Args: raw("{}")})
	require.Error(t, res.Error)
	assert.ErrorIs(t, res.Error, ErrToolNotFound)
}

func TestRegistry_Execute_PanicRecovery(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct{}
	tool, err := NewTool("panic", "Panics", func(_ context.Context, _ A) (R, error) {
		panic("oops")
	})
	require.NoError(t, err)
	reg := NewRegistry(WithRecoverPanics(true))
	reg.Register(tool)
	res := reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "panic", Args: raw(`{"x": 1}`)})
	require.Error(t, res.Error)
	var se *SystemError
	require.ErrorAs(t, res.Error, &se)
}

func TestRegistry_ExecuteBatch_PartialSuccess(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("double", "Double", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry(WithDefaultTimeout(time.Second))
	reg.Register(tool)
	calls := []ToolCall{
		{ID: "1", ToolName: "double", Args: raw(`{"x": 1}`)},
		{ID: "2", ToolName: "missing", Args: raw("{}")},
		{ID: "3", ToolName: "double", Args: raw(`{"x": 3}`)},
	}
	results := reg.ExecuteBatch(context.Background(), calls)
	require.Len(t, results, 3)
	require.NoError(t, results[0].Error)
	require.Error(t, results[1].Error)
	require.ErrorIs(t, results[1].Error, ErrToolNotFound)
	require.NoError(t, results[2].Error)
}

func TestRegistry_Shutdown(t *testing.T) {
	reg := NewRegistry()
	nop, err := NewTool("nop", "nop", func(_ context.Context, _ struct{}) (struct{}, error) {
		return struct{}{}, nil
	})
	require.NoError(t, err)
	reg.Register(nop)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = reg.Shutdown(ctx)
	require.NoError(t, err)
	res := reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "nop", Args: raw("{}")})
	assert.ErrorIs(t, res.Error, ErrShutdown)
}

func TestRegistry_Shutdown_InFlight(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct{}
	done := make(chan struct{})
	tool, err := NewTool("slow", "Slow", func(_ context.Context, _ A) (R, error) {
		time.Sleep(50 * time.Millisecond)
		close(done)
		return R{}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry(WithDefaultTimeout(5 * time.Second))
	reg.Register(tool)
	go reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "slow", Args: raw(`{"x":1}`)})
	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = reg.Shutdown(ctx)
	require.NoError(t, err)
	select {
	case <-done:
	default:
		t.Fatal("in-flight execution should have completed before Shutdown returned")
	}
}

func TestRegistry_Execute_CancelledContext(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("double", "Double", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry(WithDefaultTimeout(time.Second))
	reg.Register(tool)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := reg.Execute(ctx, ToolCall{ID: "1", ToolName: "double", Args: raw(`{"x": 1}`)})
	require.Error(t, res.Error)
	assert.True(t, errors.Is(res.Error, context.Canceled) || errors.Is(res.Error, ErrTimeout),
		"expected context.Canceled or ErrTimeout, got %v", res.Error)
}

func TestRegistry_MaxConcurrency(t *testing.T) {
	var running int32
	type A struct {
		X int `json:"x"`
	}
	type R struct{}
	tool, err := NewTool("slow", "Slow", func(ctx context.Context, _ A) (R, error) {
		atomic.AddInt32(&running, 1)
		defer atomic.AddInt32(&running, -1)
		select {
		case <-ctx.Done():
			return R{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return R{}, nil
		}
	})
	require.NoError(t, err)
	reg := NewRegistry(WithMaxConcurrency(1), WithDefaultTimeout(time.Second))
	reg.Register(tool)
	ctx := context.Background()
	go reg.Execute(ctx, ToolCall{ID: "1", ToolName: "slow", Args: raw(`{"x": 1}`)})
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&running))
	res2 := reg.Execute(ctx, ToolCall{ID: "2", ToolName: "slow", Args: raw(`{"x": 2}`)})
	require.NoError(t, res2.Error)
}

func TestRegistry_ObservabilityHooks(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("add_one", "Add one", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X + 1}, nil
	})
	require.NoError(t, err)
	var beforeCalls, afterCalls int
	var lastCall ToolCall
	var lastResult ToolResult
	var lastDuration time.Duration
	reg := NewRegistry(
		WithOnBeforeExecute(func(_ context.Context, call ToolCall) {
			beforeCalls++
			lastCall = call
		}),
		WithOnAfterExecute(func(_ context.Context, _ ToolCall, result ToolResult, duration time.Duration) {
			afterCalls++
			lastResult = result
			lastDuration = duration
		}),
	)
	reg.Register(tool)
	res := reg.Execute(context.Background(), ToolCall{ID: "h1", ToolName: "add_one", Args: raw(`{"x": 10}`)})
	require.NoError(t, res.Error)
	assert.Equal(t, 1, beforeCalls)
	assert.Equal(t, 1, afterCalls)
	assert.Equal(t, "h1", lastCall.ID)
	assert.Equal(t, "add_one", lastCall.ToolName)
	assert.Equal(t, "h1", lastResult.CallID)
	assert.NotNil(t, lastResult.Result)
	assert.GreaterOrEqual(t, lastDuration, time.Duration(0))
}
