package toolsy

import (
	"context"
	"encoding/json"
	"time"
)

// Tool is the contract for an LLM-callable instrument.
// It is provider-agnostic (no knowledge of OpenAI, Anthropic, etc.).
type Tool interface {
	Name() string
	Description() string
	// Parameters returns a valid JSON Schema as map (compatible with LLM tool definitions).
	Parameters() map[string]any
	// Execute accepts raw JSON from the LLM, runs the logic, and returns raw JSON result or error.
	Execute(ctx context.Context, argsJSON []byte) ([]byte, error)
}

// ToolMetadata is implemented by tools created with NewTool and provides optional per-tool settings.
// Registry uses Timeout() to override default execution timeout when set. Other methods expose
// tags, version, and dangerous flag for orchestration or discovery.
type ToolMetadata interface {
	Timeout() time.Duration
	Tags() []string
	Version() string
	IsDangerous() bool
}

// ToolCall is a single execution request (as produced by the LLM).
type ToolCall struct {
	ID       string
	ToolName string
	Args     json.RawMessage // JSON payload of arguments
}

// ToolResult is the outcome of one tool execution (to be sent back to the LLM).
type ToolResult struct {
	CallID   string
	ToolName string
	Result   json.RawMessage
	Error    error
}
