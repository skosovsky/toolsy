package toolsy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nilGatePolicy struct{}

func (*nilGatePolicy) Decide(context.Context, PolicyRequest) Decision { panic("nil policy invoked") }

type nilGateAuthorizer struct{}

func (*nilGateAuthorizer) Authorize(context.Context, PolicyRequest) error {
	panic("nil authorizer invoked")
}

func TestRequiredPolicyConfiguration(t *testing.T) {
	var pointer *nilGatePolicy
	var function PolicyFunc
	for name, policy := range map[string]Policy{"nil": nil, "pointer": pointer, "function": function} {
		t.Run(name, func(t *testing.T) {
			// Arrange: later valid options must not erase an explicitly broken gate.
			builder := NewRegistryBuilder().WithOptions(WithPolicy("required", policy),
				WithPolicy(
					"valid",
					PolicyFunc(func(context.Context, PolicyRequest) Decision { return AllowDecision() }),
				))
			// Act.
			reg, err := builder.Build()
			// Assert.
			require.ErrorIs(t, err, ErrPolicyConfiguration)
			assert.Nil(t, reg)
		})
	}
	// Arrange.
	builder := NewRegistryBuilder().WithOptions(WithRequirementsPolicy[any, any]("requirements", nil))
	// Act.
	_, err := builder.Build()
	// Assert.
	require.ErrorIs(t, err, ErrPolicyConfiguration)
}

func TestViewRejectsNilPolicyImplementations(t *testing.T) {
	// Arrange.
	reg, err := NewRegistryBuilder().Build()
	require.NoError(t, err)
	view, err := reg.View(RegistryViewSpec{})
	require.NoError(t, err)
	var pointer *nilGatePolicy
	var function PolicyFunc
	for name, policy := range map[string]Policy{"pointer": pointer, "function": function} {
		t.Run(name, func(t *testing.T) {
			// Act.
			_, createErr := reg.View(RegistryViewSpec{Policy: policy, PolicyID: "gate"})
			_, restoreErr := reg.RestoreView(view.Snapshot(), policy, "gate")
			// Assert.
			require.ErrorIs(t, createErr, ErrPolicyConfiguration)
			require.ErrorIs(t, restoreErr, ErrPolicyConfiguration)
		})
	}
}

func TestAuthorizerAdapterConfigurationAndCause(t *testing.T) {
	var pointer *nilGateAuthorizer
	var function AuthorizerFunc
	for name, authorizer := range map[string]Authorizer{"nil": nil, "pointer": pointer, "function": function} {
		t.Run(name, func(t *testing.T) {
			// Arrange / Act.
			policy, err := NewAuthorizerPolicy(authorizer)
			// Assert.
			require.ErrorIs(t, err, ErrPolicyConfiguration)
			assert.Nil(t, policy)
		})
	}
	// Arrange: even a repairable callback error must remain authorization denial.
	cause := NewValidationError("host denied", "secret")
	policy := mustAuthorizerPolicy(t, AuthorizerFunc(func(context.Context, PolicyRequest) error { return cause }))
	// Act.
	err := evaluatePolicy(context.Background(), policy, PolicyRequest{})
	// Assert.
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, ErrPolicyDenied)
	structured, ok := AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, CodePolicyDenied, structured.Code)
	assert.False(t, structured.Retryable)
	assert.Empty(t, structured.FixableArgs)
}

func TestBudgetGateConfigurationMatrix(t *testing.T) {
	var typedNil *testBudgetTracker
	for _, optional := range []bool{false, true} {
		for name, value := range map[string]any{"absent": nil, "nil": nil, "typednil": typedNil, "wrongtype": "tracker"} {
			t.Run(name+map[bool]string{false: "/required", true: "/optional"}[optional], func(t *testing.T) {
				// Arrange.
				env := configuredBudgetGateTestEnv(t, name, value)
				dispatched := 0
				tool := newMiddlewareMinTool(
					"gate",
					func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error {
						dispatched++
						return nil
					},
				)
				wrapped := WithBudget()(tool)
				if optional {
					wrapped = WithOptionalBudget()(tool)
				}
				// Act.
				err := wrapped.Execute(context.Background(), env, ToolInput{}, func(Chunk) error { return nil })
				// Assert.
				if optional && name == "absent" {
					require.NoError(t, err)
					assert.Equal(t, 1, dispatched)
					return
				}
				require.ErrorIs(t, err, ErrBudgetConfiguration)
				structured, ok := AsToolError(err)
				require.True(t, ok)
				assert.Equal(t, CodeInternal, structured.Code)
				assert.Equal(t, 0, dispatched)
			})
		}
	}
}

func TestBudgetCallbackSnapshotAndCancellation(t *testing.T) {
	// Arrange: callback can replace a dependency without holding the store lock.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := NewRunEnv(nil)
	input := ToolInput{ArgsJSON: []byte(`{"value":1}`)}
	dispatched := false
	tool := newMiddlewareMinTool("gate", func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error {
		dispatched = true
		return nil
	})
	tracker := &testBudgetTracker{
		allowFn: func(_ context.Context, _ ToolManifest, snapshot ToolInput) (bool, string, error) {
			snapshot.ArgsJSON[0] = '!'
			if mutationErr := Put(env, DepKeyBudget, "replaced"); mutationErr != nil {
				t.Error(mutationErr)
			}
			cancel()
			return true, "", nil
		},
	}
	if mutationErr := Put(env, DepKeyBudget, tracker); mutationErr != nil {
		t.Error(mutationErr)
	}
	// Act.
	err := WithBudget()(tool).Execute(ctx, env, input, func(Chunk) error { return nil })
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	assert.JSONEq(t, `{"value":1}`, string(input.ArgsJSON))
	assert.False(t, dispatched)
	assert.Equal(t, int64(1), tracker.calls.Load())
}

func TestBudgetCallbackErrorPreservesCause(t *testing.T) {
	// Arrange.
	cause := errors.New("budget service unavailable")
	tracker := &testBudgetTracker{allowFn: func(context.Context, ToolManifest, ToolInput) (bool, string, error) {
		return false, "", cause
	}}
	tool := newMiddlewareMinTool("gate", func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error {
		t.Fatal("failed budget callback dispatched handler")
		return nil
	})
	// Act.
	err := WithBudget()(
		tool,
	).Execute(context.Background(), budgetEnv(tracker), ToolInput{}, func(Chunk) error { return nil })
	// Assert.
	require.ErrorIs(t, err, cause)
}

func TestBudgetGateRunsBeforeCacheReplay(t *testing.T) {
	// Arrange.
	calls := 0
	tool, err := NewTool("cached_gate", "Budget gated cache", func(context.Context, *RunEnv, struct{}) (string, error) {
		calls++
		return "value", nil
	})
	require.NoError(t, err)
	cache := mustResultCache(t, func(context.Context, PreparedCall) (string, error) { return "host", nil })
	reg, err := NewRegistryBuilder(WithExecutionProfile(cache)).Use(WithBudget()).Add(tool).Build()
	require.NoError(t, err)
	tracker := &testBudgetTracker{}
	call := ToolCall{ToolName: "cached_gate", Input: ToolInput{ArgsJSON: []byte(`{}`)}, Env: budgetEnv(tracker)}
	runCacheCall(t, reg, call)
	runCacheCall(t, reg, call)
	// Act: valid cached data must not bypass a missing gate on another call.
	call.Env = NewRunEnv(nil)
	yielded := 0
	err = reg.Execute(context.Background(), call, func(Chunk) error { yielded++; return nil })
	// Assert.
	require.ErrorIs(t, err, ErrBudgetConfiguration)
	assert.Equal(t, 0, yielded)
	assert.Equal(t, 1, calls)
	assert.Equal(t, int64(2), tracker.calls.Load())
}

func TestAsyncBudgetMissingDependencyReportsCompletionError(t *testing.T) {
	// Arrange: registry places its middleware inside background execution.
	completed := make(chan error, 1)
	tool := newMiddlewareMinTool("async_gate", func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error {
		completed <- errors.New("unexpected dispatch")
		return nil
	})
	async := AsAsyncTool(tool, WithOnComplete(func(_ context.Context, _ string, _ []Chunk, err error) {
		completed <- err
	}))
	reg, err := NewRegistryBuilder().Use(WithBudget()).Add(async).Build()
	require.NoError(t, err)
	accepted := 0
	// Act.
	err = reg.Execute(context.Background(), ToolCall{ToolName: "async_gate", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { accepted++; return nil })
	// Assert: acceptance does not claim successful background admission.
	require.NoError(t, err)
	assert.Equal(t, 1, accepted)
	select {
	case completionErr := <-completed:
		require.ErrorIs(t, completionErr, ErrBudgetConfiguration)
	case <-time.After(5 * time.Second):
		t.Fatal("background budget rejection did not complete")
	}
	require.NoError(t, reg.Shutdown(context.Background()))
}

func configuredBudgetGateTestEnv(t *testing.T, name string, value any) *RunEnv {
	t.Helper()
	env := NewRunEnv(nil)
	if name != "absent" {
		require.NoError(t, Put(env, DepKeyBudget, value))
	}
	return env
}
