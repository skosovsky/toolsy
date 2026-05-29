package historycodec_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/historycodec"
)

func TestMarshalUnmarshalToolCall_Golden(t *testing.T) {
	call := toolsy.ToolCall{
		ToolName: "weather",
		Input: toolsy.ToolInput{
			CallID:   "call-1",
			ArgsJSON: []byte(`{"city":"Paris"}`),
		},
	}
	data, err := historycodec.MarshalToolCall(call)
	require.NoError(t, err)
	require.Contains(t, string(data), `"tool_name":"weather"`)
	require.Contains(t, string(data), `"call_id":"call-1"`)

	back, err := historycodec.UnmarshalToolCall(data)
	require.NoError(t, err)
	require.Equal(t, call, back)
}

func TestMarshalUnmarshalToolResult_Golden(t *testing.T) {
	chunk := toolsy.Chunk{
		CallID:   "call-1",
		ToolName: "weather",
		Event:    toolsy.EventResult,
		Data:     []byte(`{"temp":22}`),
		MimeType: toolsy.MimeTypeJSON,
	}
	data, err := historycodec.MarshalToolResult(chunk)
	require.NoError(t, err)
	require.Contains(t, string(data), `"call_id":"call-1"`)
	require.Contains(t, string(data), `"mime_type":"application/json"`)

	back, err := historycodec.UnmarshalToolResult(data)
	require.NoError(t, err)
	require.Equal(t, chunk.Event, back.Event)
	require.Equal(t, chunk.CallID, back.CallID)
	require.Equal(t, chunk.ToolName, back.ToolName)
	require.Equal(t, chunk.Data, back.Data)
	require.Equal(t, chunk.MimeType, back.MimeType)
}

func TestUnmarshalToolCall_UnsupportedVersion(t *testing.T) {
	_, err := historycodec.UnmarshalToolCall([]byte(`{"v":99,"tool_name":"x","args_json":{}}`))
	require.Error(t, err)
}

func TestMarshalToolResult_OmitsControlPlane(t *testing.T) {
	chunk := toolsy.Chunk{
		Event:   toolsy.EventControl,
		Control: &toolsy.PauseSignal{Reason: "wait"},
	}
	data, err := historycodec.MarshalToolResult(chunk)
	require.NoError(t, err)
	back, err := historycodec.UnmarshalToolResult(data)
	require.NoError(t, err)
	require.Equal(t, toolsy.EventResult, back.Event)
	require.Nil(t, back.Control)
}
