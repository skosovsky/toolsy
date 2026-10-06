package toolsy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyConstructorRejectsReplayMetadata(t *testing.T) {
	for _, value := range []any{false, true, nil, "false"} {
		t.Run("reserved", func(t *testing.T) {
			// Arrange.
			base, err := NewTool("private", "Private", func(context.Context, *RunEnv, struct{}) (string, error) {
				return "private", nil
			})
			require.NoError(t, err)
			// Act.
			tool, err := NewPolicyTool(ToolPolicySpec[NoSubject, NoScope, struct{}]{
				Tool: base,
				ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[struct{}], error) {
					return ValidatedArgs[struct{}]{Raw: []byte(`{}`)}, nil
				},
				EnvelopeMetadata: map[string]any{CacheReplayMetadata: value},
			})
			// Assert: even a false/nil declaration is rejected before dispatch.
			require.ErrorContains(t, err, "reserved replay metadata")
			assert.Nil(t, tool)
		})
	}
}

func TestPolicyReplayOverlayPreservesProvenance(t *testing.T) {
	// Arrange: defense in depth for repeated transforms, independent of bootstrap validation.
	envelope := ToolEnvelope{Audience: AudienceInternal, Metadata: map[string]any{CacheReplayMetadata: true}}
	chunk := Chunk{Event: EventResult, Envelope: &envelope}
	overlay := map[string]any{CacheReplayMetadata: false, "host": "label"}
	// Act: emulate inner/outer and capture/delivery transform composition.
	for range 4 {
		chunk = applyPolicyToolEnvelope(chunk, "", AudienceModel, overlay)
	}
	// Assert: no widening, marker loss or mutation of source metadata.
	assert.Equal(t, AudienceInternal, chunk.ToolEnvelope().Audience)
	assert.Equal(t, true, chunk.ToolEnvelope().Metadata[CacheReplayMetadata])
	assert.Equal(t, "label", chunk.ToolEnvelope().Metadata["host"])
	assert.Equal(t, true, envelope.Metadata[CacheReplayMetadata])
	assert.Equal(t, false, overlay[CacheReplayMetadata])
}

func TestNestedPolicyReplayEffectsApplyOnce(t *testing.T) {
	for _, kind := range []string{"cache", "operation"} {
		t.Run(kind, func(t *testing.T) {
			// Arrange: one shared handler; multiple metadata transforms around persistence/replay.
			calls, reductions := 0, 0
			base, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, struct{}, string, string]{
				Name:        "private",
				Description: "Private",
				Options:     []ToolOption{WithIdempotent()},
				Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
					calls++
					return ToolResult[string, string]{Value: "private", Effects: []string{"apply"}}, nil
				},
			})
			require.NoError(t, err)
			wrap := func(audience ToolAudience) Tool {
				t.Helper()
				tool := base
				for range 2 {
					tool, err = NewPolicyTool(ToolPolicySpec[NoSubject, NoScope, struct{}]{
						Tool: tool, Audience: audience, EnvelopeMetadata: map[string]any{"host": "label"},
						ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[struct{}], error) {
							return ValidatedArgs[struct{}]{Raw: []byte(`{}`)}, nil
						},
					})
					require.NoError(t, err)
				}
				return tool
			}
			profile := compositionProfile(t, kind)
			var outcomes []ToolEnvelope
			reducer := func(chunk Chunk) error {
				outcome := chunk.ToolEnvelope()
				outcomes = append(outcomes, outcome)
				if replay, _ := outcome.Metadata[CacheReplayMetadata].(bool); !replay {
					reductions += len(chunk.Effects)
				}
				return nil
			}
			// Act: persist privately, then broaden the current wrapper and replay twice.
			for i, audience := range []ToolAudience{AudienceInternal, AudienceModel, AudienceModel} {
				call := ToolCall{
					ToolName: "private",
					Input:    ToolInput{CallID: string(rune('a' + i)), ArgsJSON: []byte(`{}`)},
				}
				require.NoError(
					t,
					compositionExecutor(t, wrap(audience), profile, "session")(context.Background(), call, reducer),
				)
			}
			// Assert: replay remains private and retains provenance through every wrapper.
			assert.Equal(t, 1, calls)
			assert.Equal(t, 1, reductions)
			require.Len(t, outcomes, 3)
			for _, outcome := range outcomes[1:] {
				assert.Equal(t, AudienceInternal, outcome.Audience)
				assert.Equal(t, true, outcome.Metadata[CacheReplayMetadata])
			}
		})
	}
}
