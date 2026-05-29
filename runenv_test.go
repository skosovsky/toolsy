package toolsy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type testRunEnv struct {
	Token string
}

func TestBindEnv_EnvFromContext(t *testing.T) {
	ctx := BindEnv(context.Background(), testRunEnv{Token: "abc"})
	env, ok := EnvFromContext[testRunEnv](ctx)
	require.True(t, ok)
	require.Equal(t, "abc", env.Token)

	_, wrong := EnvFromContext[string](ctx)
	require.False(t, wrong)
}

func TestMiddlewareBudget_UsesBindEnv(t *testing.T) {
	tool := newMiddlewareMinTool("t", func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
		return nil
	})
	tracker := &testBudgetTracker{
		allowFn: func(_ context.Context, _ ToolManifest, _ ToolInput) (bool, string, error) {
			return true, "", nil
		},
	}
	reg, err := NewRegistryBuilder().Use(WithBudget()).Add(tool).Build()
	require.NoError(t, err)

	ctx := BindEnv(context.Background(), BudgetEnv{Budget: tracker})
	err = reg.Execute(ctx, ToolCall{
		ToolName: "t",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	require.NoError(t, err)
}
