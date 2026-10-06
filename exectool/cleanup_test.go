package exectool

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

func TestCleanupCauseDoesNotReplaceExecutionClassification(t *testing.T) {
	for _, tc := range []struct {
		name      string
		primary   error
		secondary error
		code      toolsy.ErrorCode
	}{
		{"completed_cleanup_deadline", nil, context.DeadlineExceeded, toolsy.CodeInternal},
		{"completed_cleanup_cancel", nil, context.Canceled, toolsy.CodeInternal},
		{"timeout_cleanup_cancel", ErrTimeout, context.Canceled, toolsy.CodeTimeout},
		{"output_limit_cleanup_deadline", textprocessor.NewReadLimitError("stdout", 1, nil), context.DeadlineExceeded, toolsy.CodeValidationFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: secondary cleanup causes are inspectable without reclassifying execution.
			cleanup := &CleanupError{Backend: "fixture", Operation: "remove", Cause: tc.secondary}
			runErr := errors.Join(tc.primary, cleanup)
			tool, err := New(&mockSandbox{languages: []string{"test"}, err: runErr})
			require.NoError(t, err)
			yields := 0
			// Act.
			err = tool.Execute(context.Background(), nil, toolsy.ToolInput{
				ArgsJSON: []byte(`{"language":"test","code":"effect"}`),
			}, func(toolsy.Chunk) error { yields++; return nil })
			// Assert: incomplete cleanup never emits a success; the primary category survives.
			var diagnostic *CleanupError
			require.ErrorAs(t, err, &diagnostic)
			require.ErrorIs(t, diagnostic.Cause, tc.secondary)
			require.ErrorIs(t, err, ErrSandboxCleanup)
			require.NotErrorIs(t, err, tc.secondary)
			toolError, ok := toolsy.AsToolError(err)
			require.True(t, ok)
			require.Equal(t, tc.code, toolError.Code)
			require.Zero(t, yields)
		})
	}
}
