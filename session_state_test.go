package toolsy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type sessionStatePayload struct {
	Name   string         `json:"name"`
	Count  int            `json:"count"`
	Nested map[string]int `json:"nested"`
}

func newTestSession(t *testing.T, opts ...SessionOption) *Session {
	t.Helper()
	reg, err := NewRegistryBuilder().Build()
	require.NoError(t, err)
	sess, err := NewSession(reg, opts...)
	require.NoError(t, err)
	return sess
}

func TestSessionState_ExportImportRoundtripViaJSON(t *testing.T) {
	t.Parallel()
	regTypes := NewStateTypeRegistry()
	require.NoError(t, regTypes.Register("payload", sessionStatePayload{}))
	sess := newTestSession(t, WithStateTypeRegistry(regTypes))

	want := sessionStatePayload{
		Name:   "agent",
		Count:  3,
		Nested: map[string]int{"a": 1},
	}
	SetSessionState(sess, "payload", want)

	export := sess.Export()
	raw, err := json.Marshal(export)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(raw, &wire))

	sess2 := newTestSession(t, WithStateTypeRegistry(regTypes))
	require.NoError(t, sess2.Import(wire))

	got, ok := GetSessionState[sessionStatePayload](sess2, "payload")
	require.True(t, ok)
	require.Equal(t, want, got)

	env := NewRunEnv(sess2)
	viaEnv, ok := GetState[sessionStatePayload](env, "payload")
	require.True(t, ok)
	require.Equal(t, want, viaEnv)
}

func TestStateTypeRegistry_RegisterPointerPrototype(t *testing.T) {
	t.Parallel()
	regTypes := NewStateTypeRegistry()
	require.NoError(t, regTypes.Register("payload", &sessionStatePayload{}))
	sess := newTestSession(t, WithStateTypeRegistry(regTypes))

	want := sessionStatePayload{Name: "ptr", Count: 1}
	SetSessionState(sess, "payload", want)

	export := sess.Export()
	raw, err := json.Marshal(export)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(raw, &wire))

	sess2 := newTestSession(t, WithStateTypeRegistry(regTypes))
	require.NoError(t, sess2.Import(wire))

	got, ok := GetSessionState[sessionStatePayload](sess2, "payload")
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestSessionState_ImportRegisteredKey_InvalidPayloadPreservesState(t *testing.T) {
	t.Parallel()
	regTypes := NewStateTypeRegistry()
	require.NoError(t, regTypes.Register("payload", sessionStatePayload{}))
	sess := newTestSession(t, WithStateTypeRegistry(regTypes))
	SetSessionState(sess, "payload", sessionStatePayload{Name: "keep"})

	err := sess.Import(map[string]any{
		"payload": map[string]any{"name": 123, "count": "bad"},
	})
	require.Error(t, err)

	got, ok := GetSessionState[sessionStatePayload](sess, "payload")
	require.True(t, ok)
	require.Equal(t, "keep", got.Name)
}

func TestSessionState_ImportNilClears(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t)
	SetSessionState(sess, "k", "v")
	require.NoError(t, sess.Import(nil))
	_, ok := GetSessionState[string](sess, "k")
	require.False(t, ok)
}

func TestValidateRunEnvSession_Mismatch(t *testing.T) {
	t.Parallel()
	s1 := newTestSession(t)
	s2 := newTestSession(t)
	env := NewRunEnv(s2)
	require.Error(t, ValidateRunEnvSession(s1, env))
}

func TestValidateRunEnvSession_NilSessionOrEnv(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t)
	env := NewRunEnv(sess)

	err := ValidateRunEnvSession(nil, env)
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeValidationFailed, te.Code)

	err = ValidateRunEnvSession(sess, nil)
	require.Error(t, err)
	te, ok = AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeValidationFailed, te.Code)
}

func TestSessionState_ImportUnregisteredStructRemainsGenericMap(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t)
	SetSessionState(sess, "payload", sessionStatePayload{Name: "raw"})

	export := sess.Export()
	raw, err := json.Marshal(export)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(raw, &wire))

	sess2 := newTestSession(t)
	require.NoError(t, sess2.Import(wire))

	_, ok := GetSessionState[sessionStatePayload](sess2, "payload")
	require.False(t, ok)

	generic, ok := GetSessionState[map[string]any](sess2, "payload")
	require.True(t, ok)
	require.Equal(t, "raw", generic["name"])
}

func TestSessionExecute_NilEnv_SetStateNoOp(t *testing.T) {
	t.Parallel()
	const stateKey = "written"
	tool := newMiddlewareMinTool("writer",
		func(_ context.Context, env *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			SetState(env, stateKey, true)
			return nil
		},
	)
	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	err = sess.Execute(context.Background(), ToolCall{
		ToolName: "writer",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      nil,
	}, func(Chunk) error { return nil })
	require.NoError(t, err)

	_, ok := GetSessionState[bool](sess, stateKey)
	require.False(t, ok)
}

func TestSessionExecute_RejectsForeignRunEnv(t *testing.T) {
	t.Parallel()
	tool := newMiddlewareMinTool("t", func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
		return nil
	})
	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)

	s1, err := NewSession(reg)
	require.NoError(t, err)
	s2, err := NewSession(reg)
	require.NoError(t, err)
	env := NewRunEnv(s2)

	err = s1.Execute(context.Background(), ToolCall{
		ToolName: "t",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      env,
	}, func(Chunk) error { return nil })
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeValidationFailed, te.Code)
}

func TestStateTypeRegistry_RegisterInvalid(t *testing.T) {
	t.Parallel()
	reg := NewStateTypeRegistry()
	require.Error(t, reg.Register("", sessionStatePayload{}))
	require.Error(t, reg.Register("k", nil))
	var nilReg *StateTypeRegistry
	require.Error(t, nilReg.Register("k", sessionStatePayload{}))
}

func TestSessionExport_NilSession(t *testing.T) {
	t.Parallel()
	var s *Session
	export := s.Export()
	require.NotNil(t, export)
	require.Empty(t, export)
}

func TestSessionState_ConcurrentSetAndExport(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t)
	const key = "n"
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 100 {
			SetSessionState(sess, key, i)
		}
	}()
	for range 100 {
		_ = sess.Export()
	}
	<-done
	_, ok := GetSessionState[int](sess, key)
	require.True(t, ok)
}

func TestSessionExport_ExcludesDeps(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t)
	env := NewRunEnv(sess)
	Put(env, "db", mockHTTP{})
	SetSessionState(sess, "trace", "x")

	export := sess.Export()
	require.Contains(t, export, "trace")
	require.NotContains(t, export, "db")
}
