package toolsy

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noSleepRetryOption() RetryOption {
	return func(c *retryConfig) {
		c.sleep = func(context.Context, time.Duration) error { return nil }
	}
}

func TestWithIdempotentRetry_ReadOnlyRetriesAndSucceeds(t *testing.T) {
	var attempts atomic.Int64
	inner := newMiddlewareMinTool(
		"readonly_retry",
		func(_ context.Context, _ RunContext, _ ToolInput, yield func(Chunk) error) error {
			attempt := attempts.Add(1)
			if attempt < 3 {
				return ErrTimeout
			}
			return yield(Chunk{Event: EventResult, Data: []byte("ok"), MimeType: MimeTypeText})
		},
	)
	inner.manifest.Metadata = map[string]any{"read_only": true}

	wrapped := WithIdempotentRetry(noSleepRetryOption())(inner)

	var got string
	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(c Chunk) error {
		got = string(c.Data)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), attempts.Load())
	assert.Equal(t, "ok", got)
}

func TestWithIdempotentRetry_NonReadOnlyNoRetry(t *testing.T) {
	var attempts atomic.Int64
	inner := newMiddlewareMinTool(
		"write_tool",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			attempts.Add(1)
			return ErrTimeout
		},
	)
	wrapped := WithIdempotentRetry(noSleepRetryOption())(inner)

	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error {
		return nil
	})
	require.ErrorIs(t, err, ErrTimeout)
	assert.Equal(t, int64(1), attempts.Load())
}

func TestWithIdempotentRetry_NoRetryAfterChunkDelivery(t *testing.T) {
	var attempts atomic.Int64
	inner := newMiddlewareMinTool(
		"streaming_readonly",
		func(_ context.Context, _ RunContext, _ ToolInput, yield func(Chunk) error) error {
			attempts.Add(1)
			if err := yield(Chunk{Event: EventProgress, Data: []byte("p"), MimeType: MimeTypeText}); err != nil {
				return err
			}
			return ErrTimeout
		},
	)
	inner.manifest.Metadata = map[string]any{"read_only": true}
	wrapped := WithIdempotentRetry(noSleepRetryOption())(inner)

	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error {
		return nil
	})
	require.ErrorIs(t, err, ErrTimeout)
	assert.Equal(t, int64(1), attempts.Load())
}

func TestWithIdempotentRetry_DoesNotRetrySuspend(t *testing.T) {
	var attempts atomic.Int64
	inner := newMiddlewareMinTool(
		"suspend",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			attempts.Add(1)
			return ErrSuspend
		},
	)
	inner.manifest.Metadata = map[string]any{"read_only": true}
	wrapped := WithIdempotentRetry(noSleepRetryOption())(inner)

	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error {
		return nil
	})
	require.ErrorIs(t, err, ErrSuspend)
	assert.Equal(t, int64(1), attempts.Load())
}

func TestWithIdempotentRetry_RetriesRetryableClientError(t *testing.T) {
	var attempts atomic.Int64
	inner := newMiddlewareMinTool(
		"client_retryable",
		func(_ context.Context, _ RunContext, _ ToolInput, yield func(Chunk) error) error {
			attempt := attempts.Add(1)
			if attempt == 1 {
				return &ClientError{Reason: "temporary upstream issue", Retryable: true}
			}
			return yield(Chunk{Event: EventResult, Data: []byte("done"), MimeType: MimeTypeText})
		},
	)
	inner.manifest.Metadata = map[string]any{"read_only": true}
	wrapped := WithIdempotentRetry(noSleepRetryOption())(inner)

	var data string
	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(c Chunk) error {
		data = string(c.Data)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), attempts.Load())
	assert.Equal(t, "done", data)
}

func TestWithIdempotentRetry_RespectsMaxAttempts(t *testing.T) {
	var attempts atomic.Int64
	inner := newMiddlewareMinTool(
		"max_attempts",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			attempts.Add(1)
			return ErrTimeout
		},
	)
	inner.manifest.Metadata = map[string]any{"read_only": true}
	wrapped := WithIdempotentRetry(
		WithRetryMaxAttempts(2),
		noSleepRetryOption(),
	)(inner)

	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error {
		return nil
	})
	require.ErrorIs(t, err, ErrTimeout)
	assert.Equal(t, int64(2), attempts.Load())
}

func TestWithIdempotentRetry_BackoffRespectsContextCancellation(t *testing.T) {
	var attempts atomic.Int64
	inner := newMiddlewareMinTool(
		"retry_ctx_cancel",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			attempts.Add(1)
			return ErrTimeout
		},
	)
	inner.manifest.Metadata = map[string]any{"read_only": true}

	sleepStarted := make(chan struct{})
	wrapped := WithIdempotentRetry(
		WithRetryMaxAttempts(3),
		func(c *retryConfig) {
			c.sleep = func(ctx context.Context, _ time.Duration) error {
				close(sleepStarted)
				<-ctx.Done()
				return ctx.Err()
			}
		},
	)(inner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- wrapped.Execute(ctx, RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error { return nil })
	}()

	<-sleepStarted
	cancel()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for execute to return after context cancellation")
	}
	assert.Equal(t, int64(1), attempts.Load())
}

func TestDefaultShouldRetry_DoesNotRetryContextCanceled(t *testing.T) {
	assert.False(t, defaultShouldRetry(context.Canceled))
}

func TestDefaultShouldRetry_UnknownErrorNotRetried(t *testing.T) {
	assert.False(t, defaultShouldRetry(errors.New("unknown")))
}
