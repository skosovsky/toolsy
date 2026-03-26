package human

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestRequestApproval_YieldsSuspendChunkThenReturnsErrSuspend(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	approvalTool := tools[0]

	var gotChunk toolsy.Chunk
	err = approvalTool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"action":"delete","reason":"user asked"}`)},
		func(c toolsy.Chunk) error {
			gotChunk = c
			return nil
		},
	)
	require.ErrorIs(t, err, toolsy.ErrSuspend)
	require.Equal(t, toolsy.EventSuspend, gotChunk.Event)
	require.Equal(t, toolsy.MimeTypeJSON, gotChunk.MimeType)
	require.JSONEq(
		t,
		`{"kind":"approval","action":"delete","reason":"user asked"}`,
		string(gotChunk.Data),
	)
}

func TestAskClarification_YieldsSuspendChunkThenReturnsErrSuspend(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	clarificationTool := tools[1]

	var gotChunk toolsy.Chunk
	err = clarificationTool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"question":"Which button?"}`)},
		func(c toolsy.Chunk) error {
			gotChunk = c
			return nil
		},
	)
	require.ErrorIs(t, err, toolsy.ErrSuspend)
	require.Equal(t, toolsy.EventSuspend, gotChunk.Event)
	require.Equal(t, toolsy.MimeTypeJSON, gotChunk.MimeType)
	require.JSONEq(
		t,
		`{"kind":"clarification","question":"Which button?"}`,
		string(gotChunk.Data),
	)
}

func TestRequestApproval_YieldErrorShortCircuitsBeforeErrSuspend(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	approvalTool := tools[0]

	yieldErr := errors.New("stream closed")
	err = approvalTool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"action":"delete","reason":"user asked"}`)},
		func(toolsy.Chunk) error { return yieldErr },
	)
	require.ErrorIs(t, err, toolsy.ErrStreamAborted)
	require.NotErrorIs(t, err, toolsy.ErrSuspend)
}

func TestRequestApproval_PayloadShape(t *testing.T) {
	tools, err := AsTools()
	require.NoError(t, err)
	approvalTool := tools[0]

	var payload map[string]string
	err = approvalTool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"action":"send_email","reason":"user requested"}`)},
		func(c toolsy.Chunk) error {
			return json.Unmarshal(c.Data, &payload)
		},
	)
	require.ErrorIs(t, err, toolsy.ErrSuspend)
	require.Equal(
		t,
		map[string]string{
			"kind":   "approval",
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

	var payload map[string]string
	err = clarificationTool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"question":"What is the deadline?"}`)},
		func(c toolsy.Chunk) error {
			return json.Unmarshal(c.Data, &payload)
		},
	)
	require.ErrorIs(t, err, toolsy.ErrSuspend)
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
	tools, err := AsTools(WithApprovalName("approve"), WithClarificationName("clarify"))
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "approve", tools[0].Manifest().Name)
	require.Equal(t, "clarify", tools[1].Manifest().Name)
}
