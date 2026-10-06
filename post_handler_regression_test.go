package toolsy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPostHandlerFailureNeverRepairsArguments(t *testing.T) {
	for _, phase := range []string{"result_validator", "effect_validator", "postcondition"} {
		for _, wrapped := range []bool{false, true} {
			t.Run(phase+map[bool]string{false: "/plain", true: "/tool-error"}[wrapped], func(t *testing.T) {
				// Arrange: external effect precedes validator failure.
				cause := errors.New("contract cause")
				var failure = cause
				if wrapped {
					failure = &ToolError{
						Code:        CodeValidationFailed,
						Reason:      "repair me",
						Retryable:   true,
						FixableArgs: []string{"value"},
						Err:         cause,
					}
				}
				effects, results := 0, 0
				spec := TypedToolSpec[NoSubject, NoScope, struct{}, string, string]{
					Name:        "write",
					Description: "Write",
					Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
						effects++
						return NewToolResult[string, string]("ok"), nil
					},
				}
				switch phase {
				case "result_validator":
					spec.ResultValidator = func(string) error { return failure }
				case "effect_validator":
					spec.EffectValidator = func([]string) error { return failure }
				case "postcondition":
					spec.Postcondition = func(ToolResult[string, string]) error { return failure }
				}
				tool, err := NewTypedTool(spec)
				require.NoError(t, err)
				execute := func() error {
					return tool.Execute(
						context.Background(),
						NewRunEnv(nil),
						ToolInput{ArgsJSON: []byte(`{}`)},
						func(Chunk) error { results++; return nil },
					)
				}
				// Act: a host repair flow only re-dispatches correctable argument failures.
				err = execute()
				if te, ok := AsToolError(err); ok && ClientCorrectable(te.Code) {
					err = execute()
				}
				// Assert.
				require.Error(t, err)
				te, ok := AsToolError(err)
				require.True(t, ok)
				require.Equal(t, CodeInternal, te.Code)
				require.False(t, ClientCorrectable(te.Code))
				require.False(t, te.Retryable)
				require.Empty(t, te.FixableArgs)
				var contract *ResultContractError
				require.ErrorAs(t, err, &contract)
				require.Equal(t, phase, contract.Kind)
				require.ErrorIs(t, err, cause)
				require.Equal(t, 1, effects)
				require.Zero(t, results)
			})
		}
	}
}

type postHandlerClaimStore struct {
	OperationStore

	binding    OperationBinding
	afterClaim func()
}

func (s *postHandlerClaimStore) Claim(ctx context.Context, claim OperationClaim) (ClaimResult, error) {
	result, err := s.OperationStore.Claim(ctx, claim)
	if err == nil {
		s.binding = claim.Binding
		if s.afterClaim != nil {
			s.afterClaim()
		}
	}
	return result, err
}

func postHandlerProfile(t *testing.T, store OperationStore, clock func() time.Time) *OperationProfile {
	t.Helper()
	profile, err := NewOperationProfile(store, func(context.Context, PreparedCall) (OperationIntent, error) {
		return OperationIntent{
			Namespace:         "test",
			Scope:             "tenant",
			Subject:           "host",
			OperationID:       "one",
			AttemptID:         "attempt",
			PolicyFingerprint: "policy",
			CanonicalDigest:   "digest",
			CanonicalRules:    "json",
			DisplayJSON:       []byte(`{}`),
		}, nil
	}, JSONResultCodec[string, string]{}, "host", clock, time.Minute, 0)
	require.NoError(t, err)
	return profile
}

func TestPostHandlerJournalOutcome(t *testing.T) {
	for _, phase := range []string{"result_validator", "effect_validator", "postcondition", "success-delivery-failure"} {
		t.Run(phase, func(t *testing.T) {
			// Arrange.
			now := time.Now()
			store := &postHandlerClaimStore{OperationStore: NewMemoryOperationStore()}
			profile := postHandlerProfile(t, store, func() time.Time { return now })
			cause := errors.New("contract failure")
			calls, results := 0, 0
			spec := TypedToolSpec[NoSubject, NoScope, struct{}, string, string]{
				Name:        "write",
				Description: "Write",
				Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
					calls++
					return NewToolResult[string, string]("ok"), nil
				},
			}
			switch phase {
			case "result_validator":
				spec.ResultValidator = func(string) error { return cause }
			case "effect_validator":
				spec.EffectValidator = func([]string) error { return cause }
			case "postcondition":
				spec.Postcondition = func(ToolResult[string, string]) error { return cause }
			}
			tool, err := NewTypedTool(spec)
			require.NoError(t, err)
			execute := func(yield func(Chunk) error) error {
				return tool.Execute(
					context.Background(),
					NewRunEnv(nil, WithRunExecutionProfile(profile)),
					ToolInput{ArgsJSON: []byte(`{}`)},
					yield,
				)
			}
			deliveryErr := errors.New("delivery stopped")
			// Act.
			err = execute(func(Chunk) error { results++; return deliveryErr })
			record, found, inspectErr := store.Inspect(context.Background(), store.binding)
			repeatErr := execute(func(Chunk) error { return nil })
			// Assert.
			require.NoError(t, inspectErr)
			require.True(t, found)
			require.Equal(t, 1, calls)
			if phase == "success-delivery-failure" {
				require.ErrorIs(t, err, deliveryErr)
				require.Equal(t, OperationCompleted, record.State)
				require.NotEmpty(t, record.Result)
				require.NoError(t, repeatErr)
				require.Equal(t, 1, results)
			} else {
				var unknown *OperationOutcomeError
				require.ErrorAs(t, err, &unknown)
				require.True(t, unknown.DispatchInvoked)
				require.ErrorIs(t, err, cause)
				require.Equal(t, OperationUnknown, record.State)
				require.Empty(t, record.Result)
				require.ErrorContains(t, repeatErr, string(OperationUnknown))
				require.Zero(t, results)
			}
		})
	}
}

func TestPostHandlerArgumentFailuresRemainCorrectable(t *testing.T) {
	// Arrange.
	calls := 0
	tool, err := NewTypedTool(
		TypedToolSpec[NoSubject, NoScope, struct{}, string, string]{
			Name:         "args",
			Description:  "Args",
			ArgValidator: func(struct{}) error { return NewValidationError("fix args") },
			Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
				calls++
				return NewToolResult[string, string]("ok"), nil
			},
		},
	)
	require.NoError(t, err)
	// Act.
	err = tool.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	// Assert.
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.True(t, ClientCorrectable(te.Code))
	require.Zero(t, calls)
}

func TestAfterClaimPreDispatchFailureIsTruthful(t *testing.T) {
	for _, mode := range []string{"canceled", "expired"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: claim succeeds, then context/lease fails before invoke.
			now := time.Now()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := &postHandlerClaimStore{OperationStore: NewMemoryOperationStore()}
			if mode == "canceled" {
				store.afterClaim = cancel
			} else {
				store.afterClaim = func() { now = now.Add(2 * time.Minute) }
			}
			profile := postHandlerProfile(t, store, func() time.Time { return now })
			calls := 0
			tool, err := NewTool(
				"write",
				"Write",
				func(context.Context, *RunEnv, struct{}) (string, error) { calls++; return "ok", nil },
			)
			require.NoError(t, err)
			// Act.
			err = tool.Execute(
				ctx,
				NewRunEnv(nil, WithRunExecutionProfile(profile)),
				ToolInput{ArgsJSON: []byte(`{}`)},
				func(Chunk) error { return nil },
			)
			record, found, inspectErr := store.Inspect(context.Background(), store.binding)
			// Assert.
			var unknown *OperationOutcomeError
			require.ErrorAs(t, err, &unknown)
			require.False(t, unknown.DispatchInvoked)
			require.Contains(t, unknown.Error(), "after claim")
			require.NotContains(t, unknown.Error(), "after dispatch")
			if mode == "canceled" {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				var expired *OperationError
				require.ErrorAs(t, err, &expired)
				require.Equal(t, "claim_expired", expired.Kind)
			}
			require.NoError(t, inspectErr)
			require.True(t, found)
			require.Equal(t, OperationUnknown, record.State)
			require.Zero(t, calls)
		})
	}
}

func TestPostHandlerDeadlineCauseStaysNonretryable(t *testing.T) {
	for _, phase := range []string{"result_validator", "effect_validator", "postcondition"} {
		t.Run(phase, func(t *testing.T) {
			// Arrange: the host callback's timeout is a post-handler contract failure.
			calls := 0
			spec := TypedToolSpec[NoSubject, NoScope, struct{}, string, string]{
				Name:        "write",
				Description: "Write",
				Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
					calls++
					return NewToolResult[string, string]("ok"), nil
				},
			}
			switch phase {
			case "result_validator":
				spec.ResultValidator = func(string) error { return context.DeadlineExceeded }
			case "effect_validator":
				spec.EffectValidator = func([]string) error { return context.DeadlineExceeded }
			case "postcondition":
				spec.Postcondition = func(ToolResult[string, string]) error { return context.DeadlineExceeded }
			}
			tool, err := NewTypedTool(spec)
			require.NoError(t, err)
			reg, err := NewRegistry(tool)
			require.NoError(t, err)
			// Act.
			err = reg.Execute(
				context.Background(),
				ToolCall{ToolName: "write", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
				func(Chunk) error { return nil },
			)
			// Assert.
			te, ok := AsToolError(err)
			require.True(t, ok)
			require.Equal(t, CodeInternal, te.Code)
			require.False(t, te.Retryable)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Equal(t, 1, calls)
		})
	}
}

func TestPostHandlerFormatterPreservesRecovery(t *testing.T) {
	for _, cause := range []error{errors.New("failure"), context.DeadlineExceeded, context.Canceled} {
		for _, journal := range []bool{false, true} {
			// Arrange.
			var contract error = &ResultContractError{Kind: "result_validator", Cause: cause}
			if journal {
				contract = &OperationOutcomeError{Cause: contract, DispatchInvoked: true}
			}
			err := NewInternalError(contract)
			// Act.
			chunk := NewErrorChunkFromErr(err)
			wire, wireErr := unmarshalToolErrorWire(chunk.Data)
			summary := ErrorChunkSummaryText(chunk, err)
			// Assert.
			require.NoError(t, wireErr)
			require.Equal(t, CodeInternal, wire.Code)
			require.False(t, wire.Retryable)
			require.Empty(t, wire.FixableArgs)
			require.Contains(t, summary, "reconciliation")
			require.NotContains(t, summary, "Retry later")
			require.NotContains(t, summary, "Fix the tool arguments")
		}
	}
}

func TestPostHandlerFormatterBypassAndNestedHandler(t *testing.T) {
	for _, cause := range []error{errors.New("failure"), context.DeadlineExceeded} {
		// Arrange: a tool returns the known outcome from a nested invocation.
		original := NewInternalError(&ResultContractError{Kind: "postcondition", Cause: cause})
		tool, err := NewTool(
			"nested",
			"Nested",
			func(context.Context, *RunEnv, struct{}) (string, error) { return "", original },
		)
		require.NoError(t, err)
		wrapped := WithErrorFormatter()(tool)
		chunks := 0
		// Act.
		err = wrapped.Execute(
			context.Background(),
			NewRunEnv(nil),
			ToolInput{ArgsJSON: []byte(`{}`)},
			func(Chunk) error { chunks++; return nil },
		)
		// Assert.
		te, ok := AsToolError(err)
		require.True(t, ok)
		require.Equal(t, CodeInternal, te.Code)
		require.False(t, te.Retryable)
		require.ErrorIs(t, err, cause)
		require.Zero(t, chunks)
	}
}

func TestPostHandlerDiagnosticCausesDoNotControlRouting(t *testing.T) {
	for _, cause := range []error{ErrPause, ErrYield, ErrHalt, ErrHostEvent, ErrStreamAborted, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			// Arrange: a callback embeds a control/interrupt cause in a repairable error.
			failure := &ToolError{Code: CodeValidationFailed, Retryable: true, Err: cause}
			calls := 0
			tool, err := NewTypedTool(
				TypedToolSpec[NoSubject, NoScope, struct{}, string, string]{
					Name:        "write",
					Description: "Write",
					Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
						calls++
						return NewToolResult[string, string]("ok"), nil
					},
					Postcondition: func(ToolResult[string, string]) error { return failure },
				},
			)
			require.NoError(t, err)
			reg, err := NewRegistry(tool)
			require.NoError(t, err)
			call := ToolCall{ToolName: "write", Input: ToolInput{ArgsJSON: []byte(`{}`)}}
			var iteratorErr error
			var chunks []Chunk
			// Act: iterator and batch must expose contract failure rather than hide it.
			for _, err := range reg.ExecuteIter(context.Background(), call) {
				iteratorErr = err
			}
			batchErr := reg.ExecuteBatchStream(
				context.Background(),
				[]ToolCall{call},
				func(c Chunk) error { chunks = append(chunks, c); return nil },
			)
			session, err := NewSession(reg)
			require.NoError(t, err)
			outcome, sessionErr := session.RunCall(context.Background(), call)
			// Assert.
			require.ErrorIs(t, iteratorErr, cause)
			require.False(t, IsControlError(iteratorErr))
			require.NoError(t, batchErr)
			require.Len(t, chunks, 1)
			require.True(t, chunks[0].IsError)
			require.Contains(t, ErrorChunkSummaryText(chunks[0], nil), "reconciliation")
			require.ErrorIs(t, sessionErr, cause)
			require.Equal(t, OutcomeInfrastructureError, outcome.Status)
			require.Empty(t, outcome.Controls)
			require.Equal(t, 3, calls)
		})
	}
}
