package toolsy

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockHTTP struct{}

type pingDB interface {
	Ping()
}

type pingDBImpl struct{}

func (pingDBImpl) Ping() {}

func TestPut_Require_TypedNilInterface(t *testing.T) {
	env := NewRunEnv(nil)
	var iface pingDB = (*pingDBImpl)(nil)
	Put(env, "db", iface)

	_, err := Require[pingDB](env, "db")
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeDependencyMissing, te.Code)
}

func TestPut_Require_Lookup_TypedNil(t *testing.T) {
	env := NewRunEnv(nil)
	var p *mockHTTP
	var i any = p
	Put(env, "db", i)

	_, err := Require[mockHTTP](env, "db")
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeDependencyMissing, te.Code)

	_, ok = Lookup[mockHTTP](env, "db")
	require.False(t, ok)
}

func TestSetState_GetState_RoundTrip(t *testing.T) {
	sess, err := NewSession(nil)
	require.NoError(t, err)
	env := NewRunEnv(sess)
	SetState(env, "trace", "abc-123")
	v, ok := GetState[string](env, "trace")
	require.True(t, ok)
	require.Equal(t, "abc-123", v)
}

func TestNamespaceIsolation_ClientKey(t *testing.T) {
	sess, err := NewSession(nil)
	require.NoError(t, err)
	env := NewRunEnv(sess)
	Put(env, "client", mockHTTP{})
	SetState(env, "client", "user-id")

	got, err := Require[mockHTTP](env, "client")
	require.NoError(t, err)
	_ = got

	s, ok := GetState[string](env, "client")
	require.True(t, ok)
	require.Equal(t, "user-id", s)
}

func TestMiddlewareBudget_UsesDepKey(t *testing.T) {
	tool := newMiddlewareMinTool("t", func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
		return nil
	})
	tracker := &testBudgetTracker{
		allowFn: func(_ context.Context, _ ToolManifest, _ ToolInput) (bool, string, error) {
			return true, "", nil
		},
	}
	reg, err := NewRegistryBuilder().Use(WithBudget()).Add(tool).Build()
	require.NoError(t, err)

	env := NewRunEnv(nil)
	Put(env, DepKeyBudget, tracker)
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "t",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      env,
	}, func(Chunk) error { return nil })
	require.NoError(t, err)
}

func TestSameRunEnv_OrchestratorToTool(t *testing.T) {
	sess, err := NewSession(nil)
	require.NoError(t, err)
	env := NewRunEnv(sess)
	SetState(env, "token", "shared")

	var seen string
	type tokenOut struct{ Token string }
	tool, err := NewTool("read_token", "read", func(_ context.Context, e *RunEnv, _ struct{}) (tokenOut, error) {
		v, ok := GetState[string](e, "token")
		if !ok {
			return tokenOut{}, NewValidationError("missing token")
		}
		seen = v
		return tokenOut{Token: v}, nil
	})
	require.NoError(t, err)

	reg, err := NewRegistry(tool)
	require.NoError(t, err)
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "read_token",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      env,
	}, func(Chunk) error { return nil })
	require.NoError(t, err)
	require.Equal(t, "shared", seen)
}

func TestRunEnv_CloneSharesStore_NoDataRace(t *testing.T) {
	sess, err := NewSession(nil)
	require.NoError(t, err)
	parent := NewRunEnv(sess)
	exec := parent.cloneForExecute(nil, nil)

	const key = "counter"
	SetState(parent, key, 0)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 200 {
			SetState(exec, key, i)
		}
	}()

	for range 200 {
		_, _ = GetState[int](parent, key)
	}
	<-done
	v, ok := GetState[int](parent, key)
	require.True(t, ok)
	require.GreaterOrEqual(t, v, 0)
}

// Ensure http import is used by mockHTTP if needed elsewhere.
var _ = http.MethodGet
