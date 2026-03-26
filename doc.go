// Package toolsy provides a type-safe engine for building and executing LLM-callable tools.
//
// # Overview
//
// Pipeline: Go handler + args struct -> NewTool/NewStreamTool -> Tool -> Registry ->
// Execute. Input JSON is validated against generated schema, the handler is called,
// and results are streamed as Chunk values.
//
// # v2 contracts
//
//   - Tool interface:
//     Manifest() ToolManifest
//     Execute(ctx, run, input, yield)
//   - ToolCall carries Input ToolInput (ArgsJSON + Attachments).
//   - Chunk payload is MIME-aware: Event, Data, MimeType, IsError, Metadata.
//   - Event is strongly typed via EventType (EventProgress/EventResult/EventSuspend).
//   - RunContext carries runtime dependencies (Credentials, State, Services).
//   - Registry runtime is immutable; use RegistryBuilder for setup-time mutation.
//
// Use Extractor when you only need schema generation/validation. Use NewDynamicTool or
// NewProxyTool for runtime schemas (OpenAPI, MCP, etc.).
//
// # Example
//
//	type Args struct { City string `json:"city" jsonschema:"City name"` }
//	type Out struct { Temp float64 `json:"temp"` }
//	tool, _ := toolsy.NewTool("weather", "Get weather", func(_ context.Context, _ toolsy.RunContext, a Args) (Out, error) {
//		return Out{Temp: 22.5}, nil
//	})
//	reg, _ := toolsy.NewRegistryBuilder().Add(tool).Build()
//	call := toolsy.ToolCall{
//		ID: "1", ToolName: "weather",
//		Input: toolsy.ToolInput{ArgsJSON: []byte(`{"city":"Moscow"}`)},
//	}
//	var out Out
//	_ = reg.Execute(ctx, call, func(c toolsy.Chunk) error { return json.Unmarshal(c.Data, &out) })
package toolsy
