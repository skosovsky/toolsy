package toolsy

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithIdempotency_ReturnsCachedResult(t *testing.T) {
	var calls atomic.Int32
	tool, err := NewTool("echo", "Echo", func(_ context.Context, _ RunContext, _ struct {
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
	tool, err := NewTool("mut", "Mut", func(_ context.Context, _ RunContext, _ struct{}) (struct{}, error) {
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
