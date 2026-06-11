package toolsy

import (
	"context"
	"errors"
)

// ArgValidator validates typed arguments after schema parse (Layer 2+).
type ArgValidator[T any] func(T) error

// ResultValidator validates typed results before marshaling (postcondition).
type ResultValidator[R any] func(R) error

// TypedToolSpec describes a first-class typed tool contract.
type TypedToolSpec[T, R any] struct {
	Name, Description string
	ArgValidator      ArgValidator[T]
	ResultValidator   ResultValidator[R]
	Handler           func(ctx context.Context, env *RunEnv, args T) (R, error)
	Options           []ToolOption
}

// NewTypedTool builds a [Tool] from [TypedToolSpec] with optional arg/result validators.
func NewTypedTool[T, R any](spec TypedToolSpec[T, R]) (Tool, error) {
	if spec.Handler == nil {
		return nil, errTypedToolNilHandler
	}
	name := spec.Name
	desc := spec.Description
	handler := spec.Handler
	argVal := spec.ArgValidator
	resVal := spec.ResultValidator

	return NewTool[T, R](name, desc, func(ctx context.Context, env *RunEnv, args T) (R, error) {
		if argVal != nil {
			if err := argVal(args); err != nil {
				var zero R
				return zero, wrapArgValidatorError(err)
			}
		}
		res, err := handler(ctx, env, args)
		if err != nil {
			var zero R
			return zero, err
		}
		if resVal != nil {
			if err := resVal(res); err != nil {
				var zero R
				return zero, wrapResultValidatorError(err)
			}
		}
		return res, nil
	}, spec.Options...)
}

var errTypedToolNilHandler = errors.New("toolsy: typed tool handler must not be nil")

func wrapArgValidatorError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := AsToolError(err); ok {
		return err
	}
	return NewValidationError(err.Error())
}

func wrapResultValidatorError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := AsToolError(err); ok {
		return err
	}
	return NewValidationError("result validation failed: " + err.Error())
}
