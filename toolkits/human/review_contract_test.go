package human

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestReviewIntentManifestAndOptions(t *testing.T) {
	// Arrange/Act.
	tools, err := AsTools()
	// Assert.
	require.NoError(t, err)
	require.Equal(t, "request_human_review", tools[0].Manifest().Name)
	require.Equal(t, "ask_human_clarification", tools[1].Manifest().Name)
	require.Contains(t, tools[0].Manifest().Description, "does not authorize")
	for _, tool := range tools {
		require.NotEqual(t, "request_approval", tool.Manifest().Name)
	}
	// Arrange/Act/Assert: names/descriptions configure data presentation only.
	tools, err = AsTools(WithReviewName("review_intent"), WithReviewDescription("Host review intent"))
	require.NoError(t, err)
	require.Equal(t, "review_intent", tools[0].Manifest().Name)
	require.Equal(t, "Host review intent", tools[0].Manifest().Description)
}

func TestReviewIntentFitsCoreControlBoundary(t *testing.T) {
	// Arrange: account for complete encoded object syntax and the new payload kind.
	empty, err := json.Marshal(map[string]string{"kind": "human_review", "action": "", "reason": ""})
	require.NoError(t, err)
	tools, err := AsTools(WithMaxPayloadBytes(toolsy.MaxControlBytes))
	require.NoError(t, err)
	for _, extra := range []int{0, 1} {
		t.Run(map[int]string{0: "inclusive", 1: "over"}[extra], func(t *testing.T) {
			action := strings.Repeat("a", toolsy.MaxControlBytes-len(empty)+extra)
			args, marshalErr := json.Marshal(map[string]string{"action": action, "reason": ""})
			require.NoError(t, marshalErr)
			var delivered []string
			// Act.
			err = tools[0].Execute(
				context.Background(),
				nil,
				toolsy.ToolInput{ArgsJSON: args},
				func(c toolsy.Chunk) error {
					delivered = append(delivered, c.Control.(*toolsy.PauseSignal).Reason)
					return nil
				},
			)
			// Assert.
			if extra != 0 {
				require.ErrorIs(t, err, toolsy.ErrValidation)
				require.Empty(t, delivered)
				return
			}
			require.ErrorIs(t, err, toolsy.ErrPause)
			require.Len(t, delivered, 1)
			require.Len(t, delivered[0], toolsy.MaxControlBytes)
			var intent map[string]string
			require.NoError(t, json.Unmarshal([]byte(delivered[0]), &intent))
			require.Equal(t, map[string]string{"kind": "human_review", "action": action, "reason": ""}, intent)
		})
	}
	_, err = AsTools(WithMaxPayloadBytes(toolsy.MaxControlBytes + 1))
	require.Error(t, err)
}

func TestReviewIntentCancellationNeverIssuesControl(t *testing.T) {
	// Arrange.
	tools, err := AsTools()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, test := range []struct {
		index int
		args  string
	}{{0, `{"action":"delete","reason":"approved"}`}, {1, `{"question":"yes?"}`}} {
		delivered := 0
		// Act.
		err = tools[test.index].Execute(
			ctx,
			nil,
			toolsy.ToolInput{ArgsJSON: []byte(test.args)},
			func(toolsy.Chunk) error { delivered++; return nil },
		)
		// Assert.
		require.ErrorIs(t, err, context.Canceled)
		require.Zero(t, delivered)
	}
}
