package toolsy

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mapServiceProvider map[string]any

func (m mapServiceProvider) Get(key string) (any, bool) {
	v, ok := m[key]
	return v, ok
}

type testBudgetTracker struct {
	allowFn func(context.Context, ToolManifest, ToolInput) (bool, string, error)
	calls   atomic.Int64
}

func (t *testBudgetTracker) Allow(ctx context.Context, manifest ToolManifest, input ToolInput) (bool, string, error) {
	t.calls.Add(1)
	if t.allowFn == nil {
		return true, "", nil
	}
	return t.allowFn(ctx, manifest, input)
}

func TestWithBudget_NoServicesPassThrough(t *testing.T) {
	var executed atomic.Bool
	inner := newMiddlewareMinTool(
		"noop",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			executed.Store(true)
			return nil
		},
	)
	wrapped := WithBudget()(inner)

	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error {
		return nil
	})
	require.NoError(t, err)
	assert.True(t, executed.Load())
}

func TestWithBudget_MissingBudgetServicePassThrough(t *testing.T) {
	var executed atomic.Bool
	inner := newMiddlewareMinTool(
		"noop",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			executed.Store(true)
			return nil
		},
	)
	wrapped := WithBudget()(inner)

	err := wrapped.Execute(
		context.Background(),
		RunContext{Services: mapServiceProvider{"other": "x"}},
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	assert.True(t, executed.Load())
}

func TestWithBudget_DeniedEmitsErrorChunkAndSkipsTool(t *testing.T) {
	var executed atomic.Bool
	inner := newMiddlewareMinTool(
		"guarded",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			executed.Store(true)
			return nil
		},
	)
	tracker := &testBudgetTracker{
		allowFn: func(_ context.Context, _ ToolManifest, _ ToolInput) (bool, string, error) {
			return false, "token budget exceeded", nil
		},
	}
	wrapped := WithBudget()(inner)

	var chunks []Chunk
	err := wrapped.Execute(
		context.Background(),
		RunContext{Services: mapServiceProvider{"budget": tracker}},
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsError)
	assert.Equal(t, MimeTypeText, chunks[0].MimeType)
	assert.Contains(t, string(chunks[0].Data), "token budget exceeded")
	assert.False(t, executed.Load())
	assert.Equal(t, int64(1), tracker.calls.Load())
}

func TestWithBudget_BudgetTypeMismatchReturnsSystemError(t *testing.T) {
	inner := newMiddlewareMinTool(
		"guarded",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			return nil
		},
	)
	wrapped := WithBudget()(inner)

	err := wrapped.Execute(
		context.Background(),
		RunContext{Services: mapServiceProvider{"budget": "not-a-tracker"}},
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	var sysErr *SystemError
	require.ErrorAs(t, err, &sysErr)
	assert.Contains(t, sysErr.Err.Error(), "unexpected type")
}

func TestWithBudget_AllowErrorReturnsSystemError(t *testing.T) {
	inner := newMiddlewareMinTool(
		"guarded",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			return nil
		},
	)
	tracker := &testBudgetTracker{
		allowFn: func(_ context.Context, _ ToolManifest, _ ToolInput) (bool, string, error) {
			return false, "", errors.New("backend unavailable")
		},
	}
	wrapped := WithBudget()(inner)

	err := wrapped.Execute(
		context.Background(),
		RunContext{Services: mapServiceProvider{"budget": tracker}},
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	var sysErr *SystemError
	require.ErrorAs(t, err, &sysErr)
	assert.Contains(t, sysErr.Err.Error(), "budget allow check failed")
}

func TestMiddlewareStack_BudgetCheckedOnceWithTruncationAndBatchErrorNotDuplicated(t *testing.T) {
	var attempts atomic.Int64
	tool := newMiddlewareMinTool(
		"readonly_network",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			attempts.Add(1)
			return ErrTimeout
		},
	)
	tool.manifest.Metadata = map[string]any{"read_only": true}

	tracker := &testBudgetTracker{
		allowFn: func(_ context.Context, _ ToolManifest, _ ToolInput) (bool, string, error) {
			return true, "", nil
		},
	}
	runCtx := RunContext{Services: mapServiceProvider{"budget": tracker}}

	reg, err := NewRegistryBuilder().
		Use(
			WithTruncation(32, WithTruncationSuffix("...")),
			WithErrorFormatter(),
			WithBudget(),
		).
		Add(tool).
		Build()
	require.NoError(t, err)

	var chunks []Chunk
	err = reg.ExecuteBatchStream(
		context.Background(),
		[]ToolCall{{
			ToolName: "readonly_network",
			Run:      runCtx,
			Input:    ToolInput{CallID: "c1", ArgsJSON: []byte(`{}`)},
		}},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsError)
	assert.Equal(t, MimeTypeText, chunks[0].MimeType)
	assert.Equal(t, int64(1), attempts.Load())
	assert.Equal(t, int64(1), tracker.calls.Load())
	assert.Contains(t, string(chunks[0].Data), "...")
}

func TestMiddlewareStack_BudgetDenySkipsTool(t *testing.T) {
	var attempts atomic.Int64
	tool := newMiddlewareMinTool(
		"readonly_budget",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			attempts.Add(1)
			return ErrTimeout
		},
	)
	tool.manifest.Metadata = map[string]any{"read_only": true}

	tracker := &testBudgetTracker{
		allowFn: func(_ context.Context, _ ToolManifest, _ ToolInput) (bool, string, error) {
			return false, "budget exceeded", nil
		},
	}
	runCtx := RunContext{Services: mapServiceProvider{"budget": tracker}}

	reg, err := NewRegistryBuilder().
		Use(
			WithErrorFormatter(),
			WithBudget(),
		).
		Add(tool).
		Build()
	require.NoError(t, err)

	var chunks []Chunk
	err = reg.Execute(
		context.Background(),
		ToolCall{
			ToolName: "readonly_budget",
			Run:      runCtx,
			Input:    ToolInput{CallID: "c1", ArgsJSON: []byte(`{}`)},
		},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsError)
	assert.Equal(t, int64(0), attempts.Load())
	assert.Equal(t, int64(1), tracker.calls.Load())
}
