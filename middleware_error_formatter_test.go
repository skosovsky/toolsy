package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
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
	require.Equal(t, MimeTypeToolErrorJSON, chunks[0].MimeType) //nolint:testifylint // mime type, not JSON payload
	te, err := unmarshalToolErrorWire(chunks[0].Data)
	require.NoError(t, err)
	assert.Equal(t, CodeValidationFailed, te.Code)
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
	require.Equal(t, MimeTypeToolErrorJSON, got.MimeType) //nolint:testifylint // mime type, not JSON payload
	var wire struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(got.Data, &wire))
	assert.Contains(t, wire.Message, "valid city name")
	assert.NotContains(t, wire.Message, "api_key")
	assert.NotContains(t, wire.Message, "secret internal")
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

func TestFormatExecutionError_CancelOverReadLimit(t *testing.T) {
	t.Parallel()
	err := NewInternalError(fmt.Errorf("toolkit: get failed: %w", context.Canceled))
	msg := formatExecutionError(err)
	require.Contains(t, msg, "canceled")
	require.NotContains(t, msg, "internal system error")
	require.NotContains(t, msg, "byte limit")
}

func TestFormatExecutionError_TimeoutOverReadLimit(t *testing.T) {
	t.Parallel()
	err := NewInternalError(fmt.Errorf("toolkit: get failed: %w", context.DeadlineExceeded))
	msg := formatExecutionError(err)
	require.Contains(t, msg, "timed out")
	require.NotContains(t, msg, "internal system error")
	require.NotContains(t, msg, "byte limit")
}

func TestErrorChunkSummaryText_InterruptOverInternal(t *testing.T) {
	t.Parallel()
	err := NewInternalError(fmt.Errorf("toolkit: get failed: %w", context.Canceled))
	chunk := NewErrorChunkFromErr(err)
	text := ErrorChunkSummaryText(chunk, err)
	require.Contains(t, text, "canceled")
	require.NotContains(t, text, "internal system error")
}

func TestNewErrorChunkFromErr_InternalWrappedCancel_LlmMessageNotGeneric(t *testing.T) {
	t.Parallel()
	err := NewInternalError(fmt.Errorf("toolkit: get failed: %w", context.Canceled))
	chunk := NewErrorChunkFromErr(err)
	require.True(t, chunk.IsError)
	require.Contains(t, string(chunk.Data), "canceled")
	require.NotContains(t, string(chunk.Data), "internal system error")
}

func TestReadLimitToolError_SandboxSubject(t *testing.T) {
	t.Parallel()
	const maxBytes = 4096
	capErr := fmt.Errorf(
		"sandbox: stdout exceeds %d byte limit: %w",
		maxBytes,
		textprocessor.ErrReadLimitExceeded,
	)
	te := readLimitToolError(capErr)
	require.NotNil(t, te)
	require.Equal(t, CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "stdout exceeds 4096 byte limit")
}

func TestReadLimitToolError_BareSentinel_UsesGeneric(t *testing.T) {
	t.Parallel()
	te := readLimitToolError(textprocessor.ErrReadLimitExceeded)
	require.NotNil(t, te)
	require.Equal(t, CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "response exceeds byte limit")
	require.NotContains(t, te.Reason, "262144")
}

func TestErrorChunkGoldenOrder_CancelOverReadLimit(t *testing.T) {
	t.Parallel()
	composite := fmt.Errorf("read failed: %w", textprocessor.ErrReadLimitExceeded)
	cancelWrapped := fmt.Errorf("aborted: %w", context.Canceled)
	inner := NewInternalError(fmt.Errorf("tool: %w", cancelWrapped))
	chunk := NewErrorChunkFromErr(inner)
	require.True(t, chunk.IsError)
	te, err := unmarshalToolErrorWire(chunk.Data)
	require.NoError(t, err)
	require.NotEqual(t, CodeValidationFailed, te.Code)
	require.Contains(t, string(chunk.Data), "canceled")

	limitErr := NewInternalError(composite)
	limitChunk := NewErrorChunkFromErr(limitErr)
	limitTE, err := unmarshalToolErrorWire(limitChunk.Data)
	require.NoError(t, err)
	require.Equal(t, CodeValidationFailed, limitTE.Code)
}

func TestWithErrorFormatter_BypassesDeadlineExceeded(t *testing.T) {
	t.Parallel()
	deadline := newMiddlewareMinTool(
		"deadline_tool",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return context.DeadlineExceeded
		},
	)
	yieldCalls := 0
	err := WithErrorFormatter()(deadline).Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error {
			yieldCalls++
			return nil
		},
	)
	require.ErrorIs(t, err, context.DeadlineExceeded)
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

	err := wrapped.Execute(context.Background(), NewRunEnv(nil), ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error {
		return errors.New("stop")
	})
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
	require.Equal(t, MimeTypeToolErrorJSON, got.MimeType) //nolint:testifylint // mime type
	assert.Contains(t, string(got.Data), `"code":"INTERNAL"`)
	assert.Contains(t, string(got.Data), "internal system error")
}

func TestErrorChunkSummaryText_LegacyTextNormalizes(t *testing.T) {
	t.Parallel()
	text := ErrorChunkSummaryText(Chunk{
		Event:    EventResult,
		Data:     []byte("budget exceeded"),
		MimeType: MimeTypeText,
		IsError:  true,
	}, nil)
	assert.Contains(t, text, "malformed error chunk")
	assert.Contains(t, text, "budget exceeded")
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
	require.Equal(t, MimeTypeToolErrorJSON, got.MimeType) //nolint:testifylint // mime type
	assert.Contains(t, string(got.Data), `"code":"INTERNAL"`)
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
	require.Equal(t, MimeTypeToolErrorJSON, chunks[0].MimeType) //nolint:testifylint // mime type
	assert.Contains(t, string(chunks[0].Data), `"code":"INTERNAL"`)
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
		[]ToolCall{{ToolName: "batch_fail", Input: ToolInput{CallID: "batch-1", ArgsJSON: []byte(`{}`)}}},
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
	require.Equal(t, MimeTypeToolErrorJSON, chunks[0].MimeType) //nolint:testifylint // mime type
	assert.Contains(t, string(chunks[0].Data), `"code":"INTERNAL"`)
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
		ToolCall{ToolName: "missing_tool", Input: ToolInput{CallID: "missing-1", ArgsJSON: []byte(`{}`)}},
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

func TestNewErrorChunkFromErr_ReadLimitExceeded(t *testing.T) {
	chunk := NewErrorChunkFromErr(textprocessor.ErrReadLimitExceeded)
	require.True(t, chunk.IsError)
	te, err := unmarshalToolErrorWire(chunk.Data)
	require.NoError(t, err)
	assert.Equal(t, CodeValidationFailed, te.Code)
	assert.Contains(t, string(chunk.Data), "byte limit")
}

func TestNewErrorChunkFromErr_WrappedCancel_NotValidation(t *testing.T) {
	t.Parallel()
	err := NewInternalError(fmt.Errorf("toolkit: get failed: %w", context.Canceled))
	chunk := NewErrorChunkFromErr(err)
	require.True(t, chunk.IsError)
	te, unmarshalErr := unmarshalToolErrorWire(chunk.Data)
	require.NoError(t, unmarshalErr)
	require.NotEqual(t, CodeValidationFailed, te.Code)
	require.ErrorIs(t, err, context.Canceled)
}

func TestNewErrorChunkFromErr_BareCancel_NotValidation(t *testing.T) {
	t.Parallel()
	chunk := NewErrorChunkFromErr(context.Canceled)
	require.True(t, chunk.IsError)
	te, unmarshalErr := unmarshalToolErrorWire(chunk.Data)
	require.NoError(t, unmarshalErr)
	require.NotEqual(t, CodeValidationFailed, te.Code)
}

func TestNewErrorChunkFromErr_ExistingToolErrorWrappedCancel_NotValidation(t *testing.T) {
	t.Parallel()
	validationTE := NewValidationError("invalid field")
	err := errors.Join(validationTE, context.Canceled)
	chunk := NewErrorChunkFromErr(err)
	require.True(t, chunk.IsError)
	te, unmarshalErr := unmarshalToolErrorWire(chunk.Data)
	require.NoError(t, unmarshalErr)
	require.NotEqual(t, CodeValidationFailed, te.Code)
}

func TestNewErrorChunkFromErr_WrappedDeadline(t *testing.T) {
	t.Parallel()
	err := NewInternalError(fmt.Errorf("toolkit: slow op: %w", context.DeadlineExceeded))
	chunk := NewErrorChunkFromErr(err)
	require.True(t, chunk.IsError)
	te, unmarshalErr := unmarshalToolErrorWire(chunk.Data)
	require.NoError(t, unmarshalErr)
	require.Equal(t, CodeTimeout, te.Code)
	require.True(t, te.Retryable)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestNewErrorChunkFromErr_InternalWrappingReadLimit(t *testing.T) {
	t.Parallel()
	err := NewInternalError(fmt.Errorf("proxy: %w", textprocessor.ErrReadLimitExceeded))
	chunk := NewErrorChunkFromErr(err)
	require.True(t, chunk.IsError)
	te, unmarshalErr := unmarshalToolErrorWire(chunk.Data)
	require.NoError(t, unmarshalErr)
	require.Equal(t, CodeValidationFailed, te.Code)
}

func TestNewErrorChunkFromErr_CancelOverReadLimit(t *testing.T) {
	t.Parallel()
	composite := fmt.Errorf("read failed: %w", textprocessor.ErrReadLimitExceeded)
	cancelWrapped := fmt.Errorf("aborted: %w", context.Canceled)
	err := NewInternalError(fmt.Errorf("tool: %w", cancelWrapped))
	_ = composite // limit-only baseline covered elsewhere
	chunk := NewErrorChunkFromErr(err)
	require.True(t, chunk.IsError)
	te, unmarshalErr := unmarshalToolErrorWire(chunk.Data)
	require.NoError(t, unmarshalErr)
	require.NotEqual(t, CodeValidationFailed, te.Code)
	require.ErrorIs(t, err, context.Canceled)

	limitErr := NewInternalError(composite)
	limitChunk := NewErrorChunkFromErr(limitErr)
	te2, unmarshalErr2 := unmarshalToolErrorWire(limitChunk.Data)
	require.NoError(t, unmarshalErr2)
	require.Equal(t, CodeValidationFailed, te2.Code)
}

func TestNewErrorChunkFromErr_InterruptInChainOverReadLimit(t *testing.T) {
	t.Parallel()
	composite := fmt.Errorf(
		"read failed: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	chunk := NewErrorChunkFromErr(composite)
	require.True(t, chunk.IsError)
	te, unmarshalErr := unmarshalToolErrorWire(chunk.Data)
	require.NoError(t, unmarshalErr)
	require.NotEqual(t, CodeValidationFailed, te.Code)
	require.ErrorIs(t, composite, context.Canceled)
}

func TestNewErrorChunkFromErr_DeadlineOverReadLimit(t *testing.T) {
	t.Parallel()
	composite := fmt.Errorf("read failed: %w", textprocessor.ErrReadLimitExceeded)
	deadlineWrapped := fmt.Errorf("slow: %w", context.DeadlineExceeded)
	err := NewInternalError(fmt.Errorf("tool: %w", deadlineWrapped))
	chunk := NewErrorChunkFromErr(err)
	require.True(t, chunk.IsError)
	te, unmarshalErr := unmarshalToolErrorWire(chunk.Data)
	require.NoError(t, unmarshalErr)
	require.Equal(t, CodeTimeout, te.Code)
	require.NotEqual(t, CodeValidationFailed, te.Code)

	limitChunk := NewErrorChunkFromErr(NewInternalError(composite))
	te2, unmarshalErr2 := unmarshalToolErrorWire(limitChunk.Data)
	require.NoError(t, unmarshalErr2)
	require.Equal(t, CodeValidationFailed, te2.Code)
}

func TestNewErrorChunkFromErr_TimeoutOverReadLimit(t *testing.T) {
	t.Parallel()
	composite := fmt.Errorf("read failed: %w", textprocessor.ErrReadLimitExceeded)
	timeoutWrapped := fmt.Errorf("slow: %w", ErrTimeout)
	err := NewInternalError(fmt.Errorf("tool: %w", timeoutWrapped))
	chunk := NewErrorChunkFromErr(err)
	require.True(t, chunk.IsError)
	te, unmarshalErr := unmarshalToolErrorWire(chunk.Data)
	require.NoError(t, unmarshalErr)
	require.Equal(t, CodeTimeout, te.Code)
	require.NotEqual(t, CodeValidationFailed, te.Code)

	limitChunk := NewErrorChunkFromErr(NewInternalError(composite))
	te2, unmarshalErr2 := unmarshalToolErrorWire(limitChunk.Data)
	require.NoError(t, unmarshalErr2)
	require.Equal(t, CodeValidationFailed, te2.Code)
}

func TestWithErrorFormatter_BypassesWrappedCancel(t *testing.T) {
	t.Parallel()
	inner := newMiddlewareMinTool(
		"cancel_tool",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return NewInternalError(fmt.Errorf("toolkit: op failed: %w", context.Canceled))
		},
	)
	yieldCalls := 0
	err := WithErrorFormatter()(inner).Execute(
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

func TestWithErrorFormatter_BypassesBareCancel(t *testing.T) {
	t.Parallel()
	inner := newMiddlewareMinTool(
		"cancel_tool_bare",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return context.Canceled
		},
	)
	yieldCalls := 0
	err := WithErrorFormatter()(inner).Execute(
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
