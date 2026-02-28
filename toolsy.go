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
	// Execute runs the tool and streams result data via yield. The tool may call yield
	// once (simple response) or multiple times (streaming). If yield returns an error,
	// execution must stop and that error is returned (wrapped as ErrStreamAborted).
	Execute(ctx context.Context, argsJSON []byte, yield func(data []byte) error) error
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

// Chunk is a single stream event from a tool execution. Used by ExecuteBatchStream so the
// caller can identify which call and tool produced each chunk when multiple tools run in parallel.
type Chunk struct {
	CallID   string
	ToolName string
	Data     []byte
}

// ExecutionSummary is passed to the after-execution hook (WithOnAfterExecute) when a tool
// execution finishes (success or error). ChunksDelivered and TotalBytes reflect what was
// successfully sent via yield before any error.
type ExecutionSummary struct {
	CallID          string
	ToolName        string
	Error           error
	ChunksDelivered int
	TotalBytes      int64
}
