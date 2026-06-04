package toolsy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithErrorFormatter_ValidationErrorBecomesErrorChunk(t *testing.T) {
	inner := newMiddlewareMinTool(
		"client_fail",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return NewValidationError("city is required")
		},
	)
	wrapped := WithErrorFormatter()(inner)

	var chunks []Chunk
	err := wrapped.Execute(
		context.Background(),
		NewRunEnv(nil),
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
	assert.Contains(t, string(chunks[0].Data), "city is required")
}

func TestWithErrorFormatter_InternalToolErrorDoesNotLeakInternalMessage(t *testing.T) {
	inner := newMiddlewareMinTool(
		"system_fail",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return NewInternalError(errors.New("db password leaked"))
		},
	)
	wrapped := WithErrorFormatter()(inner)

	var got Chunk
	err := wrapped.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(c Chunk) error {
			got = c
			return nil
		},
	)
	require.NoError(t, err)
	assert.True(t, got.IsError)
	assert.NotContains(t, string(got.Data), "password")
	assert.Contains(t, string(got.Data), "internal system error")
}

func TestWithErrorFormatter_DependencyMissingHint(t *testing.T) {
	inner := newMiddlewareMinTool(
		"dep_missing",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return NewDependencyMissingError("db")
		},
	)
	wrapped := WithErrorFormatter()(inner)

	var got Chunk
	err := wrapped.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(c Chunk) error {
			got = c
			return nil
		},
	)
	require.NoError(t, err)
	assert.True(t, got.IsError)
	assert.Contains(t, string(got.Data), "required dependency is missing")
	assert.Contains(t, string(got.Data), "agent configuration")
}

func TestWithErrorFormatter_ToolsContractMissingHint(t *testing.T) {
	inner := newMiddlewareMinTool(
		"contract_missing",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return NewToolsContractMissingError([]string{"a", "b"}, []string{"b"})
		},
	)
	wrapped := WithErrorFormatter()(inner)

	var got Chunk
	err := wrapped.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(c Chunk) error {
			got = c
			return nil
		},
	)
	require.NoError(t, err)
	assert.True(t, got.IsError)
	assert.Contains(t, string(got.Data), "required tools are not registered")
	assert.Contains(t, string(got.Data), "Register missing tools")
}

func TestWithErrorFormatter_PrefersSafeMessageOverReason(t *testing.T) {
	inner := newMiddlewareMinTool(
		"safe_msg",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return WithSafeMessage(
				NewValidationError("secret internal: api_key=leaked"),
				"Please provide a valid city name",
			)
		},
	)
	wrapped := WithErrorFormatter()(inner)

	var got Chunk
	err := wrapped.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(c Chunk) error {
			got = c
			return nil
		},
	)
	require.NoError(t, err)
	body := string(got.Data)
	assert.Contains(t, body, "valid city name")
	assert.NotContains(t, body, "api_key")
	assert.NotContains(t, body, "secret internal")
}

func TestWithErrorFormatter_BypassesSuspendAndStreamAborted(t *testing.T) {
	suspend := newMiddlewareMinTool(
		"suspend",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return ErrPause
		},
	)
	streamAborted := newMiddlewareMinTool(
		"abort",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return ErrStreamAborted
		},
	)

	err := WithErrorFormatter()(suspend).Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrPause)

	err = WithErrorFormatter()(streamAborted).Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrStreamAborted)
}

func TestWithErrorFormatter_BypassesContextCanceled(t *testing.T) {
	canceled := newMiddlewareMinTool(
		"canceled",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return context.Canceled
		},
	)

	yieldCalls := 0
	err := WithErrorFormatter()(canceled).Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error {
			yieldCalls++
			return nil
		},
	)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, yieldCalls)
}

func TestWithErrorFormatter_YieldFailureIsWrapped(t *testing.T) {
	inner := newMiddlewareMinTool(
		"fail",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return errors.New("boom")
		},
	)
	wrapped := WithErrorFormatter()(inner)

	err := wrapped.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error {
			return errors.New("stop")
		},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStreamAborted)
}

func TestWithErrorFormatter_PlainErrorPreservesActionableMessage(t *testing.T) {
	inner := newMiddlewareMinTool(
		"plain_fail",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return errors.New("rate limit exceeded")
		},
	)
	wrapped := WithErrorFormatter()(inner)

	var got Chunk
	err := wrapped.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(c Chunk) error {
			got = c
			return nil
		},
	)
	require.NoError(t, err)
	assert.True(t, got.IsError)
	assert.Contains(t, string(got.Data), "rate limit exceeded")
}

func TestWithErrorFormatter_PlainErrorUsesFirstLineOnly(t *testing.T) {
	inner := newMiddlewareMinTool(
		"multiline_fail",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return errors.New("database connection failed\nstack line 1")
		},
	)
	wrapped := WithErrorFormatter()(inner)

	var got Chunk
	err := wrapped.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(c Chunk) error {
			got = c
			return nil
		},
	)
	require.NoError(t, err)
	assert.True(t, got.IsError)
	assert.Contains(t, string(got.Data), "database connection failed")
	assert.NotContains(t, string(got.Data), "stack line 1")
}

func TestSessionExecute_ErrorFormatterSoftErrorCountsStep(t *testing.T) {
	inner := newMiddlewareMinTool(
		"soft_error",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return errors.New("raw failure")
		},
	)
	reg, err := NewRegistryBuilder().Use(WithErrorFormatter()).Add(inner).Build()
	require.NoError(t, err)

	session, err := NewSession(reg, WithMaxSteps(5))
	require.NoError(t, err)
	var got Chunk
	err = session.Execute(
		context.Background(),
		ToolCall{ToolName: "soft_error", Input: ToolInput{CallID: "c1", ArgsJSON: []byte(`{}`)}},
		func(c Chunk) error {
			got = c
			return nil
		},
	)
	require.NoError(t, err)
	assert.True(t, got.IsError)
	assert.Equal(t, int64(1), session.Track().ExecutionCount())
}

func TestWithErrorFormatter_RegistryExecuteIter_EmitsSoftErrorChunk(t *testing.T) {
	inner := newMiddlewareMinTool(
		"iter_fail",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return errors.New("iter failure")
		},
	)
	reg, err := NewRegistryBuilder().Use(WithErrorFormatter()).Add(inner).Build()
	require.NoError(t, err)

	var chunks []Chunk
	var iterErrs []error
	for chunk, iterErr := range reg.ExecuteIter(
		context.Background(),
		ToolCall{ToolName: "iter_fail", Input: ToolInput{CallID: "iter-1", ArgsJSON: []byte(`{}`)}},
	) {
		if iterErr != nil {
			iterErrs = append(iterErrs, iterErr)
			continue
		}
		chunks = append(chunks, chunk)
	}

	require.Empty(t, iterErrs)
	require.Len(t, chunks, 1)
	assert.Equal(t, "iter-1", chunks[0].CallID)
	assert.Equal(t, "iter_fail", chunks[0].ToolName)
	assert.True(t, chunks[0].IsError)
	assert.Contains(t, string(chunks[0].Data), "iter failure")
}

func TestWithErrorFormatter_RegistryExecuteBatchStream_NoDuplicateErrorChunk(t *testing.T) {
	inner := newMiddlewareMinTool(
		"batch_fail",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return errors.New("batch failure")
		},
	)
	reg, err := NewRegistryBuilder().Use(WithErrorFormatter()).Add(inner).Build()
	require.NoError(t, err)

	var chunks []Chunk
	err = reg.ExecuteBatchStream(
		context.Background(),
		[]ToolCall{
			{ToolName: "batch_fail", Input: ToolInput{CallID: "batch-1", ArgsJSON: []byte(`{}`)}},
		},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, "batch-1", chunks[0].CallID)
	assert.Equal(t, "batch_fail", chunks[0].ToolName)
	assert.True(t, chunks[0].IsError)
	assert.Contains(t, string(chunks[0].Data), "batch failure")
}

func TestWithErrorFormatter_PreToolErrorsRemainHard(t *testing.T) {
	tool := newMiddlewareMinTool(
		"ok_tool",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return nil
		},
	)
	reg, err := NewRegistryBuilder().Use(WithErrorFormatter()).Add(tool).Build()
	require.NoError(t, err)

	var missingToolChunks []Chunk
	err = reg.Execute(
		context.Background(),
		ToolCall{
			ToolName: "missing_tool",
			Input:    ToolInput{CallID: "missing-1", ArgsJSON: []byte(`{}`)},
		},
		func(c Chunk) error {
			missingToolChunks = append(missingToolChunks, c)
			return nil
		},
	)
	requireToolErrorCode(t, err, CodeToolNotFound, ErrToolNotFound)
	require.Empty(t, missingToolChunks)

	session, err := NewSession(reg, WithMaxSteps(1))
	require.NoError(t, err)
	err = session.Execute(
		context.Background(),
		ToolCall{ToolName: "ok_tool", Input: ToolInput{CallID: "step-1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)

	var maxStepChunks []Chunk
	err = session.Execute(
		context.Background(),
		ToolCall{ToolName: "ok_tool", Input: ToolInput{CallID: "step-2", ArgsJSON: []byte(`{}`)}},
		func(c Chunk) error {
			maxStepChunks = append(maxStepChunks, c)
			return nil
		},
	)
	requireToolErrorCode(t, err, CodeMaxStepsExceeded, ErrMaxStepsExceeded)
	require.Empty(t, maxStepChunks)
}
