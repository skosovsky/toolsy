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
	started := make(chan struct{})
	done := make(chan struct{})
	tool, err := NewTool("slow", "Slow", func(_ context.Context, _ A) (R, error) {
		close(started)
		time.Sleep(50 * time.Millisecond)
		close(done)
		return R{}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry(WithDefaultTimeout(5 * time.Second))
	reg.Register(tool)
	go reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "slow", Args: raw(`{"x":1}`)})
	<-started
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
	started := make(chan struct{}, 1)
	type A struct {
		X int `json:"x"`
	}
	type R struct{}
	tool, err := NewTool("slow", "Slow", func(ctx context.Context, _ A) (R, error) {
		atomic.AddInt32(&running, 1)
		defer atomic.AddInt32(&running, -1)
		select {
		case started <- struct{}{}:
		default:
		}
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
	<-started
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

func TestRegistry_ExecuteBatch_Empty(t *testing.T) {
	reg := NewRegistry()
	results := reg.ExecuteBatch(context.Background(), nil)
	assert.Empty(t, results)
	results = reg.ExecuteBatch(context.Background(), []ToolCall{})
	assert.Empty(t, results)
}

func TestRegistry_Shutdown_Idempotent(t *testing.T) {
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
	err = reg.Shutdown(ctx)
	require.NoError(t, err)
}

func TestRegistry_Register_Overwrite(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	first, err := NewTool("same", "First", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X}, nil
	})
	require.NoError(t, err)
	second, err := NewTool("same", "Second", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X * 10}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry()
	reg.Register(first)
	reg.Register(second)
	got, ok := reg.GetTool("same")
	require.True(t, ok)
	require.Same(t, second, got)
	res := reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "same", Args: raw(`{"x": 5}`)})
	require.NoError(t, res.Error)
	var out R
	require.NoError(t, json.Unmarshal(res.Result, &out))
	assert.Equal(t, 50, out.Y)
}

func TestRegistry_MaxConcurrency_Unlimited(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("inc", "Increment", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X + 1}, nil
	})
	require.NoError(t, err)
	for _, n := range []int{0, -1} {
		name := "Zero"
		if n < 0 {
			name = "Negative"
		}
		t.Run(name, func(t *testing.T) {
			reg := NewRegistry(WithMaxConcurrency(n), WithDefaultTimeout(time.Second))
			reg.Register(tool)
			results := reg.ExecuteBatch(context.Background(), []ToolCall{
				{ID: "1", ToolName: "inc", Args: raw(`{"x": 1}`)},
				{ID: "2", ToolName: "inc", Args: raw(`{"x": 2}`)},
			})
			require.Len(t, results, 2)
			require.NoError(t, results[0].Error)
			require.NoError(t, results[1].Error)
		})
	}
}

func TestRegistry_OnAfter_ErrorPath(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct{}
	errSentinel := errors.New("tool error")
	tool, err := NewTool("fail", "Fails", func(_ context.Context, _ A) (R, error) {
		return R{}, errSentinel
	})
	require.NoError(t, err)
	var afterCalls int
	var lastResult ToolResult
	reg := NewRegistry(WithOnAfterExecute(func(_ context.Context, _ ToolCall, result ToolResult, _ time.Duration) {
		afterCalls++
		lastResult = result
	}))
	reg.Register(tool)
	res := reg.Execute(context.Background(), ToolCall{ID: "e1", ToolName: "fail", Args: raw(`{"x": 1}`)})
	require.Error(t, res.Error)
	require.ErrorIs(t, res.Error, errSentinel)
	assert.Equal(t, 1, afterCalls)
	assert.Equal(t, "e1", lastResult.CallID)
	assert.Equal(t, "fail", lastResult.ToolName)
	assert.ErrorIs(t, lastResult.Error, errSentinel)
}
