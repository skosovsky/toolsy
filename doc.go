// Package toolsy provides a universal, type-safe engine for registering, describing,
// and safely executing tools (functions) for LLM agents.
//
// # Overview
//
// LLMs produce tool calls as JSON. This package turns that JSON into concrete Go
// function calls: unmarshal → validate (against the same JSON Schema shown to the
// LLM) → execute → marshal result or return a clear error for self-correction.
//
// Pipeline: Go function + argument struct → NewTool (reflection + schema) → Tool →
// Registry → Execute (unmarshal, validate, call, marshal) → ToolResult.
//
// # Key concepts
//
//   - Single Source of Truth: one set of struct tags (json for field names/omitempty,
//     jsonschema for descriptions) drives both the schema sent to the LLM and the validation of incoming JSON.
//   - Partial Success: ExecuteBatch collects all results; one failure does not cancel others.
//   - Self-Correction: ClientError carries human-readable messages back to the LLM.
//
// Use Extractor for schema generation and validation without a full Tool pipeline (e.g. in custom orchestrators).
// Use NewDynamicTool when the schema is only available at runtime (e.g. from OpenAPI); it validates against the schema and passes raw JSON to the handler.
// Schema generation and validation use github.com/google/jsonschema-go.
// Schema and Parameters: Extractor.Schema() and Tool.Parameters() return a shallow copy (top-level map only);
// nested maps are shared—do not mutate the returned value or nested maps; treat as read-only or clone deeply if modifying.
// Call RegisterType at init time to map custom types (e.g. uuid.UUID) to JSON Schema type/format before first NewTool or NewExtractor.
// See Tool, ToolCall, ToolResult for the core types, and NewTool / NewRegistry for setup.
//
// # Example
//
//	type Args struct { City string `json:"city" jsonschema:"City name"` }
//	type Out  struct { Temp float64 `json:"temp"` }
//	tool, err := toolsy.NewTool("weather", "Get weather", func(_ context.Context, a Args) (Out, error) {
//	    return Out{Temp: 22.5}, nil
//	})
//	if err != nil { ... }
//	reg := toolsy.NewRegistry()
//	reg.Register(tool)
//	result := reg.Execute(ctx, toolsy.ToolCall{ID: "1", ToolName: "weather", Args: []byte(`{"city":"Moscow"}`)})
package toolsy
