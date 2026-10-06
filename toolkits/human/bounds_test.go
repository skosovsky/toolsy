package human

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestPausePayloadExactBounds(t *testing.T) {
	for _, question := range []string{"<你好>", strings.Repeat("<", 20), strings.Repeat("界", 20)} {
		// Arrange: the budget includes JSON escaping, syntax and payload kind.
		payload, err := json.Marshal(map[string]string{"kind": "clarification", "question": question})
		require.NoError(t, err)
		args, err := json.Marshal(map[string]string{"question": question})
		require.NoError(t, err)
		for _, limit := range []int{1, len(payload) - 1, len(payload)} {
			tools, buildErr := AsTools(WithMaxPayloadBytes(limit))
			require.NoError(t, buildErr)
			var reasons []string
			// Act.
			err = tools[1].Execute(
				context.Background(),
				nil,
				toolsy.ToolInput{ArgsJSON: args},
				func(chunk toolsy.Chunk) error {
					reasons = append(reasons, chunk.Control.(*toolsy.PauseSignal).Reason)
					return nil
				},
			)
			// Assert: no partial pause or silent question mutation.
			if limit < len(payload) {
				require.ErrorIs(t, err, toolsy.ErrValidation)
				require.Empty(t, reasons)
			} else {
				require.ErrorIs(t, err, toolsy.ErrPause)
				require.Equal(t, []string{string(payload)}, reasons)
				require.True(t, json.Valid([]byte(reasons[0])))
			}
		}
	}
}

func TestPauseRejectsInvalidConfigurationAndLargeAction(t *testing.T) {
	// Arrange and act: explicit invalid limits and options fail construction.
	for _, limit := range []int{0, -1, toolsy.MaxControlBytes + 1} {
		_, err := AsTools(WithMaxPayloadBytes(limit))
		require.Error(t, err)
	}
	_, err := AsTools(nil)
	require.Error(t, err)
	tools, err := AsTools()
	require.NoError(t, err)
	args, err := json.Marshal(map[string]string{"action": strings.Repeat("x", defaultMaxPayloadBytes), "reason": "yes"})
	require.NoError(t, err)
	yields := 0
	// Act.
	err = tools[0].Execute(context.Background(), nil, toolsy.ToolInput{ArgsJSON: args}, func(toolsy.Chunk) error {
		yields++
		return nil
	})
	// Assert: the oversized request does not publish an apparently accepted pause.
	require.ErrorIs(t, err, toolsy.ErrValidation)
	require.Zero(t, yields)
}
