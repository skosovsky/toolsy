package toolsy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
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

func withPayloadStateCodec(t *testing.T) SessionOption {
	t.Helper()
	codecs := NewStateCodecRegistry()
	require.NoError(t, RegisterJSONCodec[sessionStatePayload](codecs, "payload"))
	return WithStateCodecRegistry(codecs)
}

func withCounterStateCodec(t *testing.T) SessionOption {
	t.Helper()
	codecs := NewStateCodecRegistry()
	require.NoError(t, RegisterJSONCodec[sessionStatePayload](codecs, "counter"))
	return WithStateCodecRegistry(codecs)
}

func TestSessionState_ExportImportSnapshotRoundtripViaJSON(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t, withPayloadStateCodec(t))

	want := sessionStatePayload{
		Name:   "agent",
		Count:  3,
		Nested: map[string]int{"a": 1},
	}
	SetSessionState(sess, "payload", want)

	snap, err := sess.ExportSnapshot()
	require.NoError(t, err)
	raw, err := json.Marshal(snap)
	require.NoError(t, err)

	restored, err := NewSessionSnapshotFromJSON(raw)
	require.NoError(t, err)

	sess2 := newTestSession(t, withPayloadStateCodec(t))
	require.NoError(t, sess2.ImportSnapshot(restored))

	got, ok := GetSessionState[sessionStatePayload](sess2, "payload")
	require.True(t, ok)
	require.Equal(t, want, got)

	env := NewRunEnv(sess2)
	viaEnv, ok := GetState[sessionStatePayload](env, "payload")
	require.True(t, ok)
	require.Equal(t, want, viaEnv)
}

func TestStateCodecRegistry_StructRoundtrip(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t, withPayloadStateCodec(t))

	want := sessionStatePayload{Name: "ptr", Count: 1}
	SetSessionState(sess, "payload", want)

	snap, err := sess.ExportSnapshot()
	require.NoError(t, err)
	raw, err := json.Marshal(snap)
	require.NoError(t, err)

	restored, err := NewSessionSnapshotFromJSON(raw)
	require.NoError(t, err)

	sess2 := newTestSession(t, withPayloadStateCodec(t))
	require.NoError(t, sess2.ImportSnapshot(restored))

	got, ok := GetSessionState[sessionStatePayload](sess2, "payload")
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestSessionState_ImportSnapshotRegisteredKey_InvalidPayloadPreservesState(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t, withPayloadStateCodec(t))
	SetSessionState(sess, "payload", sessionStatePayload{Name: "keep"})

	raw, err := json.Marshal(sessionSnapshotWire{
		Version: sessionSnapshotVersion,
		Payload: json.RawMessage(`{"payload":{"name":123,"count":"bad"}}`),
	})
	require.NoError(t, err)
	snap, err := NewSessionSnapshotFromJSON(raw)
	require.NoError(t, err)

	err = sess.ImportSnapshot(snap)
	requireToolErrorCode(t, err, CodeInternal)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.False(t, te.Retryable)

	got, ok := GetSessionState[sessionStatePayload](sess, "payload")
	require.True(t, ok)
	require.Equal(t, "keep", got.Name)
}

func TestSessionState_ImportSnapshotEmptyClears(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t)
	SetSessionState(sess, "k", "v")

	empty, err := newTestSession(t).ExportSnapshot()
	require.NoError(t, err)
	require.NoError(t, sess.ImportSnapshot(empty))

	_, ok := GetSessionState[string](sess, "k")
	require.False(t, ok)
}

func TestSessionState_ImportSnapshotUnregisteredStructRemainsGenericMap(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t)
	SetSessionState(sess, "payload", sessionStatePayload{Name: "raw"})

	snap, err := sess.ExportSnapshot()
	require.NoError(t, err)
	raw, err := json.Marshal(snap)
	require.NoError(t, err)

	restored, err := NewSessionSnapshotFromJSON(raw)
	require.NoError(t, err)

	sess2 := newTestSession(t)
	require.NoError(t, sess2.ImportSnapshot(restored))

	_, ok := GetSessionState[sessionStatePayload](sess2, "payload")
	require.False(t, ok)

	generic, ok := GetSessionState[map[string]any](sess2, "payload")
	require.True(t, ok)
	require.Equal(t, "raw", generic["name"])
}

func TestImportSnapshot_StrictMode_UnregisteredKeyFails(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t)
	SetSessionState(sess, "payload", sessionStatePayload{Name: "raw"})

	snap, err := sess.ExportSnapshot()
	require.NoError(t, err)

	sess2 := newTestSession(t, WithStrictStateCodecs(true))
	err = sess2.ImportSnapshot(snap)
	require.Error(t, err)
	requireToolErrorCode(t, err, CodeStateCodecMissing)
}

func TestImportSnapshot_StrictMode_EmptySnapshotClears(t *testing.T) {
	t.Parallel()
	codecs := NewStateCodecRegistry()
	require.NoError(t, RegisterJSONCodec[sessionStatePayload](codecs, "payload"))
	sess := newTestSession(t, WithStateCodecRegistry(codecs), WithStrictStateCodecs(true))
	SetSessionState(sess, "payload", sessionStatePayload{Name: "keep"})

	empty, err := newTestSession(t, WithStrictStateCodecs(true)).ExportSnapshot()
	require.NoError(t, err)
	require.NoError(t, sess.ImportSnapshot(empty))

	_, ok := GetSessionState[sessionStatePayload](sess, "payload")
	require.False(t, ok)
}

func TestImportSnapshot_StrictMode_NullKeyClearsWithoutCodec(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t, WithStrictStateCodecs(true))
	SetSessionState(sess, "orphan", "value")

	raw, err := json.Marshal(sessionSnapshotWire{
		Version: sessionSnapshotVersion,
		Payload: json.RawMessage(`{"orphan":null}`),
	})
	require.NoError(t, err)
	snap, err := NewSessionSnapshotFromJSON(raw)
	require.NoError(t, err)

	require.NoError(t, sess.ImportSnapshot(snap))
	_, ok := GetSessionState[string](sess, "orphan")
	require.False(t, ok)
}

func TestExportSnapshot_StrictMode_UnregisteredKeyFails(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t, WithStrictStateCodecs(true))
	SetSessionState(sess, "payload", sessionStatePayload{Name: "x"})

	_, err := sess.ExportSnapshot()
	require.Error(t, err)
	requireToolErrorCode(t, err, CodeStateCodecMissing)
}

func TestExportImportSnapshot_StrictMode_Roundtrip(t *testing.T) {
	t.Parallel()
	codecs := NewStateCodecRegistry()
	require.NoError(t, RegisterJSONCodec[sessionStatePayload](codecs, "payload"))
	opts := []SessionOption{WithStateCodecRegistry(codecs), WithStrictStateCodecs(true)}

	sess := newTestSession(t, opts...)
	want := sessionStatePayload{Name: "strict", Count: 2}
	SetSessionState(sess, "payload", want)

	snap, err := sess.ExportSnapshot()
	require.NoError(t, err)

	sess2 := newTestSession(t, opts...)
	require.NoError(t, sess2.ImportSnapshot(snap))

	got, ok := GetSessionState[sessionStatePayload](sess2, "payload")
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestSessionState_ConcurrentSetAndExportSnapshot(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t)
	done := make(chan struct{})
	go func() {
		for i := range 50 {
			SetSessionState(sess, "k", i)
		}
		close(done)
	}()
	for range 50 {
		_, _ = sess.ExportSnapshot()
	}
	<-done
}

func TestStateCodecRegistry_RegisterJSONCodec(t *testing.T) {
	t.Parallel()
	codecs := NewStateCodecRegistry()
	require.NoError(t, RegisterJSONCodec[sessionStatePayload](codecs, "payload"))
	sess := newTestSession(t, WithStateCodecRegistry(codecs))

	want := sessionStatePayload{Name: "codec", Count: 2}
	SetSessionState(sess, "payload", want)

	snap, err := sess.ExportSnapshot()
	require.NoError(t, err)

	sess2 := newTestSession(t, WithStateCodecRegistry(codecs))
	require.NoError(t, sess2.ImportSnapshot(snap))

	got, ok := GetSessionState[sessionStatePayload](sess2, "payload")
	require.True(t, ok)
	assert.Equal(t, want, got)
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

func TestNewSessionSnapshotFromJSON_InvalidWire(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  []byte
	}{
		{name: "not json", raw: []byte(`not json`)},
		{name: "zero version", raw: []byte(`{"version":0,"payload":{}}`)},
		{name: "missing payload", raw: []byte(`{"version":1}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewSessionSnapshotFromJSON(tc.raw)
			requireToolErrorCode(t, err, CodeInternal)
			te, ok := AsToolError(err)
			require.True(t, ok)
			assert.False(t, te.Retryable)
		})
	}
}

func TestImportSnapshot_EmptySnapshot(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t)
	SetSessionState(sess, "keep", "original")

	err := sess.ImportSnapshot(SessionSnapshot{})
	requireToolErrorCode(t, err, CodeInternal)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.False(t, te.Retryable)

	got, ok := GetSessionState[string](sess, "keep")
	require.True(t, ok)
	require.Equal(t, "original", got)
}

func TestImportSnapshot_UnsupportedVersion(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(sessionSnapshotWire{
		Version: 99,
		Payload: json.RawMessage(`{"k":"v"}`),
	})
	require.NoError(t, err)
	snap, err := NewSessionSnapshotFromJSON(raw)
	require.NoError(t, err)

	sess := newTestSession(t)
	SetSessionState(sess, "keep", "original")
	err = sess.ImportSnapshot(snap)
	require.Error(t, err)
	requireToolErrorCode(t, err, CodeInternal)

	got, ok := GetSessionState[string](sess, "keep")
	require.True(t, ok)
	require.Equal(t, "original", got)
}

func TestStateCodecRegistry_JSONRoundtrip(t *testing.T) {
	t.Parallel()
	codecs := NewStateCodecRegistry()
	require.NoError(t, RegisterJSONCodec[sessionStatePayload](codecs, "payload"))
	sess := newTestSession(t, WithStateCodecRegistry(codecs))

	want := sessionStatePayload{Name: "roundtrip", Count: 7}
	SetSessionState(sess, "payload", want)

	snap, err := sess.ExportSnapshot()
	require.NoError(t, err)
	raw, err := json.Marshal(snap)
	require.NoError(t, err)

	restored, err := NewSessionSnapshotFromJSON(raw)
	require.NoError(t, err)

	sess2 := newTestSession(t, WithStateCodecRegistry(codecs))
	require.NoError(t, sess2.ImportSnapshot(restored))

	got, ok := GetSessionState[sessionStatePayload](sess2, "payload")
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestNewSession_DuplicateCodecKey(t *testing.T) {
	t.Parallel()
	codecs := NewStateCodecRegistry()
	require.NoError(t, RegisterJSONCodec[sessionStatePayload](codecs, "payload"))
	require.Error(t, RegisterJSONCodec[sessionStatePayload](codecs, "payload"))

	reg, err := NewRegistryBuilder().Build()
	require.NoError(t, err)
	_, err = NewSession(reg, WithStateCodecRegistry(codecs))
	require.NoError(t, err)
}

func TestSessionState_ConcurrentImportExport(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t, withCounterStateCodec(t))
	sess2 := newTestSession(t, withCounterStateCodec(t))

	done := make(chan struct{})
	go func() {
		for i := range 30 {
			SetSessionState(sess, "counter", sessionStatePayload{Name: "writer", Count: i})
		}
		close(done)
	}()

	for range 30 {
		snap, err := sess.ExportSnapshot()
		if err == nil {
			_ = sess2.ImportSnapshot(snap)
		}
	}
	<-done
}

func TestSessionExportSnapshot_StateCodecTypeMismatch(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t, withPayloadStateCodec(t))
	sess.stateMu.Lock()
	sess.state["payload"] = "wrong-type"
	sess.stateMu.Unlock()

	_, err := sess.ExportSnapshot()
	requireToolErrorCode(t, err, CodeInternal)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.False(t, te.Retryable)
	assert.Contains(t, te.Reason, "type mismatch")
}

func TestSessionExportSnapshot_MarshalFailure(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t)
	sess.stateMu.Lock()
	sess.state["bad"] = make(chan int)
	sess.stateMu.Unlock()

	_, err := sess.ExportSnapshot()
	requireToolErrorCode(t, err, CodeInternal)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.False(t, te.Retryable)
	assert.Contains(t, te.Reason, "export state key")
}

func TestNewSessionSnapshotFromJSON_CorruptPayload(t *testing.T) {
	t.Parallel()
	_, err := NewSessionSnapshotFromJSON([]byte(`not json`))
	requireToolErrorCode(t, err, CodeInternal)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.False(t, te.Retryable)
}

type stringStateCodec struct{}

func (stringStateCodec) Encode(v string) ([]byte, error) {
	return json.Marshal(v)
}

func (stringStateCodec) Decode(data []byte) (string, error) {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return "", err
	}
	return s, nil
}

func TestRegisterStateCodec_CustomCodec(t *testing.T) {
	t.Parallel()
	codecs := NewStateCodecRegistry()
	require.NoError(t, RegisterStateCodec(codecs, "tag", stringStateCodec{}))
	sess := newTestSession(t, WithStateCodecRegistry(codecs))
	SetSessionState(sess, "tag", "hello")

	snap, err := sess.ExportSnapshot()
	require.NoError(t, err)

	sess2 := newTestSession(t, WithStateCodecRegistry(codecs))
	require.NoError(t, sess2.ImportSnapshot(snap))

	got, ok := GetSessionState[string](sess2, "tag")
	require.True(t, ok)
	assert.Equal(t, "hello", got)
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

func TestSessionExportSnapshot_NilSession(t *testing.T) {
	t.Parallel()
	var s *Session
	_, err := s.ExportSnapshot()
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeValidationFailed, te.Code)
}

func TestSessionExportSnapshot_ExcludesDeps(t *testing.T) {
	t.Parallel()
	sess := newTestSession(t)
	env := NewRunEnv(sess)
	Put(env, "db", mockHTTP{})
	SetSessionState(sess, "trace", "x")

	snap, err := sess.ExportSnapshot()
	require.NoError(t, err)
	raw, err := json.Marshal(snap)
	require.NoError(t, err)

	sess2 := newTestSession(t)
	restored, err := NewSessionSnapshotFromJSON(raw)
	require.NoError(t, err)
	require.NoError(t, sess2.ImportSnapshot(restored))

	got, ok := GetSessionState[string](sess2, "trace")
	require.True(t, ok)
	require.Equal(t, "x", got)

	env2 := NewRunEnv(sess2)
	_, err = Require[mockHTTP](env2, "db")
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeDependencyMissing, te.Code)
}
