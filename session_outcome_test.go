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

func TestSession_RunCall_Success(t *testing.T) {
	t.Parallel()
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		Double int `json:"double"`
	}
	tool, err := NewTool("double", "Double", func(_ context.Context, _ *RunEnv, a args) (result, error) {
		return result{Double: a.N * 2}, nil
	})
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "double",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":21}`)},
		Env:      NewRunEnv(sess),
	})
	require.NoError(t, err)
	require.Nil(t, outcome.ExecutionError)

	decoded, err := DecodeOutcomeAs[result](outcome)
	require.NoError(t, err)
	assert.Equal(t, 42, decoded.Double)
}

func TestSession_RunCall_BusinessValidationError(t *testing.T) {
	t.Parallel()
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		OK bool `json:"ok"`
	}
	tool, err := NewTool("check", "Check", func(_ context.Context, _ *RunEnv, a args) (result, error) {
		if a.N < 0 {
			return result{}, NewValidationError("n must be non-negative")
		}
		return result{OK: true}, nil
	})
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Add(tool).Use(WithErrorFormatter()).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "check",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":-1}`)},
		Env:      NewRunEnv(sess),
	})
	require.NoError(t, err)
	require.NotNil(t, outcome.ExecutionError)
	te, ok := AsToolError(outcome.ExecutionError)
	require.True(t, ok)
	assert.Equal(t, CodeValidationFailed, te.Code)
}

func TestSession_RunCall_BusinessValidationError_WithoutFormatter(t *testing.T) {
	t.Parallel()
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		OK bool `json:"ok"`
	}
	tool, err := NewTool("check", "Check", func(_ context.Context, _ *RunEnv, a args) (result, error) {
		if a.N < 0 {
			return result{}, NewValidationError("n must be non-negative")
		}
		return result{OK: true}, nil
	})
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "check",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":-1}`)},
		Env:      NewRunEnv(sess),
	})
	require.NoError(t, err)
	require.NotNil(t, outcome.ExecutionError)
	te, ok := AsToolError(outcome.ExecutionError)
	require.True(t, ok)
	assert.Equal(t, CodeValidationFailed, te.Code)
}

func TestSession_RunCall_ControlSignal(t *testing.T) {
	t.Parallel()
	tool, err := NewStreamTool[struct{}]("pause_tool", "Pause",
		func(_ context.Context, _ *RunEnv, _ struct{}, yield func(Chunk) error) error {
			return YieldControl(yield, &PauseSignal{Reason: `{"await":"human"}`})
		},
	)
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "pause_tool",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	require.ErrorIs(t, err, ErrPause)
	require.Len(t, outcome.Controls, 1)
	pause, ok := outcome.Controls[0].(*PauseSignal)
	require.True(t, ok)
	wantReason := `{"await":"human"}`
	assert.Equal(t, wantReason, pause.Reason)
}

func TestRunCallInfraError_Classification(t *testing.T) {
	t.Parallel()

	businessCases := []struct {
		name string
		err  error
	}{
		{name: "validation", err: NewValidationError("bad input")},
		{name: "schema", err: NewSchemaError("invalid schema")},
		{name: "budget", err: NewBudgetExceededError("quota exceeded")},
	}
	for _, tc := range businessCases {
		t.Run("business_"+tc.name, func(t *testing.T) {
			t.Parallel()
			assert.False(t, runCallInfraError(tc.err))
		})
	}

	infraCases := []struct {
		name string
		err  error
	}{
		{name: "not_found", err: NewToolNotFoundError()},
		{name: "shutdown", err: NewShutdownError()},
		{name: "max_steps", err: NewMaxStepsExceededError()},
		{name: "registry_not_ready", err: NewRegistryStateError()},
		{name: "stream_aborted", err: ErrStreamAborted},
		{name: "dependency_missing", err: NewDependencyMissingError("db")},
		{name: "contract_missing", err: NewToolsContractMissingError([]string{"a"}, []string{"a"})},
		{name: "policy_denied", err: NewPolicyDeniedError("blocked")},
		{name: "capability_denied", err: NewCapabilityDeniedError("hidden", RegistryViewSnapshot{ID: "view-1"})},
		{name: "internal", err: NewInternalError(errors.New("malformed chunk"))},
		{name: "plain", err: assert.AnError},
		{name: "context_canceled", err: context.Canceled},
		{name: "wrapped_cancel", err: NewInternalError(fmt.Errorf("tool failed: %w", context.Canceled))},
		{name: "timeout_from_deadline", err: NewTimeoutErrorFrom(context.DeadlineExceeded, true)},
		{name: "bare_deadline", err: context.DeadlineExceeded},
	}
	for _, tc := range infraCases {
		t.Run("infra_"+tc.name, func(t *testing.T) {
			t.Parallel()
			assert.True(t, runCallInfraError(tc.err))
		})
	}
}

func TestSession_RunCall_ContextCancel_IsInfraNotBusiness(t *testing.T) {
	t.Parallel()

	runCancelCase := func(t *testing.T, withFormatter bool) {
		t.Helper()
		tool, err := NewTool("cancel_tool", "Cancel", func(_ context.Context, _ *RunEnv, _ struct{}) (struct{}, error) {
			return struct{}{}, NewInternalError(fmt.Errorf("tool failed: %w", context.Canceled))
		})
		require.NoError(t, err)

		b := NewRegistryBuilder().Add(tool)
		if withFormatter {
			b = b.Use(WithErrorFormatter())
		}
		reg, err := b.Build()
		require.NoError(t, err)
		sess, err := NewSession(reg)
		require.NoError(t, err)

		outcome, err := sess.RunCall(context.Background(), ToolCall{
			ToolName: "cancel_tool",
			Input:    ToolInput{ArgsJSON: []byte(`{}`)},
			Env:      NewRunEnv(sess),
		})
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
		require.True(t, runCallInfraError(err))
		require.Nil(t, outcome.ExecutionError)
		require.Empty(t, outcome.Result)
		assert.Equal(t, OutcomeInfrastructureError, outcome.Status)
	}

	t.Run("with_formatter", func(t *testing.T) {
		t.Parallel()
		runCancelCase(t, true)
	})
	t.Run("without_formatter", func(t *testing.T) {
		t.Parallel()
		runCancelCase(t, false)
	})
}

func TestSession_RunCall_CancelOverReadLimit_IsInfra(t *testing.T) {
	t.Parallel()
	composite := fmt.Errorf("read failed: %w", textprocessor.ErrReadLimitExceeded)
	cancelWrapped := fmt.Errorf("aborted: %w", context.Canceled)
	tool, err := NewTool(
		"cancel_limit_tool",
		"CancelLimit",
		func(_ context.Context, _ *RunEnv, _ struct{}) (struct{}, error) {
			return struct{}{}, NewInternalError(fmt.Errorf("tool: %w", cancelWrapped))
		},
	)
	require.NoError(t, err)
	_ = composite

	reg, err := NewRegistryBuilder().Add(tool).Use(WithErrorFormatter()).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "cancel_limit_tool",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, runCallInfraError(err))
	require.Nil(t, outcome.ExecutionError)
	assert.Equal(t, OutcomeInfrastructureError, outcome.Status)
}

func TestSession_RunCall_DeadlineExceeded_IsInfraWithChain(t *testing.T) {
	t.Parallel()
	tool, err := NewTool("deadline_tool", "Deadline", func(_ context.Context, _ *RunEnv, _ struct{}) (struct{}, error) {
		return struct{}{}, NewInternalError(fmt.Errorf("tool failed: %w", context.DeadlineExceeded))
	})
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Add(tool).Use(WithErrorFormatter()).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "deadline_tool",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.True(t, runCallInfraError(err))
	require.Nil(t, outcome.ExecutionError)
	assert.Equal(t, OutcomeInfrastructureError, outcome.Status)
	te, ok := AsToolError(err)
	if ok {
		require.Equal(t, CodeTimeout, te.Code)
	}
}

func TestSession_RunCall_InfraMaxSteps(t *testing.T) {
	t.Parallel()
	tool := newMiddlewareMinTool("ok", func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
		return nil
	})
	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg, WithMaxSteps(1))
	require.NoError(t, err)

	_, err = sess.RunCall(context.Background(), ToolCall{
		ToolName: "ok",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "ok",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	requireToolErrorCode(t, err, CodeMaxStepsExceeded, ErrMaxStepsExceeded)
	assert.Equal(t, OutcomeInfrastructureError, outcome.Status)
}

func TestSession_RunCall_InfraDependencyMissing(t *testing.T) {
	t.Parallel()
	tool := newMiddlewareMinTool("needs_db",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return NewDependencyMissingError("db")
		},
	)
	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "needs_db",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	requireToolErrorCode(t, err, CodeDependencyMissing)
	assert.Equal(t, OutcomeInfrastructureError, outcome.Status)
}

func TestSession_RunCall_InfraDependencyMissing_WithFormatter(t *testing.T) {
	t.Parallel()
	tool := newMiddlewareMinTool("needs_db",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return NewDependencyMissingError("db")
		},
	)
	reg, err := NewRegistryBuilder().Use(WithErrorFormatter()).Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	_, err = sess.RunCall(context.Background(), ToolCall{
		ToolName: "needs_db",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	requireToolErrorCode(t, err, CodeDependencyMissing)
}

func TestSession_RunCall_InfraToolsContractMissing_WithFormatter(t *testing.T) {
	t.Parallel()
	tool := newMiddlewareMinTool("contract",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return NewToolsContractMissingError([]string{"a", "b"}, []string{"b"})
		},
	)
	reg, err := NewRegistryBuilder().Use(WithErrorFormatter()).Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	_, err = sess.RunCall(context.Background(), ToolCall{
		ToolName: "contract",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	requireToolErrorCode(t, err, CodeToolsContractMissing)
}

func TestSession_RunCall_StructuredErrorEnvelope(t *testing.T) {
	t.Parallel()
	type args struct {
		City string `json:"city"`
	}
	type result struct {
		OK bool `json:"ok"`
	}
	tool, err := NewTool("check", "Check", func(_ context.Context, _ *RunEnv, a args) (result, error) {
		if a.City == "" {
			return result{}, NewValidationError("city is required", "city")
		}
		return result{OK: true}, nil
	})
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Use(WithErrorFormatter()).Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "check",
		Input:    ToolInput{ArgsJSON: []byte(`{"city":""}`)},
		Env:      NewRunEnv(sess),
	})
	require.NoError(t, err)
	require.NotNil(t, outcome.ExecutionError)
	te, ok := AsToolError(outcome.ExecutionError)
	require.True(t, ok)
	assert.Equal(t, CodeValidationFailed, te.Code)
	assert.False(t, te.Retryable)
	assert.Equal(t, []string{"city"}, te.FixableArgs)
}

func TestSession_RunCall_NilSession(t *testing.T) {
	t.Parallel()
	var sess *Session
	_, err := sess.RunCall(context.Background(), ToolCall{ToolName: "x"})
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, CodeValidationFailed, te.Code)
}

func TestWithBudget_RunCallPreservesCode(t *testing.T) {
	t.Parallel()
	inner := newMiddlewareMinTool(
		"guarded",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return nil
		},
	)
	tracker := &testBudgetTracker{
		allowFn: func(_ context.Context, _ ToolManifest, _ ToolInput) (bool, string, error) {
			return false, "token budget exceeded", nil
		},
	}
	reg, err := NewRegistryBuilder().Use(WithBudget()).Add(inner).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)
	env := NewRunEnv(sess)
	Put(env, DepKeyBudget, tracker)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "guarded",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      env,
	})
	require.NoError(t, err)
	require.NotNil(t, outcome.ExecutionError)
	te, ok := AsToolError(outcome.ExecutionError)
	require.True(t, ok)
	assert.Equal(t, CodeBudgetExceeded, te.Code)
}

func TestSession_RunCall_ErrorChunkClearsPriorResult(t *testing.T) {
	t.Parallel()
	tool := newMiddlewareMinTool(
		"then_fail",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			if err := yield(Chunk{
				Event:    EventResult,
				Data:     []byte(`{"ok":true}`),
				MimeType: MimeTypeJSON,
			}); err != nil {
				return err
			}
			return NewValidationError("failed after result")
		},
	)
	reg, err := NewRegistryBuilder().Use(WithErrorFormatter()).Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "then_fail",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	require.NoError(t, err)
	require.NotNil(t, outcome.ExecutionError)
	assert.Nil(t, outcome.Result)
	assert.Empty(t, outcome.ResultMimeType)
}

func TestSession_RunCall_LegacyTextErrorChunk_NormalizedToInfraError(t *testing.T) {
	t.Parallel()
	tool := newMiddlewareMinTool(
		"legacy_text_error",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{
				Event:    EventResult,
				Data:     []byte("budget exceeded"),
				MimeType: MimeTypeText,
				IsError:  true,
			})
		},
	)
	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "legacy_text_error",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	requireToolErrorCode(t, err, CodeInternal)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.Contains(t, te.Reason, "malformed error chunk")
	assert.Equal(t, OutcomeInfrastructureError, outcome.Status)
}

func TestSession_RunCall_StructuredBusinessErrorChunk_ToOutcome(t *testing.T) {
	t.Parallel()
	tool := newMiddlewareMinTool(
		"validation_error",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(NewErrorChunkFromErr(NewValidationError("bad input")))
		},
	)
	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "validation_error",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	require.NoError(t, err)
	requireToolErrorCode(t, outcome.ExecutionError, CodeValidationFailed)
	assert.NoError(t, err)
}

func TestSession_RunCall_InfraInternal_WithFormatter(t *testing.T) {
	t.Parallel()
	tool := newMiddlewareMinTool(
		"internal_fail",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return NewInternalError(errors.New("db down"))
		},
	)
	reg, err := NewRegistryBuilder().Use(WithErrorFormatter()).Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	_, err = sess.RunCall(context.Background(), ToolCall{
		ToolName: "internal_fail",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	requireToolErrorCode(t, err, CodeInternal)
}

func TestSession_RunCall_ProgressBeforeBusinessError(t *testing.T) {
	t.Parallel()
	tool, err := NewStreamTool[struct{}]("stream_fail", "Stream fail",
		func(_ context.Context, _ *RunEnv, _ struct{}, yield func(Chunk) error) error {
			if err := yield(Chunk{
				Event:    EventProgress,
				Data:     []byte(`{"step":1}`),
				MimeType: MimeTypeJSON,
			}); err != nil {
				return err
			}
			data, err := marshalToolErrorWire(NewValidationError("failed"), "Error executing tool: failed")
			if err != nil {
				return err
			}
			return yield(Chunk{
				Event:    EventResult,
				Data:     data,
				MimeType: MimeTypeToolErrorJSON,
				IsError:  true,
			})
		},
	)
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "stream_fail",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	require.NoError(t, err)
	require.NotNil(t, outcome.ExecutionError)
	require.Len(t, outcome.Progress, 1)
	assert.Equal(t, EventProgress, outcome.Progress[0].Event)
}

func TestSession_RunCall_InfraToolNotFound(t *testing.T) {
	t.Parallel()
	reg, err := NewRegistryBuilder().Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "missing",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, CodeToolNotFound, te.Code)
	assert.Equal(t, OutcomeInfrastructureError, outcome.Status)
}

func TestSession_RunCall_SandboxStdoutReadLimit_BusinessOutcome(t *testing.T) {
	t.Parallel()
	const maxOut = 256 * 1024
	capErr := fmt.Errorf(
		"sandbox: stdout exceeds %d byte limit: %w",
		maxOut,
		textprocessor.ErrReadLimitExceeded,
	)
	tool := newMiddlewareMinTool(
		"exec_sandbox",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return MapSandboxReadLimitError(capErr)
		},
	)
	reg, err := NewRegistryBuilder().Add(tool).Use(WithErrorFormatter()).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "exec_sandbox",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	require.NoError(t, err)
	requireToolErrorCode(t, outcome.ExecutionError, CodeValidationFailed)
	te, ok := AsToolError(outcome.ExecutionError)
	require.True(t, ok)
	assert.Contains(t, te.Reason, "stdout")
	assert.False(t, runCallInfraError(outcome.ExecutionError))
}

func TestNewTypedTool_Validators(t *testing.T) {
	t.Parallel()
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		V int `json:"v"`
	}
	// Arrange.
	tool, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, args, result, struct{}]{
		Name:        "typed",
		Description: "Typed",
		ArgValidator: func(a args) error {
			if a.N <= 0 {
				return NewValidationError("n must be positive")
			}
			return nil
		},
		ResultValidator: func(r result) error {
			if r.V <= 0 {
				return NewValidationError("result must be positive")
			}
			return nil
		},
		Handler: func(
			_ context.Context,
			_ TypedCallContext[NoSubject, NoScope],
			_ *RunEnv,
			a args,
		) (ToolResult[result, struct{}], error) {
			return NewToolResult[result, struct{}](result{V: a.N}), nil
		},
	})
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)

	// Act.
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "typed",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":0}`)},
		Env:      NewRunEnv(nil),
	}, func(Chunk) error { return nil })

	// Assert.
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, CodeValidationFailed, te.Code)

	// Act.
	var res result
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "typed",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":2}`)},
		Env:      NewRunEnv(nil),
	}, func(c Chunk) error { return json.Unmarshal(c.Data, &res) })

	// Assert.
	require.NoError(t, err)
	assert.Equal(t, 2, res.V)
}
