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
	var result []byte
	err = reg.Execute(context.Background(), ToolCall{
		ID: "1", ToolName: "double", Args: raw(`{"x": 7}`),
	}, func(chunk []byte) error {
		result = chunk
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	var out R
	require.NoError(t, json.Unmarshal(result, &out))
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
	err := reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "missing", Args: raw("{}")}, func([]byte) error { return nil })
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrToolNotFound)
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
	err = reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "panic", Args: raw(`{"x": 1}`)}, func([]byte) error { return nil })
	require.Error(t, err)
	var se *SystemError
	require.ErrorAs(t, err, &se)
}

// TestRegistry_Execute_PanicRecovery_OnAfterSummary verifies that when panic is recovered,
// the returned error is non-nil and onAfter receives ExecutionSummary with that error.
func TestRegistry_Execute_PanicRecovery_OnAfterSummary(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct{}
	tool, err := NewTool("panic", "Panics", func(_ context.Context, _ A) (R, error) {
		panic("oops")
	})
	require.NoError(t, err)
	var lastSummary ExecutionSummary
	reg := NewRegistry(
		WithRecoverPanics(true),
		WithOnAfterExecute(func(_ context.Context, _ ToolCall, summary ExecutionSummary, _ time.Duration) {
			lastSummary = summary
		}),
	)
	reg.Register(tool)
	err = reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "panic", Args: raw(`{"x": 1}`)}, func([]byte) error { return nil })
	require.Error(t, err)
	var panicSE *SystemError
	require.ErrorAs(t, err, &panicSE)
	assert.Equal(t, "1", lastSummary.CallID)
	assert.Equal(t, "panic", lastSummary.ToolName)
	require.Error(t, lastSummary.Error)
	require.ErrorAs(t, lastSummary.Error, &panicSE)
}

// TestRegistry_Execute_PanicInYield verifies that panic inside the yield callback is recovered
// and Execute returns SystemError; onAfter receives summary with that error.
func TestRegistry_Execute_PanicInYield(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	tool, err := NewStreamTool("stream_two", "Yields twice", func(_ context.Context, a A, yield func([]byte) error) error {
		for i := 0; i < a.N; i++ {
			if err := yield([]byte{byte('0' + i)}); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)
	var lastSummary ExecutionSummary
	reg := NewRegistry(
		WithRecoverPanics(true),
		WithOnAfterExecute(func(_ context.Context, _ ToolCall, summary ExecutionSummary, _ time.Duration) {
			lastSummary = summary
		}),
	)
	reg.Register(tool)
	callCount := 0
	err = reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "stream_two", Args: raw(`{"n": 3}`)}, func([]byte) error {
		callCount++
		if callCount == 2 {
			panic("yield panic")
		}
		return nil
	})
	require.Error(t, err)
	var se *SystemError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, "1", lastSummary.CallID)
	assert.Equal(t, "stream_two", lastSummary.ToolName)
	require.Error(t, lastSummary.Error)
	require.ErrorAs(t, lastSummary.Error, &se)
	// First chunk was delivered before panic in yield.
	assert.Equal(t, 1, lastSummary.ChunksDelivered)
}

// TestRegistry_OnChunk_OnlySuccessfulChunks verifies that WithOnChunk is invoked only for
// successfully delivered chunks (when yield returns nil), and Chunk has correct CallID, ToolName, Data.
// ExecutionSummary reflects only successfully delivered chunks/bytes.
func TestRegistry_OnChunk_OnlySuccessfulChunks(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	tool, err := NewStreamTool("stream", "Stream N", func(_ context.Context, a A, yield func([]byte) error) error {
		for i := 0; i < a.N; i++ {
			b := []byte{byte('0' + i)}
			if err := yield(b); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)
	var onChunkCalls []Chunk
	var lastSummary ExecutionSummary
	yieldErr := errors.New("abort after first")
	reg := NewRegistry(
		WithOnChunk(func(_ context.Context, c Chunk) {
			onChunkCalls = append(onChunkCalls, c)
		}),
		WithOnAfterExecute(func(_ context.Context, _ ToolCall, summary ExecutionSummary, _ time.Duration) {
			lastSummary = summary
		}),
	)
	reg.Register(tool)
	err = reg.Execute(context.Background(), ToolCall{ID: "call-1", ToolName: "stream", Args: raw(`{"n": 3}`)}, func(data []byte) error {
		if string(data) == "1" {
			return yieldErr
		}
		return nil
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStreamAborted)
	// onChunk must be called only for chunks that were successfully delivered (first: "0").
	assert.Len(t, onChunkCalls, 1)
	assert.Equal(t, "call-1", onChunkCalls[0].CallID)
	assert.Equal(t, "stream", onChunkCalls[0].ToolName)
	assert.Equal(t, []byte("0"), onChunkCalls[0].Data)
	// ExecutionSummary: one chunk, one byte delivered before yield error.
	assert.Equal(t, 1, lastSummary.ChunksDelivered)
	assert.Equal(t, int64(1), lastSummary.TotalBytes)
}

func TestRegistry_ExecuteBatchStream_PartialSuccess(t *testing.T) {
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
	var chunks []Chunk
	err = reg.ExecuteBatchStream(context.Background(), calls, func(c Chunk) error {
		chunks = append(chunks, c)
		return nil
	})
	// ExecuteBatchStream returns first error; one call was missing so we get ErrToolNotFound.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrToolNotFound)
	// We may still get chunks from successful tools before the error is aggregated
	var out R
	for _, c := range chunks {
		if c.ToolName == "double" && len(c.Data) > 0 {
			require.NoError(t, json.Unmarshal(c.Data, &out))
			assert.True(t, out.Y == 2 || out.Y == 6)
		}
	}
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
	err = reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "nop", Args: raw("{}")}, func([]byte) error { return nil })
	assert.ErrorIs(t, err, ErrShutdown)
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
	go func() {
		_ = reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "slow", Args: raw(`{"x":1}`)}, func([]byte) error { return nil })
	}()
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
	err = reg.Execute(ctx, ToolCall{ID: "1", ToolName: "double", Args: raw(`{"x": 1}`)}, func([]byte) error { return nil })
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, ErrTimeout),
		"expected context.Canceled or ErrTimeout, got %v", err)
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
	go func() {
		_ = reg.Execute(ctx, ToolCall{ID: "1", ToolName: "slow", Args: raw(`{"x": 1}`)}, func([]byte) error { return nil })
	}()
	<-started
	assert.Equal(t, int32(1), atomic.LoadInt32(&running))
	err = reg.Execute(ctx, ToolCall{ID: "2", ToolName: "slow", Args: raw(`{"x": 2}`)}, func([]byte) error { return nil })
	require.NoError(t, err)
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
	var lastSummary ExecutionSummary
	var lastDuration time.Duration
	reg := NewRegistry(
		WithOnBeforeExecute(func(_ context.Context, call ToolCall) {
			beforeCalls++
			lastCall = call
		}),
		WithOnAfterExecute(func(_ context.Context, _ ToolCall, summary ExecutionSummary, duration time.Duration) {
			afterCalls++
			lastSummary = summary
			lastDuration = duration
		}),
	)
	reg.Register(tool)
	err = reg.Execute(context.Background(), ToolCall{ID: "h1", ToolName: "add_one", Args: raw(`{"x": 10}`)}, func([]byte) error { return nil })
	require.NoError(t, err)
	assert.Equal(t, 1, beforeCalls)
	assert.Equal(t, 1, afterCalls)
	assert.Equal(t, "h1", lastCall.ID)
	assert.Equal(t, "add_one", lastCall.ToolName)
	assert.Equal(t, "h1", lastSummary.CallID)
	assert.Equal(t, 1, lastSummary.ChunksDelivered)
	assert.GreaterOrEqual(t, lastSummary.TotalBytes, int64(1), "one chunk delivered")
	assert.GreaterOrEqual(t, lastDuration, time.Duration(0))
}

func TestRegistry_ExecuteBatchStream_Empty(t *testing.T) {
	reg := NewRegistry()
	err := reg.ExecuteBatchStream(context.Background(), nil, func(Chunk) error { return nil })
	require.NoError(t, err)
	err = reg.ExecuteBatchStream(context.Background(), []ToolCall{}, func(Chunk) error { return nil })
	require.NoError(t, err)
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
	var result []byte
	err = reg.Execute(context.Background(), ToolCall{ID: "1", ToolName: "same", Args: raw(`{"x": 5}`)}, func(chunk []byte) error {
		result = chunk
		return nil
	})
	require.NoError(t, err)
	var out R
	require.NoError(t, json.Unmarshal(result, &out))
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
			err := reg.ExecuteBatchStream(context.Background(), []ToolCall{
				{ID: "1", ToolName: "inc", Args: raw(`{"x": 1}`)},
				{ID: "2", ToolName: "inc", Args: raw(`{"x": 2}`)},
			}, func(Chunk) error { return nil })
			require.NoError(t, err)
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
	var lastSummary ExecutionSummary
	reg := NewRegistry(WithOnAfterExecute(func(_ context.Context, _ ToolCall, summary ExecutionSummary, _ time.Duration) {
		afterCalls++
		lastSummary = summary
	}))
	reg.Register(tool)
	err = reg.Execute(context.Background(), ToolCall{ID: "e1", ToolName: "fail", Args: raw(`{"x": 1}`)}, func([]byte) error { return nil })
	require.Error(t, err)
	require.ErrorIs(t, err, errSentinel)
	assert.Equal(t, 1, afterCalls)
	assert.Equal(t, "e1", lastSummary.CallID)
	assert.Equal(t, "fail", lastSummary.ToolName)
	assert.ErrorIs(t, lastSummary.Error, errSentinel)
}
