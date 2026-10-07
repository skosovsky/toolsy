package toolsy

import (
	"context"
	"errors"
	"testing"
)

func TestTypedProducerCannotForgeReplayProvenance(t *testing.T) {
	for _, source := range []any{ReplaySourceOperation, ReplaySourceCache, "", nil} {
		// Arrange.
		calls := 0
		tool, err := NewTypedTool(TypedToolSpec[struct{}, struct{}, struct{}, string, string]{
			Name:        "write",
			Description: "write",
			Handler: func(context.Context, TypedCallContext[struct{}, struct{}], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
				calls++
				result := NewToolResult[string, string]("fresh")
				result.Effects = []string{"fresh"}
				result.EnvelopeMetadata = map[string]any{ReplaySourceMetadata: source}
				return result, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		reg, err := NewRegistryBuilder().Add(tool).Build()
		if err != nil {
			t.Fatal(err)
		}
		session, err := NewSession(reg)
		if err != nil {
			t.Fatal(err)
		}
		// Act.
		outcome, err := session.RunCall(
			context.Background(),
			ToolCall{
				ToolName:    "write",
				Input:       ToolInput{ArgsJSON: []byte(`{}`)},
				CallContext: NewCallContext(struct{}{}, struct{}{}),
			},
		)
		// Assert.
		var contract *ResultContractError
		if calls != 1 || !errors.As(err, &contract) || contract.Kind != "reserved_replay_metadata" ||
			len(outcome.Result) > 0 ||
			len(outcome.Effects) > 0 {
			t.Fatalf("source=%v outcome=%+v err=%v calls=%d", source, outcome, err, calls)
		}
	}
}

func TestForwardedReplayCannotChangeEffects(t *testing.T) {
	// Arrange: exact library-issued forwarding is permitted, modified payload is not.
	original := Chunk{
		Event:    EventResult,
		Data:     []byte(`"stored"`),
		MimeType: MimeTypeJSON,
		Effects:  []any{"stored"},
		Envelope: NewResultEnvelope(
			"stored",
			[]byte(`"stored"`),
			MimeTypeJSON,
			"",
			AudienceModel,
			map[string]any{ReplaySourceMetadata: ReplaySourceOperation},
		),
	}
	replay := markReplayChunk(original)
	// Act.
	_, forwardErr := prepareFreshResultChunk(replay, nil)
	changed := replay
	changed.Effects = []any{"new-effect"}
	_, changedErr := prepareFreshResultChunk(changed, nil)
	// Assert.
	if forwardErr != nil || changedErr == nil {
		t.Fatalf("forward=%v changed=%v", forwardErr, changedErr)
	}
}
