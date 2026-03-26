package toolsy

import (
	"context"
)

// EventType enumerates chunk event kinds for Chunk: EventProgress for intermediate UI status,
// EventResult for final data or a stream chunk; EventSuspend signals orchestrator-managed pause.
type EventType string

const (
	EventProgress EventType = "progress"
	EventResult   EventType = "result"
	EventSuspend  EventType = "suspend"
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
	// Manifest returns tool definition for orchestrators and LLM adapters.
	Manifest() ToolManifest
	// Execute runs the tool and streams chunks via yield. The tool may call yield
	// once (simple response) or multiple times (streaming). If yield returns an error,
	// execution must stop and that error is returned (wrapped as ErrStreamAborted).
	Execute(ctx context.Context, run RunContext, input ToolInput, yield func(Chunk) error) error
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

// StateStore stores tool execution state outside the tool process.
type StateStore interface {
	Save(ctx context.Context, key string, data []byte) error
	Load(ctx context.Context, key string) ([]byte, error)
}

// ServiceProvider resolves runtime service dependencies by key.
type ServiceProvider interface {
	Get(key string) (any, bool)
}

// Attachment is binary input passed together with JSON args.
type Attachment struct {
	MimeType string
	Data     []byte
}

func cloneAttachments(in []Attachment) []Attachment {
	if len(in) == 0 {
		return nil
	}
	out := make([]Attachment, len(in))
	for i := range in {
		out[i].MimeType = in[i].MimeType
		if len(in[i].Data) > 0 {
			out[i].Data = make([]byte, len(in[i].Data))
			copy(out[i].Data, in[i].Data)
		}
	}
	return out
}

// ToolInput is the runtime input envelope for tool execution.
type ToolInput struct {
	ArgsJSON    []byte
	Attachments []Attachment
}

// RunContext carries runtime-only dependencies that should not be hidden in context values.
type RunContext struct {
	Credentials CredentialsProvider
	State       StateStore
	Services    ServiceProvider

	attachments []Attachment
	async       *asyncRuntime
}

// Attachments returns runtime attachments for the current call.
func (r RunContext) Attachments() []Attachment {
	return cloneAttachments(r.attachments)
}

// ToolCall is a single execution request (as produced by the LLM).
type ToolCall struct {
	ID       string
	ToolName string
	Input    ToolInput
	Run      RunContext
}

// Chunk is a single stream event from a tool execution.
type Chunk struct {
	CallID   string
	ToolName string
	Event    EventType
	Data     []byte
	MimeType string
	IsError  bool
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
