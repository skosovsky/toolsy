package toolsy

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionState_SetGet_RoundTrip(t *testing.T) {
	sess, err := NewSession(nil)
	require.NoError(t, err)
	SetSessionState(sess, "trace", "abc-123")
	v, ok := GetSessionState[string](sess, "trace")
	require.True(t, ok)
	require.Equal(t, "abc-123", v)
}

func TestSessionState_ExportImport_JSONRoundtrip(t *testing.T) {
	const key = "ctx"
	type executionContext struct {
		TraceID string `json:"trace_id"`
		Step    int    `json:"step"`
	}

	types := NewStateTypeRegistry()
	types.Register(key, executionContext{})

	sess, err := NewSession(nil, WithStateTypeRegistry(types))
	require.NoError(t, err)
	want := executionContext{TraceID: "t-1", Step: 3}
	SetSessionState(sess, key, want)

	raw, err := json.Marshal(sess.Export())
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))

	restored, err := NewSession(nil, WithStateTypeRegistry(types))
	require.NoError(t, err)
	require.NoError(t, restored.Import(payload))

	got, ok := GetSessionState[executionContext](restored, key)
	require.True(t, ok)
	assert.Equal(t, want, got)

	env := NewRunEnv(restored)
	gotEnv, ok := GetState[executionContext](env, key)
	require.True(t, ok)
	assert.Equal(t, want, gotEnv)
	assert.Same(t, restored, env.Session())
}

func TestSessionState_Export_ExcludesDeps(t *testing.T) {
	sess, err := NewSession(nil)
	require.NoError(t, err)
	env := NewRunEnv(sess)
	Put(env, "secret", 42)
	SetSessionState(sess, "visible", "yes")

	exp := sess.Export()
	assert.Equal(t, map[string]any{"visible": "yes"}, exp)
}

func TestSessionState_Import_RegisteredKeyUnmarshalError(t *testing.T) {
	types := NewStateTypeRegistry()
	types.Register("n", struct{ X int }{})

	sess, err := NewSession(nil, WithStateTypeRegistry(types))
	require.NoError(t, err)

	err = sess.Import(map[string]any{"n": "not-an-object"})
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, CodeValidationFailed, te.Code)
}

func TestSessionState_ConcurrentAccess(t *testing.T) {
	sess, err := NewSession(nil)
	require.NoError(t, err)
	const key = "counter"
	SetSessionState(sess, key, 0)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for i := range 50 {
				SetSessionState(sess, key, i)
			}
		})
	}
	wg.Wait()
	_, ok := GetSessionState[int](sess, key)
	assert.True(t, ok)
}

func TestGetState_NilEnv(t *testing.T) {
	_, ok := GetState[string](nil, "k")
	assert.False(t, ok)

	env := NewRunEnv(nil)
	_, ok = GetState[string](env, "k")
	assert.False(t, ok)
}

func TestGetSessionState_NilSession(t *testing.T) {
	_, ok := GetSessionState[string](nil, "k")
	assert.False(t, ok)
}

func TestSessionExecute_EnvSessionMismatch(t *testing.T) {
	reg, err := NewRegistryBuilder().Build()
	require.NoError(t, err)
	sessA, err := NewSession(reg)
	require.NoError(t, err)
	sessB, err := NewSession(reg)
	require.NoError(t, err)

	err = sessA.Execute(context.Background(), ToolCall{
		ToolName: "missing",
		Env:      NewRunEnv(sessB),
	}, func(Chunk) error { return nil })
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, CodeValidationFailed, te.Code)
	assert.Contains(t, err.Error(), "session/env mismatch")
}

func TestSessionState_Import_FailurePreservesState(t *testing.T) {
	types := NewStateTypeRegistry()
	types.Register("n", struct{ X int }{})

	sess, err := NewSession(nil, WithStateTypeRegistry(types))
	require.NoError(t, err)
	SetSessionState(sess, "keep", "before")

	err = sess.Import(map[string]any{"n": "bad", "keep": "after"})
	require.Error(t, err)

	v, ok := GetSessionState[string](sess, "keep")
	require.True(t, ok)
	assert.Equal(t, "before", v)
}

func TestGetState_EnvDelegatesToSession(t *testing.T) {
	sess, err := NewSession(nil)
	require.NoError(t, err)
	env := NewRunEnv(sess)
	SetState(env, "token", "via-env")
	v, ok := GetState[string](env, "token")
	require.True(t, ok)
	assert.Equal(t, "via-env", v)
}

func TestSession_Export_NilReceiver(t *testing.T) {
	assert.Nil(t, (*Session)(nil).Export())
}

func TestSession_Export_EmptySession(t *testing.T) {
	sess, err := NewSession(nil)
	require.NoError(t, err)
	exp := sess.Export()
	require.NotNil(t, exp)
	assert.Empty(t, exp)
}

func TestSession_Import_NilClearsState(t *testing.T) {
	sess, err := NewSession(nil)
	require.NoError(t, err)
	SetSessionState(sess, "k", "v")
	require.NoError(t, sess.Import(nil))
	_, ok := GetSessionState[string](sess, "k")
	assert.False(t, ok)
}

func TestSession_Import_NilReceiver(t *testing.T) {
	err := (*Session)(nil).Import(map[string]any{"k": "v"})
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, CodeValidationFailed, te.Code)
}

func TestValidateRunEnvSession(t *testing.T) {
	sessA, err := NewSession(nil)
	require.NoError(t, err)
	sessB, err := NewSession(nil)
	require.NoError(t, err)

	require.NoError(t, ValidateRunEnvSession(sessA, nil))
	require.NoError(t, ValidateRunEnvSession(sessA, NewRunEnv(nil)))
	require.NoError(t, ValidateRunEnvSession(sessA, NewRunEnv(sessA)))

	err = ValidateRunEnvSession(sessA, NewRunEnv(sessB))
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, CodeValidationFailed, te.Code)
}

func TestRegistryExecute_StateWrittenToEnvSessionNotExecutorSession(t *testing.T) {
	const key = "marker"
	sessA, err := NewSession(nil)
	require.NoError(t, err)
	sessB, err := NewSession(nil)
	require.NoError(t, err)
	SetSessionState(sessA, key, "on-a")

	tool, err := NewTool("write_marker", "write", func(_ context.Context, e *RunEnv, _ struct{}) (struct{}, error) {
		SetState(e, key, "from-tool")
		return struct{}{}, nil
	})
	require.NoError(t, err)
	reg, err := NewRegistry(tool)
	require.NoError(t, err)

	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "write_marker",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sessB),
	}, func(Chunk) error { return nil })
	require.NoError(t, err)

	vA, okA := GetSessionState[string](sessA, key)
	require.True(t, okA)
	assert.Equal(t, "on-a", vA)

	vB, okB := GetSessionState[string](sessB, key)
	require.True(t, okB)
	assert.Equal(t, "from-tool", vB)
}

func TestRunEnv_SessionAccessor(t *testing.T) {
	assert.Nil(t, (*RunEnv)(nil).Session())
	assert.Nil(t, NewRunEnv(nil).Session())
	sess, err := NewSession(nil)
	require.NoError(t, err)
	assert.Same(t, sess, NewRunEnv(sess).Session())
}

func TestSessionState_Export_ShallowCopySharesMutableValues(t *testing.T) {
	sess, err := NewSession(nil)
	require.NoError(t, err)
	inner := map[string]int{"a": 1}
	SetSessionState(sess, "m", inner)

	exp := sess.Export()
	inner["a"] = 99
	got, ok := GetSessionState[map[string]int](sess, "m")
	require.True(t, ok)
	assert.Equal(t, 99, got["a"])
	assert.Equal(t, 99, exp["m"].(map[string]int)["a"])
}
