package toolsy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

type uncertainActionError struct{ cause error }

func (e *uncertainActionError) Error() string { return "external action outcome unknown" }
func (e *uncertainActionError) Unwrap() error { return e.cause }

func TestRegistryPreservesExplicitNonRetryableTimeoutAndObservationCause(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		code  ErrorCode
	}{
		{"timeout", context.DeadlineExceeded, CodeTimeout},
		{"observation_limit", textprocessor.NewReadLimitError("remote observation", 16, nil), CodeValidationFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: an adapter dispatched an external action and cannot confirm its outcome.
			uncertain := &uncertainActionError{cause: tc.cause}
			dispatches := 0
			proxy, err := NewProxyTool("remote", "remote", []byte(`{"type":"object"}`),
				func(context.Context, *RunEnv, []byte, func(Chunk) error) error {
					dispatches++
					if tc.code == CodeTimeout {
						return NewTimeoutErrorFrom(uncertain, false)
					}
					return uncertain
				})
			require.NoError(t, err)
			registry, err := NewRegistryBuilder().Add(proxy).Build()
			require.NoError(t, err)
			delivered := 0
			// Act.
			err = registry.Execute(
				context.Background(),
				ToolCall{ToolName: "remote", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
				func(Chunk) error { delivered++; return nil },
			)
			// Assert: normalization retains the adapter's evidence and forbids blind retry.
			require.Error(t, err)
			typed, ok := AsToolError(err)
			require.True(t, ok)
			require.Equal(t, tc.code, typed.Code)
			require.False(t, typed.Retryable)
			var actual *uncertainActionError
			require.ErrorAs(t, err, &actual)
			require.Same(t, uncertain, actual)
			require.ErrorIs(t, err, tc.cause)
			require.Equal(t, 1, dispatches)
			require.Zero(t, delivered)
		})
	}
}
