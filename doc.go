// Package toolsy provides a type-safe engine for building and executing LLM-callable tools.
//
// # Overview
//
// Pipeline: Go handler + args struct -> NewTool/NewStreamTool -> Tool -> Registry ->
// Execute. Input JSON is validated against generated schema, the handler is called,
// and results are streamed as Chunk values.
//
// # vNext contracts
//
//   - Tool interface:
//     Manifest() ToolManifest
//     Execute(ctx, run, input, yield)
//   - ToolCall carries Input ToolInput (CallID + ArgsJSON + Attachments).
//   - Chunk data-plane: Event, Data, MimeType, IsError, Progress.
//   - Chunk control-plane: EventControl + typed ControlSignal (Pause/Yield/Halt/UIAction).
//   - ToolManifest SSOT: ReadOnly, RequiresConfirmation, Dangerous, Idempotent, CompletionPolicy.
//   - RunEnv: BindEnv / EnvFromContext for typed application dependencies (replaces string-key Services).
//   - Registry runtime is immutable; use RegistryBuilder for setup-time mutation.
//
// Use Extractor when you only need schema generation/validation. Use NewDynamicTool or
// NewProxyTool for runtime schemas (OpenAPI, MCP, etc.). Use historycodec for canonical
// ToolCall/ToolResult wire format and textprocessor for standalone UTF-8 truncation.
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
//		ToolName: "weather",
//		Input: toolsy.ToolInput{CallID: "1", ArgsJSON: []byte(`{"city":"Moscow"}`)},
//	}
//	var out Out
//	_ = reg.Execute(ctx, call, func(c toolsy.Chunk) error { return json.Unmarshal(c.Data, &out) })
package toolsy
