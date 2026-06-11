package toolsy_test

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/testutil"
)

func decodeAccepted(t *testing.T, c toolsy.Chunk) toolsy.AsyncAccepted {
	t.Helper()
	require.Equal(t, toolsy.EventResult, c.Event)
	require.Equal(t, toolsy.MimeTypeJSON, c.MimeType)
	var accepted toolsy.AsyncAccepted
	require.NoError(t, json.Unmarshal(c.Data, &accepted))
	return accepted
}

func TestAsAsyncTool_ReturnsTaskID(t *testing.T) {
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "heavy", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, _ func(toolsy.Chunk) error) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base)

	var accepted toolsy.AsyncAccepted
	err := wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(c toolsy.Chunk) error {
			accepted = decodeAccepted(t, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, "accepted", accepted.Status)
	require.Len(t, accepted.TaskID, 32)
	require.True(t, regexp.MustCompile(`^[a-f0-9]+$`).MatchString(accepted.TaskID))
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
		ManifestVal: toolsy.ToolManifest{Name: "echo", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: []byte(`"done"`), MimeType: toolsy.MimeTypeJSON})
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

	err := wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(_ toolsy.Chunk) error {
			return nil
		},
	)
	require.NoError(t, err)
	done.Wait()
	require.Len(t, gotID, 32)
	require.Len(t, gotChunks, 1)
	require.Equal(t, []byte(`"done"`), gotChunks[0].Data)
	require.NoError(t, gotErr)
}

func TestAsAsyncTool_NilCallback(t *testing.T) {
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "nop", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base)
	err := wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
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
		ManifestVal: toolsy.ToolManifest{Name: "fail", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error {
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
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.Error(t, gotErr)
	te, ok := toolsy.AsToolError(gotErr)
	require.True(t, ok)
	assert.Equal(t, toolsy.CodeInternal, te.Code)
	require.ErrorIs(t, te.Unwrap(), wantErr)
}

func TestAsAsyncTool_YieldError_NoGoroutine(t *testing.T) {
	yieldErr := errors.New("client closed stream")
	callbackCalled := false
	var callbackMu sync.Mutex
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "slow", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base, toolsy.WithOnComplete(func(context.Context, string, []toolsy.Chunk, error) {
		callbackMu.Lock()
		callbackCalled = true
		callbackMu.Unlock()
	}))
	err := wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error {
			return yieldErr
		},
	)
	require.ErrorIs(t, err, toolsy.ErrStreamAborted)
	require.ErrorIs(t, err, yieldErr)
	time.Sleep(80 * time.Millisecond)
	callbackMu.Lock()
	ok := callbackCalled
	callbackMu.Unlock()
	require.False(t, ok)
}

func TestAsAsyncTool_CanceledContext_NoAcceptedNoGoroutine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	callbackCalled := false
	var callbackMu sync.Mutex
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "nop", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base, toolsy.WithOnComplete(func(context.Context, string, []toolsy.Chunk, error) {
		callbackMu.Lock()
		callbackCalled = true
		callbackMu.Unlock()
	}))
	err := wrapped.Execute(
		ctx,
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	time.Sleep(50 * time.Millisecond)
	callbackMu.Lock()
	ok := callbackCalled
	callbackMu.Unlock()
	require.False(t, ok)
}

func TestAsAsyncTool_PanicInBase_CallbackGetsInternalToolError(t *testing.T) {
	var (
		done   sync.WaitGroup
		gotErr error
	)
	done.Add(1)
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "panic_tool", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error {
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
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	te, ok := toolsy.AsToolError(gotErr)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
}

func TestAsAsyncTool_OnCompletePanic_DoesNotCrashProcess(t *testing.T) {
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "ok", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base, toolsy.WithOnComplete(func(context.Context, string, []toolsy.Chunk, error) {
		panic("callback panic")
	}))
	err := wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
}

func TestAsAsyncTool_MultipleAcceptedCallsDoNotBlockRegistry(t *testing.T) {
	type A struct{}
	type R struct{}
	block := make(chan struct{})
	base, err := toolsy.NewTool(
		"slow_async",
		"slow async",
		func(_ context.Context, _ *toolsy.RunEnv, _ A) (R, error) {
			<-block
			return R{}, nil
		},
	)
	require.NoError(t, err)

	asyncTool := toolsy.AsAsyncTool(base)
	reg, err := toolsy.NewRegistryBuilder().Add(asyncTool).Build()
	require.NoError(t, err)

	err = reg.Execute(
		context.Background(),
		toolsy.ToolCall{ToolName: "slow_async", Input: toolsy.ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)

	err = reg.Execute(
		context.Background(),
		toolsy.ToolCall{ToolName: "slow_async", Input: toolsy.ToolInput{CallID: "2", ArgsJSON: []byte(`{}`)}},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)

	close(block)
}

func TestAsAsyncTool_RegistryShutdownWaitsForBackground(t *testing.T) {
	type A struct{}
	type R struct{}

	block := make(chan struct{})
	started := atomic.Bool{}
	base, err := toolsy.NewTool("worker", "worker", func(_ context.Context, _ *toolsy.RunEnv, _ A) (R, error) {
		started.Store(true)
		<-block
		return R{}, nil
	})
	require.NoError(t, err)

	reg, err := toolsy.NewRegistryBuilder().Add(toolsy.AsAsyncTool(base)).Build()
	require.NoError(t, err)

	err = reg.Execute(
		context.Background(),
		toolsy.ToolCall{ToolName: "worker", Input: toolsy.ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	require.Eventually(t, started.Load, time.Second, 10*time.Millisecond)

	done := make(chan error, 1)
	go func() {
		done <- reg.Shutdown(context.Background())
	}()
	select {
	case <-time.After(50 * time.Millisecond):
	case err := <-done:
		require.NoError(t, err)
		t.Fatalf("shutdown returned before background completed")
	}
	close(block)
	require.NoError(t, <-done)
}

func TestAsAsyncTool_InputCloneRace(t *testing.T) {
	const original = `{"v":1}`
	var (
		gotArgs string
		done    sync.WaitGroup
	)
	done.Add(1)

	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "race", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(_ context.Context, _ *toolsy.RunEnv, input toolsy.ToolInput, _ func(toolsy.Chunk) error) error {
			gotArgs = string(input.ArgsJSON)
			done.Done()
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base)
	input := toolsy.ToolInput{ArgsJSON: []byte(original)}
	err := wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		input,
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	input.ArgsJSON[0] = 'X'
	done.Wait()
	require.Equal(t, original, gotArgs)
}

func TestAsAsyncTool_BackgroundTimeout(t *testing.T) {
	t.Run("live_parent", func(t *testing.T) {
		testBackgroundTimeout(t, false)
	})
	t.Run("canceled_parent_after_accept", func(t *testing.T) {
		testBackgroundTimeout(t, true)
	})
}

func testBackgroundTimeout(t *testing.T, cancelParentAfterAccept bool) {
	t.Helper()
	block := make(chan struct{})
	var (
		done   sync.WaitGroup
		gotErr error
	)
	done.Add(1)

	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "slow", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(ctx context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, _ func(toolsy.Chunk) error) error {
			select {
			case <-block:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	wrapped := toolsy.AsAsyncTool(
		base,
		toolsy.WithBackgroundTimeout(30*time.Millisecond),
		toolsy.WithOnComplete(func(_ context.Context, _ string, _ []toolsy.Chunk, err error) {
			gotErr = err
			done.Done()
		}),
	)
	parentCtx := context.Background()
	var cancel context.CancelFunc
	if cancelParentAfterAccept {
		parentCtx, cancel = context.WithCancel(parentCtx)
	}
	err := wrapped.Execute(
		parentCtx,
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error {
			if cancelParentAfterAccept {
				cancel()
			}
			return nil
		},
	)
	require.NoError(t, err)
	done.Wait()
	require.ErrorIs(t, gotErr, context.DeadlineExceeded)
	close(block)
}

func TestAsAsyncTool_MaxCollectedChunks_Exceeded(t *testing.T) {
	const limit = 3
	var (
		done      sync.WaitGroup
		gotChunks []toolsy.Chunk
		gotErr    error
	)
	done.Add(1)
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "stream", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			for range limit + 2 {
				_ = yield(toolsy.Chunk{Event: toolsy.EventResult, Data: []byte(`"x"`), MimeType: toolsy.MimeTypeJSON})
			}
			return nil // ignores yield errors; onComplete must still see limit via backgroundYieldErr
		},
	}
	wrapped := toolsy.AsAsyncTool(
		base,
		toolsy.WithMaxCollectedChunks(limit),
		toolsy.WithOnComplete(func(_ context.Context, _ string, chunks []toolsy.Chunk, err error) {
			gotChunks = chunks
			gotErr = err
			done.Done()
		}),
	)
	err := wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.Error(t, gotErr)
	require.ErrorIs(t, gotErr, toolsy.ErrAsyncCollectedLimitExceeded)
	require.Len(t, gotChunks, limit)
}

func TestAsAsyncTool_MaxCollectedChunks_DefaultCap(t *testing.T) {
	const overDefault = toolsy.DefaultMaxCollectedChunks + 1
	var (
		done   sync.WaitGroup
		gotErr error
	)
	done.Add(1)
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "stream_default", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			for range overDefault {
				_ = yield(toolsy.Chunk{Event: toolsy.EventResult, Data: []byte(`"x"`), MimeType: toolsy.MimeTypeJSON})
			}
			return nil
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
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.ErrorIs(t, gotErr, toolsy.ErrAsyncCollectedLimitExceeded)
}

func TestAsAsyncTool_InvalidChunk_StopsBackgroundCollect(t *testing.T) {
	var (
		done      sync.WaitGroup
		gotChunks []toolsy.Chunk
		gotErr    error
	)
	done.Add(1)
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "invalid_chunk", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			for range 3 {
				_ = yield(toolsy.Chunk{Event: "not-a-valid-event", Data: []byte(`"x"`), MimeType: toolsy.MimeTypeJSON})
			}
			return nil // ignores yield errors; onComplete must still see validateChunk via backgroundYieldErr
		},
	}
	wrapped := toolsy.AsAsyncTool(
		base,
		toolsy.WithOnComplete(func(_ context.Context, _ string, chunks []toolsy.Chunk, err error) {
			gotChunks = chunks
			gotErr = err
			done.Done()
		}),
	)
	err := wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.Error(t, gotErr)
	te, ok := toolsy.AsToolError(gotErr)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
	require.Contains(t, te.Err.Error(), "unsupported chunk event")
	require.Empty(t, gotChunks)
}
