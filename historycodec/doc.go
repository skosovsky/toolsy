// Package historycodec provides the canonical wire format for persisting tool calls and results.
//
// Wire format version 1 serializes ToolCall and delivered result chunks as JSON.
// []byte fields (args_json, data) are encoded as standard JSON base64 strings per encoding/json.
// Control-plane chunks (EventControl) and progress metadata are not part of v1 wire format.
package historycodec
