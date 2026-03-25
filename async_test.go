package toolsy_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/testutil"
)

func TestAsAsyncTool_ReturnsTaskID(t *testing.T) {
	base := &testutil.MockTool{
		NameVal: "heavy",
		ExecuteFn: func(_ context.Context, _ toolsy.RunContext, _ []byte, _ func(toolsy.Chunk) error) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base)

	var accepted toolsy.AsyncAccepted
	err := wrapped.Execute(context.Background(), toolsy.RunContext{}, []byte(`{}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if a, ok := c.RawData.(toolsy.AsyncAccepted); ok {
				accepted = a
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "accepted", accepted.Status)
	require.Len(t, accepted.TaskID, 32)
	require.Regexp(t, `^[a-f0-9]+$`, accepted.TaskID)
}

func TestAsAsyncTool_OnCompleteHook(t *testing.T) {
	var (
		done      sync.WaitGroup
		gotID     string
		gotChunks []toolsy.Chunk
		gotErr    error
	)
	done.Add(1)
	base := &testutil.MockTool{
		NameVal: "echo",
		ExecuteFn: func(_ context.Context, _ toolsy.RunContext, _ []byte, yield func(toolsy.Chunk) error) error {
			return yield(toolsy.Chunk{Event: toolsy.EventResult, RawData: "done"})
		},
	}
	wrapped := toolsy.AsAsyncTool(
		base,
		toolsy.WithOnComplete(func(_ context.Context, taskID string, chunks []toolsy.Chunk, err error) {
			gotID = taskID
			gotChunks = chunks
			gotErr = err
			done.Done()
		}),
	)

	err := wrapped.Execute(context.Background(), toolsy.RunContext{}, []byte(`{}`), func(_ toolsy.Chunk) error {
		return nil
	})
	require.NoError(t, err)
	done.Wait()
	require.Len(t, gotID, 32)
	require.Len(t, gotChunks, 1)
	require.Equal(t, "done", gotChunks[0].RawData)
	require.NoError(t, gotErr)
}

func TestAsAsyncTool_NilCallback(t *testing.T) {
	base := &testutil.MockTool{
		NameVal: "nop",
		ExecuteFn: func(context.Context, toolsy.RunContext, []byte, func(toolsy.Chunk) error) error {
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base)
	err := wrapped.Execute(
		context.Background(),
		toolsy.RunContext{},
		[]byte(`{}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
}

func TestAsAsyncTool_BaseError(t *testing.T) {
	wantErr := errors.New("base failed")
	var (
		done   sync.WaitGroup
		gotErr error
	)
	done.Add(1)
	base := &testutil.MockTool{
		NameVal: "fail",
		ExecuteFn: func(context.Context, toolsy.RunContext, []byte, func(toolsy.Chunk) error) error {
			return wantErr
		},
	}
	wrapped := toolsy.AsAsyncTool(
		base,
		toolsy.WithOnComplete(func(_ context.Context, _ string, _ []toolsy.Chunk, err error) {
			gotErr = err
			done.Done()
		}),
	)
	err := wrapped.Execute(
		context.Background(),
		toolsy.RunContext{},
		[]byte(`{}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.ErrorIs(t, gotErr, wantErr)
}

func TestAsAsyncTool_YieldError_NoGoroutine(t *testing.T) {
	yieldErr := errors.New("client closed stream")
	callbackCalled := false
	var callbackMu sync.Mutex
	base := &testutil.MockTool{
		NameVal: "slow",
		ExecuteFn: func(context.Context, toolsy.RunContext, []byte, func(toolsy.Chunk) error) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base, toolsy.WithOnComplete(func(context.Context, string, []toolsy.Chunk, error) {
		callbackMu.Lock()
		callbackCalled = true
		callbackMu.Unlock()
	}))
	err := wrapped.Execute(context.Background(), toolsy.RunContext{}, []byte(`{}`), func(toolsy.Chunk) error {
		return yieldErr
	})
	require.ErrorIs(t, err, toolsy.ErrStreamAborted)
	require.ErrorIs(t, err, yieldErr)
	time.Sleep(80 * time.Millisecond)
	callbackMu.Lock()
	ok := callbackCalled
	callbackMu.Unlock()
	require.False(t, ok, "OnComplete must not be called when yield returns error")
}

func TestAsAsyncTool_CanceledContext_NoAcceptedNoGoroutine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	callbackCalled := false
	var callbackMu sync.Mutex
	base := &testutil.MockTool{
		NameVal: "nop",
		ExecuteFn: func(context.Context, toolsy.RunContext, []byte, func(toolsy.Chunk) error) error {
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base, toolsy.WithOnComplete(func(context.Context, string, []toolsy.Chunk, error) {
		callbackMu.Lock()
		callbackCalled = true
		callbackMu.Unlock()
	}))
	err := wrapped.Execute(ctx, toolsy.RunContext{}, []byte(`{}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	time.Sleep(50 * time.Millisecond)
	callbackMu.Lock()
	ok := callbackCalled
	callbackMu.Unlock()
	require.False(t, ok, "OnComplete must not be called when context is already canceled")
}

// TestAsAsyncTool_ContextCanceledBeforeYield_NoBackground verifies that a second ctx.Err() check
// immediately before yield(accepted) ensures "cancelled before accepted => no accepted, no background".
// We cancel context from another goroutine shortly after Execute starts; when we get [context.Canceled],
// OnComplete must never be called.
func TestAsAsyncTool_ContextCanceledBeforeYield_NoBackground(t *testing.T) {
	var onCompleteCalled int32
	base := &testutil.MockTool{
		NameVal: "nop",
		ExecuteFn: func(context.Context, toolsy.RunContext, []byte, func(toolsy.Chunk) error) error {
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base, toolsy.WithOnComplete(func(context.Context, string, []toolsy.Chunk, error) {
		atomic.StoreInt32(&onCompleteCalled, 1)
	}))
	for range 30 {
		atomic.StoreInt32(&onCompleteCalled, 0)
		ctx, cancel := context.WithCancel(context.Background())
		var err error
		done := make(chan struct{})
		go func() {
			err = wrapped.Execute(ctx, toolsy.RunContext{}, []byte(`{}`), func(toolsy.Chunk) error { return nil })
			close(done)
		}()
		time.Sleep(1 * time.Millisecond)
		cancel()
		<-done
		if errors.Is(err, context.Canceled) {
			time.Sleep(50 * time.Millisecond)
			require.Zero(
				t,
				atomic.LoadInt32(&onCompleteCalled),
				"OnComplete must not run when Execute returned context.Canceled (cancel before yield)",
			)
			return
		}
	}
	t.Skip(
		"context was not canceled before yield in 30 runs; second check is still exercised by TestAsAsyncTool_CanceledContext_NoAcceptedNoGoroutine",
	)
}

func TestAsAsyncTool_PanicInBase_CallbackGetsSystemError(t *testing.T) {
	var (
		done   sync.WaitGroup
		gotErr error
	)
	done.Add(1)
	base := &testutil.MockTool{
		NameVal: "panic_tool",
		ExecuteFn: func(context.Context, toolsy.RunContext, []byte, func(toolsy.Chunk) error) error {
			panic("intentional panic for test")
		},
	}
	wrapped := toolsy.AsAsyncTool(
		base,
		toolsy.WithOnComplete(func(_ context.Context, _ string, _ []toolsy.Chunk, err error) {
			gotErr = err
			done.Done()
		}),
	)
	err := wrapped.Execute(
		context.Background(),
		toolsy.RunContext{},
		[]byte(`{}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.True(t, toolsy.IsSystemError(gotErr), "OnComplete must receive SystemError when base panics")
}

func TestAsAsyncTool_OnCompletePanic_DoesNotCrashProcess(t *testing.T) {
	base := &testutil.MockTool{
		NameVal: "ok",
		ExecuteFn: func(context.Context, toolsy.RunContext, []byte, func(toolsy.Chunk) error) error {
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(
		base,
		toolsy.WithOnComplete(func(_ context.Context, _ string, _ []toolsy.Chunk, _ error) {
			panic("callback panic for test")
		}),
	)
	err := wrapped.Execute(
		context.Background(),
		toolsy.RunContext{},
		[]byte(`{}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	time.Sleep(80 * time.Millisecond) // allow goroutine to run and OnComplete to panic and be recovered
	// If we reach here without the test process crashing, the callback panic was isolated.
}

// timeoutMock implements Tool and ToolMetadata so AsAsyncTool applies per-tool timeout in the background.
type timeoutMock struct {
	*testutil.MockTool

	timeout time.Duration
}

func (m *timeoutMock) Timeout() time.Duration     { return m.timeout }
func (m *timeoutMock) Tags() []string             { return nil }
func (m *timeoutMock) Version() string            { return "" }
func (m *timeoutMock) IsDangerous() bool          { return false }
func (m *timeoutMock) IsReadOnly() bool           { return false }
func (m *timeoutMock) RequiresConfirmation() bool { return false }
func (m *timeoutMock) Sensitivity() string        { return "" }

func TestAsAsyncTool_BackgroundTimeout(t *testing.T) {
	var (
		done   sync.WaitGroup
		gotErr error
	)
	done.Add(1)
	base := &timeoutMock{
		MockTool: &testutil.MockTool{
			NameVal: "slow",
			ExecuteFn: func(ctx context.Context, _ toolsy.RunContext, _ []byte, _ func(toolsy.Chunk) error) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(50 * time.Millisecond):
					return nil
				}
			},
		},
		timeout: 5 * time.Millisecond,
	}
	wrapped := toolsy.AsAsyncTool(
		base,
		toolsy.WithOnComplete(func(_ context.Context, _ string, _ []toolsy.Chunk, err error) {
			gotErr = err
			done.Done()
		}),
	)
	err := wrapped.Execute(
		context.Background(),
		toolsy.RunContext{},
		[]byte(`{}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.Error(t, gotErr)
	require.ErrorIs(t, gotErr, context.DeadlineExceeded)
}

// TestAsAsyncTool_RegistryTimeoutMinimumHierarchy ensures effective timeout is min(registry, tool).
// When tool has longer timeout than registry, background run gets registry timeout and times out.
func TestAsAsyncTool_RegistryTimeoutMinimumHierarchy(t *testing.T) {
	var (
		done   sync.WaitGroup
		gotErr error
	)
	done.Add(1)
	base := &timeoutMock{
		MockTool: &testutil.MockTool{
			NameVal: "long_tool",
			ExecuteFn: func(ctx context.Context, _ toolsy.RunContext, _ []byte, _ func(toolsy.Chunk) error) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(200 * time.Millisecond):
					return nil
				}
			},
		},
		timeout: 200 * time.Millisecond, // tool wants 200ms
	}
	reg := toolsy.NewRegistry(toolsy.WithDefaultTimeout(25 * time.Millisecond))
	reg.Register(
		toolsy.AsAsyncTool(base, toolsy.WithOnComplete(func(_ context.Context, _ string, _ []toolsy.Chunk, err error) {
			gotErr = err
			done.Done()
		})),
	)
	err := reg.Execute(
		context.Background(),
		toolsy.ToolCall{ID: "1", ToolName: "long_tool", Args: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.Error(t, gotErr)
	require.ErrorIs(
		t,
		gotErr,
		context.DeadlineExceeded,
		"effective timeout must be min(registry 25ms, tool 200ms) = 25ms",
	)
}

// TestAsAsyncTool_RegistryDefaultTimeoutAppliesToBackground ensures that when run via Registry,
// the registry's effective timeout is applied to the async background job (not only to the accepted phase).
func TestAsAsyncTool_RegistryDefaultTimeoutAppliesToBackground(t *testing.T) {
	var (
		done   sync.WaitGroup
		gotErr error
	)
	done.Add(1)
	base := &testutil.MockTool{
		NameVal: "slow_via_registry",
		ExecuteFn: func(ctx context.Context, _ toolsy.RunContext, _ []byte, _ func(toolsy.Chunk) error) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		},
	}
	reg := toolsy.NewRegistry(toolsy.WithDefaultTimeout(15 * time.Millisecond))
	reg.Register(
		toolsy.AsAsyncTool(base, toolsy.WithOnComplete(func(_ context.Context, _ string, _ []toolsy.Chunk, err error) {
			gotErr = err
			done.Done()
		})),
	)
	err := reg.Execute(
		context.Background(),
		toolsy.ToolCall{ID: "1", ToolName: "slow_via_registry", Args: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.Error(t, gotErr)
	require.ErrorIs(t, gotErr, context.DeadlineExceeded)
}

func TestAsAsyncTool_ShutdownWaitsForBackground(t *testing.T) {
	workDone := make(chan struct{})
	base := &testutil.MockTool{
		NameVal: "slow_async",
		ExecuteFn: func(context.Context, toolsy.RunContext, []byte, func(toolsy.Chunk) error) error {
			time.Sleep(50 * time.Millisecond)
			close(workDone)
			return nil
		},
	}
	asyncTool := toolsy.AsAsyncTool(base)
	reg := toolsy.NewRegistry()
	reg.Register(asyncTool)
	ctx := context.Background()
	err := reg.Execute(
		ctx,
		toolsy.ToolCall{ID: "1", ToolName: "slow_async", Args: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = reg.Shutdown(shutdownCtx)
	require.NoError(t, err)
	select {
	case <-workDone:
		// Background job completed; Shutdown waited for it (tracked semantics).
	default:
		t.Fatal("Shutdown must wait for AsAsyncTool background job when executed via Registry")
	}
}

func TestAsAsyncTool_ConcurrencySlotHeldUntilBackgroundDone(t *testing.T) {
	// Registry with max concurrency 1: only one execution slot. Async tool holds the slot until background finishes.
	secondStarted := make(chan struct{})
	firstDone := make(chan struct{})
	base := &testutil.MockTool{
		NameVal: "holds_slot",
		ExecuteFn: func(context.Context, toolsy.RunContext, []byte, func(toolsy.Chunk) error) error {
			time.Sleep(80 * time.Millisecond)
			close(firstDone)
			return nil
		},
	}
	reg := toolsy.NewRegistry(toolsy.WithMaxConcurrency(1))
	reg.Register(toolsy.AsAsyncTool(base))
	ctx := context.Background()
	err := reg.Execute(
		ctx,
		toolsy.ToolCall{ID: "1", ToolName: "holds_slot", Args: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	// Async returned immediately; background still running. Second call should block until first background completes.
	go func() {
		_ = reg.Execute(
			ctx,
			toolsy.ToolCall{ID: "2", ToolName: "holds_slot", Args: []byte(`{}`)},
			func(toolsy.Chunk) error { return nil },
		)
		close(secondStarted)
	}()
	select {
	case <-secondStarted:
		t.Fatal("second Execute must not start until first async background job releases the slot")
	case <-firstDone:
		// First background finished; second may proceed
	case <-time.After(3 * time.Second):
		t.Fatal("first background job did not complete in time")
	}
	select {
	case <-secondStarted:
		// Second finished after first released the slot
	case <-time.After(2 * time.Second):
		t.Fatal("second Execute did not complete after first released the slot")
	}
}
