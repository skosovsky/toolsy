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
	var out R
	err = reg.Execute(context.Background(), ToolCall{
		ID: "1", ToolName: "double", Args: raw(`{"x": 7}`),
	}, func(c Chunk) error {
		out = c.RawData.(R)
		return nil
	})
	require.NoError(t, err)
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
	err := reg.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "missing", Args: raw("{}")},
		func(Chunk) error { return nil },
	)
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
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "panic", Args: raw(`{"x": 1}`)},
		func(Chunk) error { return nil },
	)
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
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "panic", Args: raw(`{"x": 1}`)},
		func(Chunk) error { return nil },
	)
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
	tool, err := NewStreamTool(
		"stream_two",
		"Yields twice",
		func(_ context.Context, a A, yield func(Chunk) error) error {
			for i := range a.N {
				if err := yield(Chunk{Data: []byte{byte('0' + i)}, MimeType: MimeTypeText}); err != nil {
					return err
				}
			}
			return nil
		},
	)
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
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "stream_two", Args: raw(`{"n": 3}`)},
		func(Chunk) error {
			callCount++
			if callCount == 2 {
				panic("yield panic")
			}
			return nil
		},
	)
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

// TestContextSafeYield verifies that when context is cancelled after receiving some chunks,
// the tool's yield sees ctx.Err() and returns; no panic, no goroutine leak.
func TestContextSafeYield(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	var yieldsBeforeCancel int32
	tool, err := NewStreamTool(
		"slow_stream",
		"Yields N with delay",
		func(ctx context.Context, a A, yield func(Chunk) error) error {
			for i := range a.N {
				if err := ctx.Err(); err != nil {
					return err
				}
				atomic.StoreInt32(&yieldsBeforeCancel, int32(i+1))
				if err := yield(Chunk{
					Event:    EventResult,
					Data:     []byte{byte('0' + i)},
					MimeType: MimeTypeText,
				}); err != nil {
					return err
				}
				time.Sleep(30 * time.Millisecond)
			}
			return nil
		},
	)
	require.NoError(t, err)
	reg := NewRegistry(WithDefaultTimeout(time.Second))
	reg.Register(tool)
	ctx, cancel := context.WithCancel(context.Background())
	var received int
	err = reg.Execute(ctx, ToolCall{ID: "1", ToolName: "slow_stream", Args: raw(`{"n": 10}`)}, func(_ Chunk) error {
		received++
		if received == 2 {
			cancel()
		}
		return nil
	})
	// After cancel we may get context.Canceled or nil (if Execute returned before timeout)
	_ = err
	assert.LessOrEqual(t, received, 3, "at most 2 requested chunks plus one possibly in flight")
	assert.LessOrEqual(t, atomic.LoadInt32(&yieldsBeforeCancel), int32(3), "tool should stop after context cancel")
}

// TestExecuteIter_BreakCancelsContext verifies that breaking out of for range ExecuteIter
// cancels the child context and the tool exits (push-to-push); no extra yield after break.
func TestExecuteIter_BreakCancelsContext(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	var yieldCount int32
	tool, err := NewStreamTool(
		"iter_stream",
		"Yields N",
		func(ctx context.Context, a A, yield func(Chunk) error) error {
			for i := range a.N {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				atomic.StoreInt32(&yieldCount, int32(i+1))
				if err := yield(Chunk{
					Event:    EventResult,
					Data:     []byte{byte('0' + i)},
					MimeType: MimeTypeText,
				}); err != nil {
					return err
				}
			}
			return nil
		},
	)
	require.NoError(t, err)
	reg := NewRegistry(WithDefaultTimeout(time.Second))
	reg.Register(tool)
	call := ToolCall{ID: "iter1", ToolName: "iter_stream", Args: raw(`{"n": 5}`)}
	iterations := 0
	for chunk, err := range reg.ExecuteIter(context.Background(), call) {
		if err != nil {
			break
		}
		iterations++
		_ = chunk
		if iterations >= 1 {
			break
		}
	}
	assert.Equal(t, 1, iterations)
	// Tool must have seen at most 1 yield (after break we cancel and return context.Canceled from callback).
	assert.LessOrEqual(t, atomic.LoadInt32(&yieldCount), int32(1))
}

// TestExecuteIter_FullPass verifies that iterating fully over ExecuteIter delivers all chunks and final error (nil or err).
func TestExecuteIter_FullPass(t *testing.T) {
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
	reg := NewRegistry()
	reg.Register(tool)
	call := ToolCall{ID: "1", ToolName: "double", Args: raw(`{"x": 21}`)}
	var chunks []Chunk
	var finalErr error
	for chunk, err := range reg.ExecuteIter(context.Background(), call) {
		if err != nil {
			finalErr = err
			break
		}
		chunks = append(chunks, chunk)
	}
	require.Len(t, chunks, 1)
	assert.JSONEq(t, `{"y":42}`, string(chunks[0].Data))
	if chunks[0].MimeType != MimeTypeJSON {
		t.Fatalf("unexpected mime type: %s", chunks[0].MimeType)
	}
	assert.Equal(t, 42, chunks[0].RawData.(R).Y)
	assert.NoError(t, finalErr)
}

// TestRegistry_OnChunk_OnlySuccessfulChunks verifies that WithOnChunk is invoked only for
// successfully delivered chunks (when yield returns nil), and Chunk has correct CallID, ToolName, Data.
// ExecutionSummary reflects only successfully delivered chunks/bytes.
func TestRegistry_OnChunk_OnlySuccessfulChunks(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	tool, err := NewStreamTool("stream", "Stream N", func(_ context.Context, a A, yield func(Chunk) error) error {
		for i := range a.N {
			b := []byte{byte('0' + i)}
			if err := yield(Chunk{Data: b, MimeType: MimeTypeText}); err != nil {
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
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "call-1", ToolName: "stream", Args: raw(`{"n": 3}`)},
		func(c Chunk) error {
			if string(c.Data) == "1" {
				return yieldErr
			}
			return nil
		},
	)
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
	// ExecuteBatchStream returns nil (tool errors are sent as IsError chunks).
	require.NoError(t, err)
	var out R
	var errorChunks int
	for _, c := range chunks {
		if c.IsError {
			errorChunks++
			continue
		}
		if c.ToolName == "double" && c.RawData != nil {
			out = c.RawData.(R)
			assert.True(t, out.Y == 2 || out.Y == 6)
		}
	}
	assert.GreaterOrEqual(t, errorChunks, 1, "missing tool should produce at least one error chunk")
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
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "nop", Args: raw("{}")},
		func(Chunk) error { return nil },
	)
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
		_ = reg.Execute(
			context.Background(),
			ToolCall{ID: "1", ToolName: "slow", Args: raw(`{"x":1}`)},
			func(Chunk) error { return nil },
		)
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
	err = reg.Execute(
		ctx,
		ToolCall{ID: "1", ToolName: "double", Args: raw(`{"x": 1}`)},
		func(Chunk) error { return nil },
	)
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
		_ = reg.Execute(
			ctx,
			ToolCall{ID: "1", ToolName: "slow", Args: raw(`{"x": 1}`)},
			func(Chunk) error { return nil },
		)
	}()
	<-started
	assert.Equal(t, int32(1), atomic.LoadInt32(&running))
	err = reg.Execute(ctx, ToolCall{ID: "2", ToolName: "slow", Args: raw(`{"x": 2}`)}, func(Chunk) error { return nil })
	require.NoError(t, err)
}

// TestRegistry_TimeoutDuringSemaphoreWait ensures WithDefaultTimeout applies to the whole execution,
// including time spent waiting for the semaphore (queue wait).
func TestRegistry_TimeoutDuringSemaphoreWait(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct{}
	// Blocker holds the slot without respecting ctx so the second call can time out while waiting.
	blocker, err := NewTool("blocker", "Blocks", func(_ context.Context, _ A) (R, error) {
		time.Sleep(300 * time.Millisecond)
		return R{}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry(WithMaxConcurrency(1), WithDefaultTimeout(50*time.Millisecond))
	reg.Register(blocker)
	ctx := context.Background()
	go func() {
		_ = reg.Execute(
			ctx,
			ToolCall{ID: "1", ToolName: "blocker", Args: raw(`{"x":1}`)},
			func(Chunk) error { return nil },
		)
	}()
	time.Sleep(15 * time.Millisecond) // let first call acquire the slot
	err = reg.Execute(
		ctx,
		ToolCall{ID: "2", ToolName: "blocker", Args: raw(`{"x":2}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTimeout, "second call must time out while waiting for semaphore")
}

// TestRegistry_TimeoutMinimumHierarchy ensures effective timeout is min(registry default, per-tool timeout).
// When tool has a longer timeout than registry, the registry default wins.
func TestRegistry_TimeoutMinimumHierarchy(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct{}
	tool, err := NewTool("slow", "Slow", func(ctx context.Context, _ A) (R, error) {
		select {
		case <-ctx.Done():
			return R{}, ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return R{}, nil
		}
	}, WithTimeout(200*time.Millisecond))
	require.NoError(t, err)
	reg := NewRegistry(WithDefaultTimeout(30 * time.Millisecond))
	reg.Register(tool)
	ctx := context.Background()
	err = reg.Execute(ctx, ToolCall{ID: "1", ToolName: "slow", Args: raw(`{"x":1}`)}, func(Chunk) error { return nil })
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTimeout, "effective timeout must be registry 30ms, not tool 200ms")
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
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "h1", ToolName: "add_one", Args: raw(`{"x": 10}`)},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	assert.Equal(t, 1, beforeCalls)
	assert.Equal(t, 1, afterCalls)
	assert.Equal(t, "h1", lastCall.ID)
	assert.Equal(t, "add_one", lastCall.ToolName)
	assert.Equal(t, "h1", lastSummary.CallID)
	assert.Equal(t, 1, lastSummary.ChunksDelivered)
	assert.Equal(t, int64(len(`{"y":11}`)), lastSummary.TotalBytes)
	assert.GreaterOrEqual(t, lastDuration, time.Duration(0))
}

// TestRegistry_ObservabilityHooks_AsyncToolOnlySeesAcceptedPhase verifies that WithOnBeforeExecute,
// WithOnAfterExecute, and WithOnChunk observe only the synchronous "accepted" phase of an async tool;
// background chunks produced by the base tool are not visible to these hooks.
func TestRegistry_ObservabilityHooks_AsyncToolOnlySeesAcceptedPhase(t *testing.T) {
	done := make(chan struct{})
	base, err := NewTool("worker", "Worker", func(_ context.Context, _ struct{}) (struct{}, error) {
		return struct{}{}, nil
	})
	require.NoError(t, err)
	// Wrap so the base yields multiple chunks in the background; we only care that hooks see one (accepted).
	asyncBase := &asyncYieldTool{base: base, done: done}
	asyncTool := AsAsyncTool(asyncBase)
	var beforeCount, chunkCount, afterCount int32
	reg := NewRegistry(
		WithOnBeforeExecute(func(context.Context, ToolCall) { atomic.AddInt32(&beforeCount, 1) }),
		WithOnChunk(func(context.Context, Chunk) { atomic.AddInt32(&chunkCount, 1) }),
		WithOnAfterExecute(
			func(context.Context, ToolCall, ExecutionSummary, time.Duration) { atomic.AddInt32(&afterCount, 1) },
		),
	)
	reg.Register(asyncTool)
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "worker", Args: raw(`{}`)},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	<-done
	require.Equal(t, int32(1), atomic.LoadInt32(&beforeCount))
	require.Equal(
		t,
		int32(1),
		atomic.LoadInt32(&chunkCount),
		"OnChunk must see only the accepted chunk, not background chunks",
	)
	require.Equal(t, int32(1), atomic.LoadInt32(&afterCount))
}

// asyncYieldTool is a Tool that wraps another and yields multiple chunks in Execute (for observability boundary test).
type asyncYieldTool struct {
	base Tool
	done chan struct{}
}

func (t *asyncYieldTool) Name() string               { return t.base.Name() }
func (t *asyncYieldTool) Description() string        { return t.base.Description() }
func (t *asyncYieldTool) Parameters() map[string]any { return t.base.Parameters() }
func (t *asyncYieldTool) Execute(ctx context.Context, run RunContext, args []byte, yield func(Chunk) error) error {
	_ = yield(Chunk{Event: EventProgress, RawData: "chunk1"})
	_ = yield(Chunk{Event: EventProgress, RawData: "chunk2"})
	_ = yield(Chunk{Event: EventResult, RawData: "chunk3"})
	close(t.done)
	return t.base.Execute(ctx, run, args, yield)
}

// TestRegistry_ExecuteBatchStream_ErrorIsolation verifies that when one tool fails and another succeeds,
// ExecuteBatchStream returns nil (tool errors are sent as IsError chunks) and both outcomes are delivered.
func TestRegistry_ExecuteBatchStream_ErrorIsolation(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	failTool, err := NewTool("fail_soon", "Fails", func(_ context.Context, _ A) (R, error) {
		return R{}, errors.New("tool failed")
	})
	require.NoError(t, err)
	okTool, err := NewTool("ok_later", "Succeeds after delay", func(_ context.Context, a A) (R, error) {
		time.Sleep(100 * time.Millisecond)
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry(WithDefaultTimeout(5 * time.Second))
	reg.Register(failTool)
	reg.Register(okTool)
	calls := []ToolCall{
		{ID: "f1", ToolName: "fail_soon", Args: raw(`{"x": 1}`)},
		{ID: "o1", ToolName: "ok_later", Args: raw(`{"x": 3}`)},
	}
	var chunks []Chunk
	err = reg.ExecuteBatchStream(context.Background(), calls, func(c Chunk) error {
		chunks = append(chunks, c)
		return nil
	})
	require.NoError(t, err)
	var errChunk, okChunk *Chunk
	for i := range chunks {
		c := &chunks[i]
		if c.IsError {
			errChunk = c
		} else if c.ToolName == "ok_later" && c.RawData != nil {
			okChunk = c
		}
	}
	require.NotNil(t, errChunk, "expected one chunk with IsError for fail_soon")
	assert.Equal(t, "f1", errChunk.CallID)
	assert.Equal(t, "fail_soon", errChunk.ToolName)
	require.NotNil(t, okChunk, "expected one success chunk for ok_later")
	out := okChunk.RawData.(R)
	assert.Equal(t, 6, out.Y)
}

// TestRegistry_ExecuteBatchStream_ValidatorFailure_ErrorChunk is a regression test for the batch bridge:
// when the validator fails, the error is mapped to Chunk{IsError: true} with LLM-readable message,
// and the tool handler is never called (fail-closed; atomic counter stays 0).
func TestRegistry_ExecuteBatchStream_ValidatorFailure_ErrorChunk(t *testing.T) {
	type A struct{}
	type R struct{}
	var handlerCalls atomic.Int32
	tool, err := NewTool("guarded", "Guarded", func(_ context.Context, _ A) (R, error) {
		handlerCalls.Add(1)
		return R{}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry(WithValidator(&testValidator{validateFn: func(_ context.Context, _, _ string) error {
		return errors.New("security validation failed")
	}}))
	reg.Register(tool)
	calls := []ToolCall{{ID: "1", ToolName: "guarded", Args: raw(`{}`)}}
	var chunks []Chunk
	err = reg.ExecuteBatchStream(context.Background(), calls, func(c Chunk) error {
		chunks = append(chunks, c)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, chunks, 1, "expected exactly one chunk (error)")
	c := chunks[0]
	assert.True(t, c.IsError, "chunk must be error-path for validator failure")
	assert.Contains(t, string(c.Data), "security validation failed", "error text must be LLM-readable")
	assert.Contains(t, string(c.Data), "Please fix the arguments and try again", "error text must hint self-correction")
	assert.Equal(t, int32(0), handlerCalls.Load(), "handler must not be called (fail-closed)")
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
	var out R
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "same", Args: raw(`{"x": 5}`)},
		func(c Chunk) error {
			out = c.RawData.(R)
			return nil
		},
	)
	require.NoError(t, err)
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
	reg := NewRegistry(
		WithOnAfterExecute(func(_ context.Context, _ ToolCall, summary ExecutionSummary, _ time.Duration) {
			afterCalls++
			lastSummary = summary
		}),
	)
	reg.Register(tool)
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "e1", ToolName: "fail", Args: raw(`{"x": 1}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	require.ErrorIs(t, err, errSentinel)
	assert.Equal(t, 1, afterCalls)
	assert.Equal(t, "e1", lastSummary.CallID)
	assert.Equal(t, "fail", lastSummary.ToolName)
	assert.ErrorIs(t, lastSummary.Error, errSentinel)
}

type testValidator struct {
	validateFn func(ctx context.Context, toolName string, argsJSON string) error
}

func (v *testValidator) Validate(ctx context.Context, toolName string, argsJSON string) error {
	if v.validateFn != nil {
		return v.validateFn(ctx, toolName, argsJSON)
	}
	return nil
}

func TestRegistry_Validator_BlocksExecution(t *testing.T) {
	type A struct {
		Query string `json:"query"`
	}
	type R struct{}
	var executed bool
	tool, err := NewTool("sql", "SQL", func(_ context.Context, _ A) (R, error) {
		executed = true
		return R{}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry(
		WithValidator(&testValidator{validateFn: func(_ context.Context, toolName string, argsJSON string) error {
			assert.Equal(t, "sql", toolName)
			if argsJSON == `{"query":"drop table users"}` {
				return errors.New("drop table detected")
			}
			return nil
		}}),
	)
	reg.Register(tool)
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "v1", ToolName: "sql", Args: raw(`{"query":"drop table users"}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	require.False(t, executed, "tool must not execute when validator rejects raw args")
	require.True(t, IsClientError(err))
	require.ErrorIs(t, err, ErrValidation)
	assert.Contains(t, err.Error(), "security validation failed")
	assert.Contains(t, err.Error(), "Please fix the arguments and try again")
}

func TestRegistry_Validator_PassesValidArgs(t *testing.T) {
	type A struct {
		Query string `json:"query"`
	}
	type R struct {
		OK bool `json:"ok"`
	}
	tool, err := NewTool("sql", "SQL", func(_ context.Context, a A) (R, error) {
		return R{OK: a.Query == "select 1"}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry(WithValidator(&testValidator{validateFn: func(_ context.Context, _ string, _ string) error {
		return nil
	}}))
	reg.Register(tool)
	var out R
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "v2", ToolName: "sql", Args: raw(`{"query":"select 1"}`)},
		func(c Chunk) error {
			out = c.RawData.(R)
			return nil
		},
	)
	require.NoError(t, err)
	assert.True(t, out.OK)
}

func TestRegistry_Validator_NilByDefault(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	type R struct {
		N int `json:"n"`
	}
	tool, err := NewTool("echo", "Echo", func(_ context.Context, a A) (R, error) {
		return R(a), nil
	})
	require.NoError(t, err)
	reg := NewRegistry()
	reg.Register(tool)
	var out R
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "v3", ToolName: "echo", Args: raw(`{"n":7}`)},
		func(c Chunk) error {
			out = c.RawData.(R)
			return nil
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 7, out.N)
}
