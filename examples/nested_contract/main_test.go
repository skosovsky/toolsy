package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestNestedSchemaValidatesBeforeDispatch(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		valid     bool
	}{
		{"object", `{"payload":{"tags":["one","two"]}}`, true},
		{"JSON string", `{"payload":"{\"tags\":[\"one\"]}"}`, false},
		{"invalid item", `{"payload":{"tags":[1]}}`, false},
		{"missing payload", `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			calls := 0
			tool, err := buildTool(func(_ context.Context, _ *toolsy.RunEnv, in request) (response, error) {
				calls++
				return response{Count: len(in.Payload.Tags)}, nil
			})
			require.NoError(t, err)
			// Act.
			err = tool.Execute(
				context.Background(),
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(tc.raw)},
				func(toolsy.Chunk) error { return nil },
			)
			// Assert.
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, 1, calls)
			} else {
				require.ErrorIs(t, err, toolsy.ErrValidation)
				require.Zero(t, calls)
			}
		})
	}
}
