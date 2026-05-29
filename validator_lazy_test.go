package toolsy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithValidatorFromRegistry_LazyBinding(t *testing.T) {
	tool := newMiddlewareMinTool("x", func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
		return nil
	})

	var built *Registry
	builder := NewRegistryBuilder().Add(tool)
	provider := func() (*Registry, error) {
		if built == nil {
			return nil, errors.New("registry not built yet")
		}
		return built, nil
	}

	reg, err := builder.
		WithOptions(WithValidatorFromRegistry(provider, func(r *Registry) Validator {
			return &testValidator{
				validateFn: func(_ context.Context, toolName string, _ string) error {
					if !r.Has(toolName) {
						return errors.New("unknown tool")
					}
					return nil
				},
			}
		})).
		Build()
	require.NoError(t, err)
	built = reg

	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "x",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	require.NoError(t, err)
}

func TestWithValidatorFromRegistry_RejectsRecursiveDelegate(t *testing.T) {
	tool := newMiddlewareMinTool("x", func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
		return nil
	})
	var built *Registry
	provider := func() (*Registry, error) { return built, nil }

	reg, err := NewRegistryBuilder().
		Add(tool).
		WithOptions(WithValidatorFromRegistry(provider, func(r *Registry) Validator {
			return r.opts.validator
		})).
		Build()
	require.NoError(t, err)
	built = reg

	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "x",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lazy validator delegate must not return another lazy validator")
}
