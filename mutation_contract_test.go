package toolsy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequiredMutationConfiguration(t *testing.T) {
	// Arrange.
	session, env := newTestSessionEnv(t)
	var nilEnv *RunEnv
	var nilSession *Session
	cases := map[string]func() error{
		"put nil env":       func() error { return Put(nilEnv, "key", "value") },
		"put zero env":      func() error { return Put(&RunEnv{}, "key", "value") },
		"put empty key":     func() error { return Put(env, "", "value") },
		"state nil env":     func() error { return SetState(nilEnv, "key", "value") },
		"state zero env":    func() error { return SetState(&RunEnv{}, "key", "value") },
		"state DI only":     func() error { return SetState(NewRunEnv(nil), "key", "value") },
		"state empty key":   func() error { return SetState(env, "", "value") },
		"session nil":       func() error { return SetSessionState(nilSession, "key", "value") },
		"session empty key": func() error { return SetSessionState(session, "", "value") },
	}
	for name, mutation := range cases {
		t.Run(name, func(t *testing.T) {
			// Act.
			err := mutation()
			// Assert.
			require.ErrorIs(t, err, ErrMutationConfiguration)
			structured, ok := AsToolError(err)
			require.True(t, ok)
			assert.Equal(t, CodeInternal, structured.Code)
			assert.False(t, structured.Retryable)
			assert.Empty(t, structured.FixableArgs)
		})
	}
}

func TestRequiredMutationsKeepOptionalReadAndReferenceSemantics(t *testing.T) {
	// Arrange.
	session, env := newTestSessionEnv(t)
	clone := env.cloneForExecute(nil, nil)
	referenced := map[string]string{"host": "value"}
	// Act.
	require.NoError(t, Put(env, "dependency", referenced))
	require.NoError(t, SetState(clone, "state", referenced))
	require.NoError(t, Put(env, "nullable", (*string)(nil)))
	require.NoError(t, SetSessionState(session, "nullable", (*string)(nil)))
	// Assert: containers are synchronized, referenced BYOT identity stays host-owned.
	dependency, err := Require[map[string]string](clone, "dependency")
	require.NoError(t, err)
	state, ok := GetSessionState[map[string]string](session, "state")
	require.True(t, ok)
	referenced["host"] = "updated"
	assert.Equal(t, "updated", dependency["host"])
	assert.Equal(t, "updated", state["host"])
	_, present := Lookup[*string](env, "nullable")
	assert.False(t, present)
	_, present = GetState[*string](env, "nullable")
	assert.False(t, present)
	_, present = GetState[string](nil, "state")
	assert.False(t, present)
	_, present = GetSessionState[string](nil, "state")
	assert.False(t, present)
	_, err = Require[*string](env, "nullable")
	require.Error(t, err)
	// A session is not required for dependency mutation.
	require.NoError(t, Put(NewRunEnv(nil), "dependency", "value"))
}

func TestToolConstructorsRejectNilHandlers(t *testing.T) {
	// Arrange: each constructor gets otherwise usable schema/stream options.
	schema := []byte(`{"type":"object","properties":{}}`)
	constructors := map[string]func() (Tool, error){
		"dynamic empty spec": func() (Tool, error) { return NewDynamicToolFromSpec(DynamicToolSpec{}) },
		"proxy empty schema": func() (Tool, error) { return NewProxyTool("gate", "Gate", nil, nil) },
		"generic":            func() (Tool, error) { return NewTool[struct{}, string]("gate", "Gate", nil) },
		"stream":             func() (Tool, error) { return NewStreamTool[struct{}]("gate", "Gate", nil, WithIndependentStream()) },
		"proxy":              func() (Tool, error) { return NewProxyTool("gate", "Gate", schema, nil) },
		"dynamic": func() (Tool, error) {
			return NewDynamicToolFromSpec(
				DynamicToolSpec{Name: "gate", Description: "Gate", Schema: MapSchemaProvider{"type": "object"}},
			)
		},
		"typed": func() (Tool, error) {
			return NewTypedTool(
				TypedToolSpec[string, string, struct{}, string, string]{Name: "gate", Description: "Gate"},
			)
		},
		"policy spec": func() (Tool, error) {
			return NewPolicyToolFromSpec(
				ToolPolicyConstructorSpec[string, string, struct{}]{
					Name:          "gate",
					Description:   "Gate",
					RawJSONSchema: schema,
				},
			)
		},
	}
	for name, construct := range constructors {
		t.Run(name, func(t *testing.T) {
			// Act.
			tool, err := construct()
			// Assert.
			require.ErrorIs(t, err, ErrToolHandlerNil)
			assert.Nil(t, tool)
		})
	}
}

func TestPolicyWrapperRejectsNilBaseTool(t *testing.T) {
	var pointer *tool
	for name, base := range map[string]Tool{"nil": nil, "typednil": pointer} {
		t.Run(name, func(t *testing.T) {
			// Arrange.
			spec := ToolPolicySpec[string, string, struct{}]{Tool: base,
				ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[struct{}], error) {
					return ValidatedArgs[struct{}]{}, nil
				}}
			// Act.
			wrapped, err := NewPolicyTool(spec)
			// Assert.
			require.Error(t, err)
			assert.Nil(t, wrapped)
		})
	}
}
