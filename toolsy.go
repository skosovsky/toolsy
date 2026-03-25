package toolsy

import (
	"context"
	"encoding/json"
	"time"
)

// Event type constants for Chunk. EventProgress is for intermediate UI status;
// EventResult is for final data or a stream chunk; EventSuspend signals orchestrator-managed pause.
const (
	EventProgress = "progress"
	EventResult   = "result"
	EventSuspend  = "suspend"
)

// MIME type constants for Chunk payloads.
const (
	MimeTypeText        = "text/plain; charset=utf-8"
	MimeTypeJSON        = "application/json"
	MimeTypeOctetStream = "application/octet-stream"
	MimeTypePNG         = "image/png"
	MimeTypeJPEG        = "image/jpeg"
)

// Tool is the contract for an LLM-callable instrument.
// It is provider-agnostic (no knowledge of OpenAI, Anthropic, etc.).
type Tool interface {
	Name() string
	Description() string
	// Parameters returns a valid JSON Schema as map (compatible with LLM tool definitions).
	Parameters() map[string]any
	// Execute runs the tool and streams chunks via yield. The tool may call yield
	// once (simple response) or multiple times (streaming). If yield returns an error,
	// execution must stop and that error is returned (wrapped as ErrStreamAborted).
	Execute(ctx context.Context, run RunContext, argsJSON []byte, yield func(Chunk) error) error
}

// ToolMetadata is implemented by tools created with NewTool and exposes optional runtime and
// orchestration metadata. Registry uses Timeout() to override the default execution timeout when
// set. The remaining methods expose discovery labels and execution policy hints such as tags,
// version, dangerous/read-only flags, confirmation requirements, and sensitivity.
type ToolMetadata interface {
	Timeout() time.Duration
	Tags() []string
	Version() string
	IsDangerous() bool
	IsReadOnly() bool
	RequiresConfirmation() bool
	Sensitivity() string
}

// Validator checks JSON arguments before tool execution (e.g. guardrails).
// If Validate returns an error, execution is aborted and the error is returned to the caller (fail-closed).
// Applications can wrap external policy engines (e.g. guardy) without coupling toolsy to them.
type Validator interface {
	Validate(ctx context.Context, toolName string, argsJSON string) error
}

// CredentialsProvider resolves runtime Authorization header values for outbound requests.
// GetAuth returns the complete header value (for example "Bearer ..." or "Basic ...").
type CredentialsProvider interface {
	GetAuth(ctx context.Context, toolName string) (string, error)
}

// RunContext carries runtime-only dependencies that should not be hidden in context values.
type RunContext struct {
	Credentials CredentialsProvider
	async       *asyncRuntime
}

// ToolCall is a single execution request (as produced by the LLM).
type ToolCall struct {
	ID       string
	ToolName string
	Args     json.RawMessage // JSON payload of arguments
	Run      RunContext
}

// Chunk is a single stream event from a tool execution. Registry (and ExecuteBatchStream) set
// CallID and ToolName when forwarding; tools may set Data, RawData, and optionally Event, IsError, Metadata.
//
// Data plus MimeType are the primary payload contract. For typed results (NewTool, NewStreamTool),
// the core serializes RawData to Data as JSON and sets MimeType to application/json. RawData remains
// as a compatibility field for callers that still prefer zero-copy typed access during this major version.
type Chunk struct {
	CallID   string
	ToolName string
	Event    string         // EventProgress, EventResult, or EventSuspend
	Data     []byte         // primary payload bytes
	MimeType string         // MIME type for Data, e.g. application/json or text/plain; charset=utf-8
	RawData  any            // deprecated compatibility field; primary payload is Data + MimeType
	IsError  bool           // true if Data contains error message text
	Metadata map[string]any // optional: progress 0-100, etc.
}

// ExecutionSummary is passed to the after-execution hook (WithOnAfterExecute) when a tool
// execution finishes (success or error). ChunksDelivered and TotalBytes count only chunks
// with !IsError (successfully delivered result chunks).
type ExecutionSummary struct {
	CallID          string
	ToolName        string
	Error           error
	ChunksDelivered int
	TotalBytes      int64
}
