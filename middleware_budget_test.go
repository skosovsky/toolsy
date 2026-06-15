package toolsy

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func budgetEnv(tracker BudgetTracker) *RunEnv {
	env := NewRunEnv(nil)
	Put(env, DepKeyBudget, tracker)
	return env
}

func TestWithBudget_NoEnvPassThrough(t *testing.T) {
	var executed atomic.Bool
	inner := newMiddlewareMinTool(
		"noop",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			executed.Store(true)
			return nil
		},
	)
	wrapped := WithBudget()(inner)

	err := wrapped.Execute(context.Background(), NewRunEnv(nil), ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error {
		return nil
	})
	require.NoError(t, err)
	assert.True(t, executed.Load())
}

func TestWithBudget_MissingBudgetDepPassThrough(t *testing.T) {
	var executed atomic.Bool
	inner := newMiddlewareMinTool(
		"noop",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			executed.Store(true)
			return nil
		},
	)
	wrapped := WithBudget()(inner)

	env := NewRunEnv(nil)
	Put(env, "other", "x")
	err := wrapped.Execute(
		context.Background(),
		env,
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
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
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
		budgetEnv(tracker),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsError)
	require.Equal(t, MimeTypeToolErrorJSON, chunks[0].MimeType) //nolint:testifylint // mime type, not JSON document
	te, err := unmarshalToolErrorWire(chunks[0].Data)
	require.NoError(t, err)
	assert.Equal(t, CodeBudgetExceeded, te.Code)
	assert.Contains(t, te.Reason, "token budget exceeded")
	assert.False(t, executed.Load())
	assert.Equal(t, int64(1), tracker.calls.Load())
}

func TestWithBudget_AllowErrorReturnsToolError(t *testing.T) {
	inner := newMiddlewareMinTool(
		"guarded",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
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
		budgetEnv(tracker),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeInternal, te.Code)
	assert.Contains(t, te.Unwrap().Error(), "budget allow check failed")
}

func TestMiddlewareStack_BudgetCheckedOnceWithTruncationAndBatchErrorNotDuplicated(t *testing.T) {
	var attempts atomic.Int64
	tool := newMiddlewareMinTool(
		"readonly_network",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			attempts.Add(1)
			return ErrTimeout
		},
	)
	tool.manifest.ReadOnly = true

	tracker := &testBudgetTracker{
		allowFn: func(_ context.Context, _ ToolManifest, _ ToolInput) (bool, string, error) {
			return true, "", nil
		},
	}
	env := budgetEnv(tracker)

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
			Input:    ToolInput{CallID: "c1", ArgsJSON: []byte(`{}`)},
			Env:      env,
		}},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Empty(t, chunks)
	assert.Equal(t, int64(1), attempts.Load())
	assert.Equal(t, int64(1), tracker.calls.Load())
}

func TestMiddlewareStack_BudgetDenySkipsTool(t *testing.T) {
	var attempts atomic.Int64
	tool := newMiddlewareMinTool(
		"readonly_budget",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			attempts.Add(1)
			return ErrTimeout
		},
	)
	tool.manifest.ReadOnly = true

	tracker := &testBudgetTracker{
		allowFn: func(_ context.Context, _ ToolManifest, _ ToolInput) (bool, string, error) {
			return false, "budget exceeded", nil
		},
	}
	env := budgetEnv(tracker)

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
			Input:    ToolInput{CallID: "c1", ArgsJSON: []byte(`{}`)},
			Env:      env,
		},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsError)
	assert.Equal(t, MimeTypeToolErrorJSON, chunks[0].MimeType) //nolint:testifylint // mime type
	te, err := unmarshalToolErrorWire(chunks[0].Data)
	require.NoError(t, err)
	assert.Equal(t, CodeBudgetExceeded, te.Code)
	assert.Contains(t, te.Reason, "budget exceeded")
	assert.Equal(t, int64(0), attempts.Load())
	assert.Equal(t, int64(1), tracker.calls.Load())
}
