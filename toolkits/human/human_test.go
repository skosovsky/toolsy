package human

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestRequestHumanReview_YieldsControlPauseThenReturnsErrPause(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	reviewTool := tools[0]

	var gotChunk toolsy.Chunk
	err = reviewTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"action":"delete","reason":"user asked"}`)},
		func(c toolsy.Chunk) error {
			gotChunk = c
			return nil
		},
	)
	require.ErrorIs(t, err, toolsy.ErrPause)
	require.Equal(t, toolsy.EventControl, gotChunk.Event)
	pause, ok := gotChunk.Control.(*toolsy.PauseSignal)
	require.True(t, ok)
	require.JSONEq(
		t,
		`{"kind":"human_review","action":"delete","reason":"user asked"}`,
		pause.Reason,
	)
}

func TestAskClarification_YieldsControlPauseThenReturnsErrPause(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	clarificationTool := tools[1]

	var gotChunk toolsy.Chunk
	err = clarificationTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"question":"Which button?"}`)},
		func(c toolsy.Chunk) error {
			gotChunk = c
			return nil
		},
	)
	require.ErrorIs(t, err, toolsy.ErrPause)
	require.Equal(t, toolsy.EventControl, gotChunk.Event)
	pause, ok := gotChunk.Control.(*toolsy.PauseSignal)
	require.True(t, ok)
	require.JSONEq(
		t,
		`{"kind":"clarification","question":"Which button?"}`,
		pause.Reason,
	)
}

func TestRequestHumanReview_YieldErrorShortCircuitsBeforeErrPause(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	reviewTool := tools[0]

	yieldErr := errors.New("stream closed")
	err = reviewTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"action":"delete","reason":"user asked"}`)},
		func(toolsy.Chunk) error { return yieldErr },
	)
	require.ErrorIs(t, err, toolsy.ErrStreamAborted)
	require.NotErrorIs(t, err, toolsy.ErrPause)
}

func TestRequestHumanReview_PayloadShape(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	reviewTool := tools[0]

	var pauseReason string
	err = reviewTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"action":"send_email","reason":"user requested"}`)},
		func(c toolsy.Chunk) error {
			if pause, ok := c.Control.(*toolsy.PauseSignal); ok {
				pauseReason = pause.Reason
			}
			return nil
		},
	)
	require.ErrorIs(t, err, toolsy.ErrPause)
	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(pauseReason), &payload))
	require.Equal(
		t,
		map[string]string{
			"kind":   "human_review",
			"action": "send_email",
			"reason": "user requested",
		},
		payload,
	)
}

func TestAskClarification_PayloadShape(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	clarificationTool := tools[1]

	var pauseReason string
	err = clarificationTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"question":"What is the deadline?"}`)},
		func(c toolsy.Chunk) error {
			if pause, ok := c.Control.(*toolsy.PauseSignal); ok {
				pauseReason = pause.Reason
			}
			return nil
		},
	)
	require.ErrorIs(t, err, toolsy.ErrPause)
	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(pauseReason), &payload))
	require.Equal(
		t,
		map[string]string{
			"kind":     "clarification",
			"question": "What is the deadline?",
		},
		payload,
	)
}

func TestAsTools_ToolCount(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	require.Len(t, tools, 2)
}

func TestAsTools_CustomNames(t *testing.T) {
	tools, err := AsTools(WithReviewName("approve"), WithClarificationName("clarify"))
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "approve", tools[0].Manifest().Name)
	require.Equal(t, "clarify", tools[1].Manifest().Name)
}

func TestAsTools_CompletionPolicySilentYield(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	for _, tool := range tools {
		require.Equal(t, toolsy.CompletionSilentYield, tool.Manifest().CompletionPolicy)
	}
}
