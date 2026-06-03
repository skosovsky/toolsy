package toolsy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolError_Error(t *testing.T) {
	tests := []struct {
		name   string
		err    *ToolError
		expect string
	}{
		{"with reason", &ToolError{Code: CodeValidationFailed, Reason: "bad enum"}, "VALIDATION_FAILED: bad enum"},
		{"empty reason", &ToolError{Code: CodeInternal}, "INTERNAL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, tt.err.Error())
		})
	}
}

func TestToolError_Unwrap(t *testing.T) {
	inner := errors.New("db connection refused")
	err := NewInternalError(inner)
	assert.Same(t, inner, err.Unwrap())
}

func TestErrorsIs_As(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		target   error
		is       bool
		asClient bool
		asSystem bool
	}{
		{"validation direct", NewValidationError("x"), ErrValidation, true, true, false},
		{"internal direct", NewInternalError(ErrTimeout), ErrTimeout, true, false, true},
		{"wrapped validation", wrapError{err: NewValidationError("y")}, nil, false, true, false},
		{"wrapped internal", wrapError{err: NewInternalError(ErrTimeout)}, ErrTimeout, true, false, true},
		{"tool not found", NewToolNotFoundError(), ErrToolNotFound, true, true, false},
		{"timeout", NewTimeoutError(true), ErrTimeout, true, false, true},
		{"shutdown", NewShutdownError(), ErrShutdown, true, false, true},
		{"max steps", NewMaxStepsExceededError(), ErrMaxStepsExceeded, true, false, true},
		{"registry state", NewRegistryStateError(), ErrRegistryState, true, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.target != nil {
				assert.Equal(t, tt.is, errors.Is(tt.err, tt.target), "errors.Is")
			}
			te, ok := AsToolError(tt.err)
			require.True(t, ok)
			assert.Equal(t, tt.asClient, ClientCorrectable(te.Code), "ClientCorrectable")
			assert.Equal(t, tt.asSystem, orchestratorSystemCode(te.Code), "orchestrator system code")
		})
	}
}

func TestNewToolsContractMissingError(t *testing.T) {
	err := NewToolsContractMissingError([]string{"a", "b"}, []string{"b"})
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeToolsContractMissing, te.Code)
	require.Equal(t, []string{"b"}, te.FixableArgs)
}

func TestNewToolNotFoundInSubsetError(t *testing.T) {
	err := NewToolNotFoundInSubsetError("missing")
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeToolNotFound, te.Code)
	require.ErrorIs(t, err, ErrToolNotFound)
	assert.Contains(t, te.Reason, "missing")
}

func TestClientCorrectable(t *testing.T) {
	require.True(t, ClientCorrectable(CodeValidationFailed))
	require.False(t, ClientCorrectable(CodeInternal))
	te, ok := AsToolError(wrapError{err: NewValidationError("y")})
	require.True(t, ok)
	require.True(t, ClientCorrectable(te.Code))
}

func TestNewDependencyMissingError(t *testing.T) {
	err := NewDependencyMissingError("db")
	require.Equal(t, CodeDependencyMissing, err.Code)
	require.False(t, ClientCorrectable(err.Code))
}

func requireClientCorrectable(t *testing.T, err error) {
	t.Helper()
	te, ok := AsToolError(err)
	require.True(t, ok, "expected ToolError, got %v", err)
	require.True(t, ClientCorrectable(te.Code), "code %s", te.Code)
}

func requireSystemToolError(t *testing.T, err error) {
	t.Helper()
	te, ok := AsToolError(err)
	require.True(t, ok, "expected ToolError, got %v", err)
	require.True(t, orchestratorSystemCode(te.Code), "code %s", te.Code)
}

func requireToolErrorCode(t *testing.T, err error, code ErrorCode, sentinels ...error) {
	t.Helper()
	te, ok := AsToolError(err)
	require.True(t, ok, "expected ToolError, got %v", err)
	require.Equal(t, code, te.Code)
	for _, s := range sentinels {
		require.ErrorIs(t, err, s)
	}
}

type wrapError struct {
	err error
}

func (e wrapError) Error() string {
	if e.err == nil {
		return ""
	}
	return "wrap: " + e.err.Error()
}
func (e wrapError) Unwrap() error { return e.err }
