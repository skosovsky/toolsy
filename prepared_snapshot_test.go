package toolsy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type snapshotSubject struct {
	Claims map[string][]string
	Next   *snapshotSubject
	At     time.Time
}

func TestPreparedSnapshotPreservesBYOTAndIsolatesExportedFields(t *testing.T) {
	// Arrange: host-owned typed context includes nested mutable fields and cycles.
	subject := &snapshotSubject{Claims: map[string][]string{"roles": {"writer"}}, At: time.Now()}
	subject.Next = subject
	values := map[string]any{}
	values["self"] = values
	context := CallContext{Subject: subject, Scope: "tenant", Values: values}
	// Act: mutate only the prepared profile's snapshot.
	snapshot := preparedContextSnapshot(context)
	cloned, ok := snapshot.Subject.(*snapshotSubject)
	require.True(t, ok)
	cloned.Claims["roles"][0] = "admin"
	nested, ok := snapshot.Values["self"].(map[string]any)
	require.True(t, ok)
	nested["private mutation"] = true
	// Assert: domain types and cycles are preserved without changing host context.
	assert.Equal(t, "writer", subject.Claims["roles"][0])
	assert.Same(t, cloned, cloned.Next)
	assert.NotSame(t, subject, cloned)
	assert.Equal(t, subject.At, cloned.At)
	_, originalMutated := values["private mutation"]
	_, snapshotMutated := snapshot.Values["private mutation"]
	assert.False(t, originalMutated)
	assert.True(t, snapshotMutated)
}

func TestCompleteResultSnapshotIsolatesNestedTypedEffects(t *testing.T) {
	// Arrange.
	type effect struct{ Tags []string }
	chunk := Chunk{
		Event:       EventResult,
		TypedResult: effect{Tags: []string{"original"}},
		Effects:     []any{effect{Tags: []string{"original"}}},
		Envelope:    &ToolEnvelope{Result: effect{Tags: []string{"original"}}},
	}
	// Act.
	copyChunk := cloneResultChunk(chunk)
	chunk.TypedResult.(effect).Tags[0] = "mutated"
	chunk.Effects[0].(effect).Tags[0] = "mutated"
	chunk.Envelope.Result.(effect).Tags[0] = "mutated"
	// Assert: buffered/cache/journal result does not share producer-owned slices.
	assert.Equal(t, "original", copyChunk.TypedResult.(effect).Tags[0])
	assert.Equal(t, "original", copyChunk.Effects[0].(effect).Tags[0])
	assert.Equal(t, "original", copyChunk.Envelope.Result.(effect).Tags[0])
}

func TestPreparedBinderPreservesOpaqueHostProof(t *testing.T) {
	// Arrange: binder-owned immutable private data is not a JSON DTO field.
	type args struct {
		Value string `json:"value"`
		proof string
	}
	var calls int
	tool, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, args, string, string]{
		Name:        "bound",
		Description: "Bound",
		ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[args], error) {
			return ValidatedArgs[args]{
				Value: args{Value: "normalized", proof: "host authorized"},
				Raw:   []byte(`{"value":"normalized"}`),
			}, nil
		},
		Policy: func(_ context.Context, req TypedPolicyRequest[NoSubject, NoScope, args]) Decision {
			if req.Args.proof != "host authorized" {
				return DenyDecision("missing immutable host proof")
			}
			return AllowDecision()
		},
		Handler: func(_ context.Context, _ TypedCallContext[NoSubject, NoScope], _ *RunEnv, bound ValidatedArgs[args]) (ToolResult[string, string], error) {
			calls++
			assert.Equal(t, "host authorized", bound.Value.proof)
			return NewToolResult[string, string](bound.Value.Value), nil
		},
	})
	require.NoError(t, err)
	profile := executionProfileFunc(
		func(_ context.Context, call PreparedCall, invoke InvocationHandler, yield func(Chunk) error) error {
			assert.JSONEq(t, `{"value":"normalized"}`, string(call.Input.ArgsJSON))
			return invoke(yield)
		},
	)
	reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(tool).Build()
	require.NoError(t, err)
	// Act.
	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "bound", Input: ToolInput{ArgsJSON: []byte(`{"value":"raw"}`)}},
		func(Chunk) error { return nil },
	)
	// Assert: defensive copying did not erase private domain state.
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}
