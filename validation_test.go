package toolsy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatable_NotImplemented(t *testing.T) {
	type Args struct {
		Low  int `json:"low"`
		High int `json:"high"`
	}
	args := &Args{Low: 10, High: 5}
	// Args does not implement Validatable; validateCustom should no-op
	err := validateCustom(args)
	assert.NoError(t, err)
}

// validatableArgs implements Validatable for tests.
type validatableArgs struct {
	Low  int `json:"low"`
	High int `json:"high"`
}

func (a validatableArgs) Validate() error {
	if a.Low > a.High {
		return errors.New("low must be <= high")
	}
	return nil
}

func TestValidatable_Implemented(t *testing.T) {
	tool, err := NewTool("validatable_tool", "desc", func(_ context.Context, _ validatableArgs) (struct{ Ok bool }, error) {
		return struct{ Ok bool }{Ok: true}, nil
	})
	require.NoError(t, err)
	// Valid: low <= high
	var res []byte
	err = tool.Execute(context.Background(), []byte(`{"low":1,"high":10}`), func(c Chunk) error {
		res = c.Data
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	// Invalid: low > high — Validatable.Validate returns error
	err = tool.Execute(context.Background(), []byte(`{"low":10,"high":5}`), func(Chunk) error { return nil })
	require.Error(t, err)
	res = nil
	assert.Nil(t, res)
	assert.True(t, IsClientError(err))
	assert.ErrorIs(t, err, ErrValidation)
}

// pointerValidatableArgs implements Validatable with pointer receiver only.
type pointerValidatableArgs struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

func (a *pointerValidatableArgs) Validate() error {
	if a.Min > a.Max {
		return errors.New("min must be <= max")
	}
	return nil
}

func TestValidatable_PointerReceiver(t *testing.T) {
	tool, err := NewTool("ptr_validatable", "desc", func(_ context.Context, _ pointerValidatableArgs) (struct{ Ok bool }, error) {
		return struct{ Ok bool }{Ok: true}, nil
	})
	require.NoError(t, err)
	// Valid: min <= max
	var res []byte
	err = tool.Execute(context.Background(), []byte(`{"min":1,"max":10}`), func(c Chunk) error {
		res = c.Data
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	// Invalid: min > max — Validatable.Validate (pointer receiver) returns error
	err = tool.Execute(context.Background(), []byte(`{"min":10,"max":5}`), func(Chunk) error { return nil })
	require.Error(t, err)
	res = nil
	assert.Nil(t, res)
	assert.True(t, IsClientError(err))
	assert.ErrorIs(t, err, ErrValidation)
}
