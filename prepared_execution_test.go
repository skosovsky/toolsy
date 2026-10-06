package toolsy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type executionProfileFunc func(context.Context, PreparedCall, InvocationHandler, func(Chunk) error) error

func (f executionProfileFunc) ExecutePrepared(
	ctx context.Context,
	call PreparedCall,
	invoke InvocationHandler,
	yield func(Chunk) error,
) error {
	return f(ctx, call, invoke, yield)
}

func TestPreparedBoundarySingleDispatch(t *testing.T) {
	// Arrange: a profile attempts to dispatch twice.
	var calls int
	tool, err := NewTool(
		"once",
		"Once",
		func(context.Context, *RunEnv, struct{}) (string, error) { calls++; return "ok", nil },
	)
	require.NoError(t, err)
	profile := executionProfileFunc(
		func(_ context.Context, _ PreparedCall, invoke InvocationHandler, yield func(Chunk) error) error {
			if invokeErr := invoke(yield); invokeErr != nil {
				return invokeErr
			}
			return invoke(yield)
		},
	)
	reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(tool).Build()
	require.NoError(t, err)
	// Act.
	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "once", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	// Assert.
	require.ErrorContains(t, err, "cannot dispatch twice")
	assert.Equal(t, 1, calls)
}

func TestPreparedBoundaryBindsEffectiveManifestAndCanonicalInput(t *testing.T) {
	// Arrange: an alias and a raw JSON tool. A profile mutates its own snapshot.
	var rawReceived string
	tool, err := NewProxyTool(
		"base",
		"Base",
		[]byte(`{"type":"object"}`),
		func(_ context.Context, _ *RunEnv, raw []byte, yield func(Chunk) error) error {
			rawReceived = string(raw)
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
	)
	require.NoError(t, err)
	var observed PreparedCall
	profile := executionProfileFunc(
		func(_ context.Context, call PreparedCall, invoke InvocationHandler, yield func(Chunk) error) error {
			observed = clonePreparedCall(call)
			call.Input.ArgsJSON[0] = 'x'
			call.Manifest.Parameters["type"] = "string"
			return invoke(yield)
		},
	)
	alias := OverrideTool(tool, WithNewName("alias"))
	reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(alias).Build()
	require.NoError(t, err)
	// Act.
	require.NoError(
		t,
		reg.Execute(
			context.Background(),
			ToolCall{ToolName: "alias", Input: ToolInput{ArgsJSON: []byte(`{ "b":2, "a":1 }`)}},
			func(Chunk) error { return nil },
		),
	)
	// Assert.
	assert.Equal(t, "alias", observed.Manifest.Name)
	assert.JSONEq(t, `{"a":1,"b":2}`, rawReceived)
	assert.Equal(t, "object", alias.Manifest().Parameters["type"])
}
