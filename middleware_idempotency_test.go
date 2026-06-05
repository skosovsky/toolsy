package toolsy

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithIdempotency_ReturnsCachedResult(t *testing.T) {
	var calls atomic.Int32
	tool, err := NewTool("echo", "Echo", func(_ context.Context, _ *RunEnv, _ struct {
		N int `json:"n"`
	}) (struct {
		N int `json:"n"`
	}, error) {
		calls.Add(1)
		return struct {
			N int `json:"n"`
		}{N: 42}, nil
	}, WithIdempotent())
	require.NoError(t, err)

	store := NewMemoryIdempotencyStore()
	reg, err := NewRegistryBuilder().
		Use(WithIdempotency(store, nil)).
		Add(tool).
		Build()
	require.NoError(t, err)

	call := ToolCall{
		ToolName: "echo",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":1}`)},
	}
	var first, second string
	err = reg.Execute(context.Background(), call, func(c Chunk) error {
		first = string(c.Data)
		return nil
	})
	require.NoError(t, err)
	err = reg.Execute(context.Background(), call, func(c Chunk) error {
		second = string(c.Data)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, int32(1), calls.Load())
}

func TestWithIdempotency_SkipsNonIdempotentTools(t *testing.T) {
	var calls atomic.Int32
	tool, err := NewTool("mut", "Mut", func(_ context.Context, _ *RunEnv, _ struct{}) (struct{}, error) {
		calls.Add(1)
		return struct{}{}, nil
	})
	require.NoError(t, err)

	store := NewMemoryIdempotencyStore()
	reg, err := NewRegistryBuilder().
		Use(WithIdempotency(store, nil)).
		Add(tool).
		Build()
	require.NoError(t, err)

	call := ToolCall{ToolName: "mut", Input: ToolInput{ArgsJSON: []byte(`{}`)}}
	require.NoError(t, reg.Execute(context.Background(), call, func(Chunk) error { return nil }))
	require.NoError(t, reg.Execute(context.Background(), call, func(Chunk) error { return nil }))
	assert.Equal(t, int32(2), calls.Load())
}

func TestWithIdempotency_WithAsyncTool_CachesFinalResult(t *testing.T) {
	var calls atomic.Int32

	base, err := NewTool("async_echo", "Async echo", func(_ context.Context, _ *RunEnv, _ struct {
		N int `json:"n"`
	}) (struct {
		N int `json:"n"`
	}, error) {
		calls.Add(1)
		return struct {
			N int `json:"n"`
		}{N: 99}, nil
	}, WithIdempotent())
	require.NoError(t, err)

	store := NewMemoryIdempotencyStore()
	call := ToolCall{
		ToolName: "async_echo",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":1}`)},
	}

	var (
		secondChunks []Chunk
		secondSync   string
	)
	for i := range 2 {
		var (
			done         sync.WaitGroup
			onCompleteCh []Chunk
		)
		done.Add(1)
		asyncWrapped := AsAsyncTool(base, WithOnComplete(func(_ context.Context, _ string, chunks []Chunk, _ error) {
			onCompleteCh = append([]Chunk(nil), chunks...)
			done.Done()
		}))
		reg, buildErr := NewRegistryBuilder().
			Use(WithIdempotency(store, nil)).
			Add(asyncWrapped).
			Build()
		require.NoError(t, buildErr)

		var accepted AsyncAccepted
		var syncPayload string
		err = reg.Execute(context.Background(), call, func(c Chunk) error {
			syncPayload = string(c.Data)
			require.NoError(t, json.Unmarshal(c.Data, &accepted))
			require.Equal(t, "accepted", accepted.Status)
			return nil
		})
		require.NoError(t, err)
		done.Wait()
		if i == 1 {
			secondChunks = onCompleteCh
			secondSync = syncPayload
		}
	}
	assert.NotEqual(t, `{"n":99}`, secondSync,
		"idempotency must not return cached final result on sync path")
	assert.Equal(t, int32(1), calls.Load(),
		"base tool must run once; second call served from idempotency cache in background")
	require.Len(t, secondChunks, 1)
	assert.JSONEq(t, `{"n":99}`, string(secondChunks[0].Data))
}

func TestManualIdempotencyWrapAsync_CachesAcceptedOnSyncPath(t *testing.T) {
	var calls atomic.Int32
	base, err := NewTool("bad_wrap", "Bad wrap", func(_ context.Context, _ *RunEnv, _ struct{}) (struct {
		OK bool `json:"ok"`
	}, error) {
		calls.Add(1)
		return struct {
			OK bool `json:"ok"`
		}{OK: true}, nil
	}, WithIdempotent())
	require.NoError(t, err)

	store := NewMemoryIdempotencyStore()
	// Unsupported: idempotency outside Build wraps the sync accept path.
	tool := WithIdempotency(store, nil)(AsAsyncTool(base))
	input := ToolInput{ArgsJSON: []byte(`{}`)}

	var sync1, sync2 string
	require.NoError(t, tool.Execute(context.Background(), NewRunEnv(), input, func(c Chunk) error {
		sync1 = string(c.Data)
		return nil
	}))
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, 10*time.Millisecond)

	require.NoError(t, tool.Execute(context.Background(), NewRunEnv(), input, func(c Chunk) error {
		sync2 = string(c.Data)
		return nil
	}))
	assert.Equal(t, sync1, sync2)
	assert.Contains(t, sync1, `"status":"accepted"`)
	assert.Equal(t, int32(1), calls.Load(),
		"second sync call must not re-run base; cache served accepted JSON")
}
