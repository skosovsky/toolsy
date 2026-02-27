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
//   - Single Source of Truth: one set of struct tags (e.g. jsonschema) drives both
//     the schema sent to the LLM and the validation of incoming JSON.
//   - Partial Success: ExecuteBatch collects all results; one failure does not cancel others.
//   - Self-Correction: ClientError carries human-readable messages back to the LLM.
//
// See Tool, ToolCall, ToolResult for the core types, and NewTool / NewRegistry for setup.
//
// # Example
//
//	type Args struct { City string `json:"city" jsonschema:"required"` }
//	type Out  struct { Temp float64 `json:"temp"` }
//	tool, err := toolsy.NewTool("weather", "Get weather", func(_ context.Context, a Args) (Out, error) {
//	    return Out{Temp: 22.5}, nil
//	})
//	if err != nil { ... }
//	reg := toolsy.NewRegistry()
//	reg.Register(tool)
//	result := reg.Execute(ctx, toolsy.ToolCall{ID: "1", ToolName: "weather", Args: []byte(`{"city":"Moscow"}`)})
package toolsy
