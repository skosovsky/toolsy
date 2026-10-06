package toolsy

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostProviderCompletionBoundaryFixture(t *testing.T) {
	// Arrange: a host/provider harness owns the completion marker. Core parser
	// validates the complete arguments but cannot infer completion from a prefix.
	var fixtures []struct {
		Name   string `json:"name"`
		Events []struct {
			Args string `json:"args"`
			End  bool   `json:"end"`
		} `json:"events"`
		WantDispatch int  `json:"want_dispatch"`
		WantError    bool `json:"want_error"`
	}
	raw, err := os.ReadFile("testdata/execution/provider-completion.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &fixtures))
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			calls := 0
			tool, err := NewTool(
				"write",
				"Write",
				func(context.Context, *RunEnv, struct{}) (string, error) { calls++; return "ok", nil },
			)
			require.NoError(t, err)
			reg, err := NewRegistry(tool)
			require.NoError(t, err)
			var parts []ContentPart
			var completionErr error
			// Act: append fragments without dispatch; only explicit end starts parsing/execution.
			for _, event := range fixture.Events {
				if !event.End {
					parts = append(
						parts,
						ContentPart{
							Type:       ContentTypeToolCall,
							ToolName:   "write",
							ToolCallID: "intent",
							ArgsChunk:  event.Args,
						},
					)
					assert.Zero(t, calls, "syntactically valid prefix does not authorize dispatch")
					continue
				}
				args, parseErr := (StandardCallParser{}).ExtractExactlyOne(parts, "write")
				if parseErr != nil {
					completionErr = parseErr
					break
				}
				completionErr = reg.Execute(
					context.Background(),
					ToolCall{ToolName: "write", Input: ToolInput{ArgsJSON: args}},
					func(Chunk) error { return nil },
				)
			}
			// Assert: incomplete/invalid calls never reach the ordinary executor.
			assert.Equal(t, fixture.WantDispatch, calls)
			if fixture.WantError {
				require.Error(t, completionErr)
			} else {
				require.NoError(t, completionErr)
			}
		})
	}
}
