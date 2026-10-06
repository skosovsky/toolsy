package toolsy

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:gocognit,nestif // Fixture modes deliberately exercise raw, typed and host-default paths together.
func TestCanonicalInputNormativeFixtures(t *testing.T) {
	// Arrange: raw tools preserve JSON number tokens/null; typed and host binders
	// declare their own concrete representation/default semantics.
	var fixtures []struct {
		ID        string          `json:"id"`
		Mode      string          `json:"mode"`
		Input     json.RawMessage `json:"input"`
		Canonical json.RawMessage `json:"canonical"`
	}
	raw, err := os.ReadFile("testdata/execution/canonical-inputs.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &fixtures))
	for _, fixture := range fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			type args struct {
				N int64 `json:"n"`
			}
			var tool Tool
			calls, snapshots, binds := 0, 0, 0
			if fixture.Mode == "raw" {
				tool, err = NewProxyTool("canonical", "Canonical", []byte(`{"type":"object"}`),
					func(_ context.Context, _ *RunEnv, input []byte, yield func(Chunk) error) error {
						calls++
						assert.Equal(t, string(fixture.Canonical), string(input))
						return yield(Chunk{Event: EventResult, Data: []byte(`"ok"`), MimeType: MimeTypeJSON})
					})
			} else {
				spec := TypedToolSpec[NoSubject, NoScope, args, string, string]{
					Name:        "canonical",
					Description: "Canonical",
					Handler: func(_ context.Context, _ TypedCallContext[NoSubject, NoScope], _ *RunEnv, bound ValidatedArgs[args]) (ToolResult[string, string], error) {
						calls++
						encoded, encodeErr := json.Marshal(bound.Value)
						require.NoError(t, encodeErr)
						assert.Equal(t, string(fixture.Canonical), string(encoded))
						return NewToolResult[string, string]("ok"), nil
					},
				}
				if fixture.Mode == "default" {
					spec.ArgsBinder = func(_ context.Context, req ArgsBindRequest) (ValidatedArgs[args], error) {
						binds++
						var nullable struct {
							N *int64 `json:"n"`
						}
						if decodeErr := json.Unmarshal(req.Input.ArgsJSON, &nullable); decodeErr != nil {
							return ValidatedArgs[args]{}, decodeErr
						}
						value := args{N: 10}
						if nullable.N != nil {
							value.N = *nullable.N
						}
						canonical, encodeErr := json.Marshal(value)
						return ValidatedArgs[args]{Value: value, Raw: canonical}, encodeErr
					}
				}
				tool, err = NewTypedTool(spec)
			}
			require.NoError(t, err)
			profile := executionProfileFunc(
				func(_ context.Context, call PreparedCall, invoke InvocationHandler, yield func(Chunk) error) error {
					snapshots++
					// Exact bytes, not JSONEq's float64 comparison: >2^53 must stay exact.
					assert.Equal(t, string(fixture.Canonical), string(call.Input.ArgsJSON))
					return invoke(yield)
				},
			)
			// Act: actual built-in preparation path, not a standalone JSON utility.
			err = tool.Execute(
				context.Background(),
				NewRunEnv(nil, WithRunExecutionProfile(profile)),
				ToolInput{ArgsJSON: fixture.Input},
				func(Chunk) error { return nil },
			)
			// Assert.
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
			assert.Equal(t, 1, snapshots)
			if fixture.Mode == "default" {
				assert.Equal(t, 1, binds)
			}
		})
	}
}
