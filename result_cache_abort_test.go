package toolsy

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResultCacheIgnoredConsumerAbortCannotPersistSuccess(t *testing.T) {
	// Arrange: a misbehaving producer ignores both callback errors.
	cache := mustResultCache(t, constantPartition)
	call := PreparedCall{
		Manifest: ToolManifest{Name: "abort", Idempotent: true},
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}
	abort := errors.New("consumer stopped")
	calls, deliveries := 0, 0
	invoke := func(yield func(Chunk) error) error {
		calls++
		_ = yield(Chunk{Event: EventProgress})
		_ = yield(Chunk{Event: EventResult, Data: []byte(`"ok"`), MimeType: MimeTypeJSON, TypedResult: "ok"})
		return nil
	}
	// Act: abort progress, then call again with an accepting consumer.
	err := cache.ExecutePrepared(context.Background(), call, invoke, func(Chunk) error { deliveries++; return abort })
	require.ErrorIs(t, err, abort)
	require.Equal(t, 1, deliveries)
	err = cache.ExecutePrepared(context.Background(), call, invoke, func(Chunk) error { return nil })
	// Assert: the aborted attempt neither delivered a result nor populated replay.
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}

func TestResultCacheIgnoredPauseOrCancellationCannotPersistSuccess(t *testing.T) {
	for _, cancelCall := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancelCall), func(t *testing.T) {
			// Arrange: producer ignores control/cancellation and claims success.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cache := mustResultCache(t, constantPartition)
			call := PreparedCall{
				Manifest: ToolManifest{Name: "interrupt", Idempotent: true},
				Input:    ToolInput{ArgsJSON: []byte(`{}`)},
			}
			calls, results := 0, 0
			invoke := func(yield func(Chunk) error) error {
				calls++
				if cancelCall {
					_ = yield(Chunk{Event: EventProgress})
					cancel()
				} else {
					_ = YieldControl(yield, &PauseSignal{Reason: "approval"})
				}
				_ = yield(Chunk{Event: EventResult, Data: []byte(`"ok"`), MimeType: MimeTypeJSON, TypedResult: "ok"})
				return nil
			}
			// Act: ignored interruption, then a clean authorized invocation.
			err := cache.ExecutePrepared(ctx, call, invoke, func(c Chunk) error {
				if c.Event == EventResult {
					results++
				}
				return nil
			})
			if cancelCall {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, ErrPause)
			}
			require.NoError(t, cache.ExecutePrepared(context.Background(), call, func(yield func(Chunk) error) error {
				calls++
				return yield(Chunk{Event: EventResult, Data: []byte(`"ok"`), MimeType: MimeTypeJSON, TypedResult: "ok"})
			}, func(Chunk) error { return nil }))
			// Assert: no completed entry or result escaped the interrupted invocation.
			assert.Equal(t, 2, calls)
			assert.Zero(t, results)
		})
	}
}
