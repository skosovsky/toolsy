package toolsy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientError(t *testing.T) {
	tests := []struct {
		name   string
		err    *ClientError
		expect string
	}{
		{"with reason", &ClientError{Reason: "bad enum"}, "invalid tool input: bad enum"},
		{"empty reason", &ClientError{Reason: ""}, "invalid tool input: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, tt.err.Error())
		})
	}
}

func TestSystemError(t *testing.T) {
	inner := errors.New("db connection refused")
	err := &SystemError{Err: inner}
	assert.Equal(t, "internal system error during tool execution", err.Error())
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
		{"ClientError direct", &ClientError{Reason: "x"}, ErrValidation, false, true, false},
		{"SystemError direct", &SystemError{Err: ErrTimeout}, ErrTimeout, true, false, true},
		{"wrapped ClientError", wrapErr{err: &ClientError{Reason: "y"}}, nil, false, true, false},
		{"wrapped SystemError", wrapErr{err: &SystemError{Err: ErrTimeout}}, ErrTimeout, true, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.target != nil {
				assert.Equal(t, tt.is, errors.Is(tt.err, tt.target), "errors.Is")
			}
			assert.Equal(t, tt.asClient, IsClientError(tt.err), "IsClientError")
			var ce *ClientError
			assert.Equal(t, tt.asClient, errors.As(tt.err, &ce))
			var se *SystemError
			assert.Equal(t, tt.asSystem, errors.As(tt.err, &se))
		})
	}
}

func TestIsClientError(t *testing.T) {
	require.True(t, IsClientError(&ClientError{Reason: "x"}))
	require.False(t, IsClientError(&SystemError{Err: errors.New("x")}))
	require.False(t, IsClientError(ErrToolNotFound))
	require.True(t, IsClientError(wrapErr{err: &ClientError{Reason: "y"}}))
}

func TestIsSystemError(t *testing.T) {
	require.True(t, IsSystemError(&SystemError{Err: errors.New("x")}))
	require.True(t, IsSystemError(wrapErr{err: &SystemError{Err: ErrTimeout}}))
	require.False(t, IsSystemError(&ClientError{Reason: "x"}))
	require.False(t, IsSystemError(ErrToolNotFound))
}

type wrapErr struct {
	err error
}

func (e wrapErr) Error() string {
	if e.err == nil {
		return ""
	}
	return "wrap: " + e.err.Error()
}
func (e wrapErr) Unwrap() error { return e.err }
