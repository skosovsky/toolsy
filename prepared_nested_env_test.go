package toolsy

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparedHandlerEnvCannotBecomeDirectNestedBinding(t *testing.T) {
	// Arrange: an inherited parent manifest/profile must never identify another action.
	childCalls, profileCalls := 0, 0
	child, err := NewTool(
		"child",
		"Child",
		func(context.Context, *RunEnv, struct{}) (string, error) { childCalls++; return "child", nil },
	)
	require.NoError(t, err)
	parent, err := NewTool("parent", "Parent", func(ctx context.Context, env *RunEnv, _ struct{}) (string, error) {
		return "", child.Execute(
			ctx,
			env,
			ToolInput{ArgsJSON: []byte(`{}`)},
			func(Chunk) error { t.Fatal("raw nested child delivered"); return nil },
		)
	})
	require.NoError(t, err)
	profile := executionProfileFunc(
		func(_ context.Context, call PreparedCall, invoke InvocationHandler, yield func(Chunk) error) error {
			profileCalls++
			assert.Equal(t, "parent", call.Manifest.Name)
			return invoke(yield)
		},
	)
	reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(parent).Build()
	require.NoError(t, err)
	// Act.
	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "parent", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	// Assert: rejected before child binder/handler/profile can use parent identity.
	require.ErrorContains(t, err, "fresh RunEnv or scoped executor")
	assert.Zero(t, childCalls)
	assert.Equal(t, 1, profileCalls)
}

func TestDirectPreparedCallsOwnStateWithSharedHostEnvironment(t *testing.T) {
	// Arrange: independently invoked calls may share host dependency/session environment.
	tool, err := NewTool(
		"direct",
		"Direct",
		func(context.Context, *RunEnv, struct{}) (string, error) { return "ok", nil },
	)
	require.NoError(t, err)
	profile := executionProfileFunc(
		func(_ context.Context, _ PreparedCall, invoke InvocationHandler, yield func(Chunk) error) error {
			return invoke(yield)
		},
	)
	env := NewRunEnv(nil, WithRunExecutionProfile(profile))
	results := make(chan error, 8)
	var wg sync.WaitGroup
	// Act: concurrent calls clone private dispatch state rather than mutating the shared env.
	for range 8 {
		wg.Go(func() {
			results <- tool.Execute(context.Background(), env, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error { return nil })
		})
	}
	wg.Wait()
	close(results)
	// Assert: no stale marker rejects independent calls or races on caller-owned state.
	for executeErr := range results {
		require.NoError(t, executeErr)
	}
	assert.False(t, env.preparedDispatch)
}
