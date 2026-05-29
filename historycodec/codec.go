package historycodec

import (
	"encoding/json"
	"fmt"

	"github.com/skosovsky/toolsy"
)

const wireVersion = 1

type wireToolCall struct {
	Version  int    `json:"v"`
	ToolName string `json:"tool_name"`
	CallID   string `json:"call_id,omitempty"`
	ArgsJSON []byte `json:"args_json"`
}

type wireToolResult struct {
	Version  int    `json:"v"`
	CallID   string `json:"call_id,omitempty"`
	ToolName string `json:"tool_name,omitempty"`
	Data     []byte `json:"data"`
	MimeType string `json:"mime_type"`
	IsError  bool   `json:"is_error,omitempty"`
}

// MarshalToolCall encodes a ToolCall using the canonical toolsy wire format.
func MarshalToolCall(call toolsy.ToolCall) ([]byte, error) {
	return json.Marshal(wireToolCall{
		Version:  wireVersion,
		ToolName: call.ToolName,
		CallID:   call.Input.CallID,
		ArgsJSON: call.Input.ArgsJSON,
	})
}

// UnmarshalToolCall decodes a ToolCall from the canonical wire format.
func UnmarshalToolCall(data []byte) (toolsy.ToolCall, error) {
	var w wireToolCall
	if err := json.Unmarshal(data, &w); err != nil {
		return toolsy.ToolCall{}, fmt.Errorf("historycodec: unmarshal tool call: %w", err)
	}
	if w.Version != wireVersion {
		return toolsy.ToolCall{}, fmt.Errorf("historycodec: unsupported wire version %d", w.Version)
	}
	return toolsy.ToolCall{ //nolint:exhaustruct // wire format omits runtime-only fields
		ToolName: w.ToolName,
		Input: toolsy.ToolInput{ //nolint:exhaustruct // attachments not serialized on wire
			CallID:   w.CallID,
			ArgsJSON: w.ArgsJSON,
		},
	}, nil
}

// MarshalToolResult encodes a delivered chunk as canonical tool result wire format.
func MarshalToolResult(chunk toolsy.Chunk) ([]byte, error) {
	return json.Marshal(wireToolResult{
		Version:  wireVersion,
		CallID:   chunk.CallID,
		ToolName: chunk.ToolName,
		Data:     chunk.Data,
		MimeType: chunk.MimeType,
		IsError:  chunk.IsError,
	})
}

// UnmarshalToolResult decodes canonical tool result wire format into a Chunk.
func UnmarshalToolResult(data []byte) (toolsy.Chunk, error) {
	var w wireToolResult
	if err := json.Unmarshal(data, &w); err != nil {
		return toolsy.Chunk{}, fmt.Errorf("historycodec: unmarshal tool result: %w", err)
	}
	if w.Version != wireVersion {
		return toolsy.Chunk{}, fmt.Errorf("historycodec: unsupported wire version %d", w.Version)
	}
	return toolsy.Chunk{
		CallID:   w.CallID,
		ToolName: w.ToolName,
		Event:    toolsy.EventResult,
		Data:     w.Data,
		MimeType: w.MimeType,
		IsError:  w.IsError,
	}, nil
}
