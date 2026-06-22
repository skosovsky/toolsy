package toolsy

import (
	"context"
)

// EventType enumerates chunk event kinds for Chunk: EventProgress for intermediate UI status,
// EventResult for final data or a stream chunk; EventControl for orchestrator-managed signals.
type EventType string

const (
	EventProgress EventType = "progress"
	EventResult   EventType = "result"
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
	Execute(ctx context.Context, env *RunEnv, input ToolInput, yield func(Chunk) error) error
}

// Validator checks JSON arguments before tool execution (e.g. guardrails).
// If Validate returns an error, execution is aborted and the error is returned to the caller (fail-closed).
// Applications can wrap external policy engines (e.g. guardy) without coupling toolsy to them.
//
// Validator is reject-only and cannot return canonical args. Use [ArgsBinder]
// through [NewTypedTool] or [NewPolicyTool] for production agent execution paths.
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
	// CallID is the orchestrator/LLM identifier for this tool call.
	// It may be empty for non-LLM callers.
	CallID string

	ArgsJSON    []byte
	Attachments []Attachment
}

// Clone returns a deep copy of in (ArgsJSON backing array and attachment bytes).
func (in ToolInput) Clone() ToolInput {
	out := ToolInput{ //nolint:exhaustruct // ArgsJSON and Attachments assigned below
		CallID: in.CallID,
	}
	if len(in.ArgsJSON) > 0 {
		out.ArgsJSON = make([]byte, len(in.ArgsJSON))
		copy(out.ArgsJSON, in.ArgsJSON)
	}
	out.Attachments = cloneAttachments(in.Attachments)
	return out
}

// ToolCall is a single execution request (as produced by the LLM).
type ToolCall struct {
	ToolName    string
	Input       ToolInput
	Env         *RunEnv
	CallContext CallContext
}

func cloneToolCall(call ToolCall) ToolCall {
	return ToolCall{
		ToolName:    call.ToolName,
		Input:       call.Input.Clone(),
		Env:         call.Env,
		CallContext: cloneCallContext(call.CallContext),
	}
}

// ProgressInfo carries optional data-plane progress for EventProgress chunks.
type ProgressInfo struct {
	Percent *int
	Total   *int
	Message string
	Label   string
	Status  string
	Token   string
}

// Chunk is a single stream event from a tool execution.
// Data-plane payloads use Data/MimeType. Control-plane signals use EventControl + Control.
type Chunk struct {
	CallID   string
	ToolName string
	Event    EventType
	Data     []byte
	MimeType string
	IsError  bool
	// TypedResult carries the host-typed result for in-process outcome aggregation.
	TypedResult any
	// EmptyResult marks an intentional successful no-op/empty result.
	EmptyResult bool
	// Noop marks an intentional successful no-op with no state/effect changes.
	Noop bool
	// Effects carries host-owned declarative effects for reducers outside toolsy.
	Effects []any
	// Controls carries declarative control-plane signals attached to a result.
	Controls []ControlSignal
	// Control carries typed orchestrator signals when Event == EventControl.
	Control ControlSignal
	// Progress carries optional progress metadata for EventProgress.
	Progress *ProgressInfo
	// Envelope classifies structured result/error payloads for downstream delivery.
	Envelope *ToolEnvelope
}

// ToolEnvelope returns a typed delivery envelope for this chunk.
func (c Chunk) ToolEnvelope() ToolEnvelope {
	if c.Envelope != nil {
		return *cloneToolEnvelope(c.Envelope)
	}
	if c.IsError {
		return *NewErrorEnvelope(executionErrorFromChunk(c), c.Data, c.MimeType, "", "", nil)
	}
	return *NewResultEnvelope(c.TypedResult, c.Data, c.MimeType, "", "", nil)
}

// ExecutionSummary is passed to the after-execution hook (WithOnAfterExecute) when a tool
// execution finishes (success or error). ChunksDelivered and TotalBytes count only chunks
// with !IsError (successfully delivered result chunks). ErrorChunks and LastErrorText
// describe delivered soft errors (chunks with IsError=true).
type ExecutionSummary struct {
	CallID          string
	ToolName        string
	Error           error
	ChunksDelivered int
	TotalBytes      int64
	ErrorChunks     int
	LastErrorText   string
}
