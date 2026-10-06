package toolsy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationTerminalStreamFailureAfterEffectRemainsUnknown(t *testing.T) {
	for _, mode := range []string{"invalid", "duplicate", "consumer-abort", "late-producer-failure"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: actual terminal builder and operation profile share one boundary.
			effects, successes := 0, 0
			tool, err := NewStreamTool(
				"terminal-write",
				"Terminal write",
				func(_ context.Context, _ *RunEnv, _ struct{}, yield func(Chunk) error) error {
					effects++ // external action may already have committed.
					valid := Chunk{Event: EventResult, Data: []byte(`{"n":1}`), MimeType: MimeTypeJSON}
					switch mode {
					case "invalid":
						_ = yield(Chunk{Event: EventResult, Data: []byte(`{"n":"invalid"}`), MimeType: MimeTypeJSON})
					case "duplicate":
						_ = yield(valid)
						_ = yield(valid)
					case "consumer-abort":
						_ = yield(Chunk{Event: EventProgress, Data: []byte("working"), MimeType: MimeTypeText})
						_ = yield(valid)
					case "late-producer-failure":
						_ = yield(valid)
						return errors.New("failure after effect and candidate")
					}
					return nil // deliberately swallow callback errors in adversarial cases.
				},
				WithTerminalStream(0),
				WithOutputSchema(
					map[string]any{
						"type":       "object",
						"required":   []string{"n"},
						"properties": map[string]any{"n": map[string]any{"type": "integer"}},
					},
				),
			)
			require.NoError(t, err)
			execute := compositionExecutor(t, tool, compositionProfile(t, "operation"), "session")
			call := ToolCall{ToolName: "terminal-write", Input: ToolInput{CallID: "first", ArgsJSON: []byte(`{}`)}}
			// Act: invalid/aborted terminal after the effect, then repeated delivery.
			err = execute(context.Background(), call, func(c Chunk) error {
				if c.Event == EventResult {
					successes++
				}
				if mode == "consumer-abort" {
					return errors.New("consumer stopped")
				}
				return nil
			})
			var uncertain *OperationOutcomeError
			require.ErrorAs(t, err, &uncertain)
			var terminal *StreamContractError
			require.ErrorAs(t, err, &terminal)
			call.Input.CallID = "repeat"
			err = execute(context.Background(), call, func(c Chunk) error {
				if c.Event == EventResult {
					successes++
				}
				return nil
			})
			// Assert: neither invalid candidate nor lease/re-delivery authorizes a new effect.
			var state *OperationStateError
			require.ErrorAs(t, err, &state)
			assert.Equal(t, OperationUnknown, state.Record.State)
			assert.Equal(t, 1, effects)
			assert.Zero(t, successes)
		})
	}
}
