// Package toolsy provides a universal, type-safe engine for registering, describing,
// and safely executing tools (functions) for LLM agents.
//
// # Overview
//
// LLMs produce tool calls as JSON. This package turns that JSON into concrete Go
// function calls: unmarshal → validate (against the same JSON Schema shown to the
// LLM) → execute → stream result via yield callback or return a clear error for self-correction.
//
// Pipeline: Go function + argument struct → NewTool (reflection + schema) → Tool →
// Registry → Execute (unmarshal, validate, call, stream result via yield).
//
// # Key concepts
//
//   - Streaming: Tool.Execute and Registry.Execute use yield func(Chunk) error. Chunk has CallID, ToolName, Event (EventProgress/EventResult), Data, IsError, Metadata. Use NewStreamTool for multi-chunk responses.
//   - Single Source of Truth: one set of struct tags (json, jsonschema, description, enum) drives schema and validation.
//   - Partial Success: ExecuteBatchStream runs calls in parallel; tool errors are sent as Chunk with IsError: true; the method returns error only for critical failures (context cancel, shutdown).
//   - Self-Correction: ClientError carries human-readable messages back to the LLM. Yield errors become ErrStreamAborted. The after-execution hook (WithOnAfterExecute) receives ExecutionSummary.
//
// Use Extractor for schema generation and validation without a full Tool pipeline (e.g. in custom orchestrators).
// Use NewDynamicTool when the schema is only available at runtime (e.g. from OpenAPI); handler receives yield for streaming.
// Schema generation and validation use github.com/google/jsonschema-go.
// Schema and Parameters: Extractor.Schema() and Tool.Parameters() return a shallow copy (top-level map only);
// nested maps are shared—do not mutate the returned value or nested maps; treat as read-only or clone deeply if modifying.
// Call RegisterType at init time to map custom types (e.g. uuid.UUID) to JSON Schema type/format before first NewTool or NewExtractor.
// See Tool, ToolCall, Chunk, ExecutionSummary for the core types, and NewTool / NewRegistry for setup.
//
// # Example
//
//	type Args struct { City string `json:"city" jsonschema:"City name"` }
//	type Out  struct { Temp float64 `json:"temp"` }
//	tool, _ := toolsy.NewTool("weather", "Get weather", func(_ context.Context, a Args) (Out, error) {
//	    return Out{Temp: 22.5}, nil
//	})
//	reg := toolsy.NewRegistry()
//	reg.Register(tool)
//	var result []byte
//	_ = reg.Execute(ctx, toolsy.ToolCall{ID: "1", ToolName: "weather", Args: []byte(`{"city":"Moscow"}`)}, func(c toolsy.Chunk) error { result = c.Data; return nil })
package toolsy
