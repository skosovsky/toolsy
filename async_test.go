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
		ExecuteFn: func(_ context.Context, _ toolsy.RunContext, _ toolsy.ToolInput, _ func(toolsy.Chunk) error) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base)

	var accepted toolsy.AsyncAccepted
	err := wrapped.Execute(
		context.Background(),
		toolsy.RunContext{},
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
		ExecuteFn: func(_ context.Context, _ toolsy.RunContext, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
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
		toolsy.RunContext{},
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
		ExecuteFn: func(context.Context, toolsy.RunContext, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base)
	err := wrapped.Execute(
		context.Background(),
		toolsy.RunContext{},
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
		ExecuteFn: func(context.Context, toolsy.RunContext, toolsy.ToolInput, func(toolsy.Chunk) error) error {
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
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
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
		ManifestVal: toolsy.ToolManifest{Name: "slow", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(context.Context, toolsy.RunContext, toolsy.ToolInput, func(toolsy.Chunk) error) error {
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
		toolsy.RunContext{},
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
		ExecuteFn: func(context.Context, toolsy.RunContext, toolsy.ToolInput, func(toolsy.Chunk) error) error {
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
		toolsy.RunContext{},
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

func TestAsAsyncTool_PanicInBase_CallbackGetsSystemError(t *testing.T) {
	var (
		done   sync.WaitGroup
		gotErr error
	)
	done.Add(1)
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "panic_tool", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(context.Context, toolsy.RunContext, toolsy.ToolInput, func(toolsy.Chunk) error) error {
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
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.True(t, toolsy.IsSystemError(gotErr))
}

func TestAsAsyncTool_OnCompletePanic_DoesNotCrashProcess(t *testing.T) {
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "ok", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(context.Context, toolsy.RunContext, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return nil
		},
	}
	wrapped := toolsy.AsAsyncTool(base, toolsy.WithOnComplete(func(context.Context, string, []toolsy.Chunk, error) {
		panic("callback panic")
	}))
	err := wrapped.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
}

func TestAsAsyncTool_RegistryHoldsSlotUntilBackgroundDone(t *testing.T) {
	type A struct{}
	type R struct{}
	block := make(chan struct{})
	base, err := toolsy.NewTool(
		"slow_async",
		"slow async",
		func(_ context.Context, _ toolsy.RunContext, _ A) (R, error) {
			<-block
			return R{}, nil
		},
	)
	require.NoError(t, err)

	asyncTool := toolsy.AsAsyncTool(base)
	reg, err := toolsy.NewRegistryBuilder(
		toolsy.WithMaxConcurrency(1),
	).Add(asyncTool).Build()
	require.NoError(t, err)

	err = reg.Execute(
		context.Background(),
		toolsy.ToolCall{ToolName: "slow_async", Input: toolsy.ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = reg.Execute(
		ctx,
		toolsy.ToolCall{ToolName: "slow_async", Input: toolsy.ToolInput{CallID: "2", ArgsJSON: []byte(`{}`)}},
		func(toolsy.Chunk) error { return nil },
	)
	require.ErrorIs(t, err, toolsy.ErrTimeout)

	close(block)
}

func TestAsAsyncTool_RegistryShutdownWaitsForBackground(t *testing.T) {
	type A struct{}
	type R struct{}

	block := make(chan struct{})
	started := atomic.Bool{}
	base, err := toolsy.NewTool("worker", "worker", func(_ context.Context, _ toolsy.RunContext, _ A) (R, error) {
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
