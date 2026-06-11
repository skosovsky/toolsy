// Package toolsy provides a type-safe engine for building and executing LLM-callable tools.
//
// # Overview
//
// Pipeline: Go handler + args struct -> NewTool/NewStreamTool -> Tool -> Registry ->
// Execute. Input JSON is validated against generated schema, the handler is called,
// and results are streamed as Chunk values.
//
// # v1.0 contracts
//
//   - Tool interface:
//     Manifest() ToolManifest
//     Execute(ctx, run, input, yield)
//   - ToolCall carries Input ToolInput (CallID + ArgsJSON + Attachments).
//   - Chunk data-plane: Event, Data, MimeType, IsError, Progress.
//   - Chunk control-plane: EventControl + typed ControlSignal (Pause/Yield/Halt/UIAction).
//   - ToolManifest SSOT: ReadOnly, RequiresConfirmation, Dangerous, Idempotent, CompletionPolicy.
//   - Session: in-memory state via SetSessionState/GetSessionState; ExportSnapshot/ImportSnapshot;
//     StateCodecRegistry for typed snapshot roundtrips (see docs/migration-task27.md and docs/adr/adr-task27-typed-contracts.md).
//   - RunCall + ToolOutcome + DecodeOutcomeAs: sync aggregation; business errors in ExecutionError (Error-as-Value).
//   - ValidateManifestContract + ManifestSet: declarative contract checks without Registry.Build.
//   - NewTypedTool, NewDynamicToolFromSpec, ManifestSet, ToolRequirements (v1.0 clear break).
//   - RunEnv: Put/Require/Lookup (deps); SetState/GetState delegate to bound Session;
//     NewRunEnv(session, opts...); session may be nil for DI-only usage.
//   - ToolError: structured errors with Code and Retryable (replaces ClientError/SystemError).
//   - CallParser + DecodeChunkAs for typed host integration (see docs/migration-task22.md).
//   - Registry runtime is immutable; use RegistryBuilder for setup-time mutation.
//   - AsAsyncTool: register via RegistryBuilder.Use(...).Add(AsAsyncTool(base)).Build()
//     so global middleware runs inside the background goroutine (unwrap-wrap in Build).
//     Nested AsAsyncTool layers anywhere in the tool chain are rejected at Build
//     (walk uses ChainUnwrapper; external middleware before Add must implement it).
//     Default background chunk buffer cap is DefaultMaxCollectedChunks (1000);
//     see WithMaxCollectedChunks and ErrAsyncCollectedLimitExceeded.
//
// Use Extractor when you only need schema generation/validation. Use NewDynamicToolFromSpec or
// NewProxyTool for runtime schemas (OpenAPI, MCP, etc.). Use historycodec for canonical
// ToolCall/ToolResult wire format and textprocessor for standalone UTF-8 truncation.
//
// # Example
//
//	type Args struct { City string `json:"city" jsonschema:"City name"` }
//	type Out struct { Temp float64 `json:"temp"` }
//	tool, _ := toolsy.NewTool("weather", "Get weather", func(_ context.Context, _ *toolsy.RunEnv, a Args) (Out, error) {
//		return Out{Temp: 22.5}, nil
//	})
//	reg, _ := toolsy.NewRegistryBuilder().Add(tool).Build()
//	env := toolsy.NewRunEnv(nil)
//	call := toolsy.ToolCall{
//		ToolName: "weather",
//		Input:    toolsy.ToolInput{CallID: "1", ArgsJSON: []byte(`{"city":"Moscow"}`)},
//		Env:      env,
//	}
//	var out Out
//	_ = reg.Execute(ctx, call, func(c toolsy.Chunk) error {
//		decoded, err := toolsy.DecodeChunkAs[Out](c)
//		if err != nil {
//			return err
//		}
//		out = *decoded
//		return nil
//	})
//
// Sync agent loops can aggregate chunks via RunCall:
//
//	sess, _ := toolsy.NewSession(reg)
//	outcome, err := sess.RunCall(ctx, call)
//	if err != nil { /* infrastructure */ }
//	if outcome.ExecutionError != nil { /* business — toolsy.AsToolError */ }
//	result, err := toolsy.DecodeOutcomeAs[Out](outcome)
package toolsy
