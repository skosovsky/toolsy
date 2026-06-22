// Package toolsy provides a type-safe engine for building and executing LLM-callable tools.
//
// # Overview
//
// Pipeline: Go typed handler + args struct -> NewTypedTool -> Registry.View ->
// Session.RunCall. Input JSON is validated against generated schema, policy runs before
// handlers, and results are returned as ToolOutcome values. Low-level Execute remains
// available for streaming adapters and transport glue that need direct Chunk handling.
//
// # Runtime contracts
//
//   - Tool interface:
//     Manifest() ToolManifest
//     Execute(ctx, run, input, yield)
//   - ToolCall carries Input ToolInput (CallID + ArgsJSON + Attachments).
//   - Chunk data-plane: Event, Data, MimeType, IsError, Progress.
//   - Chunk control-plane: EventControl + typed ControlSignal (Pause/Yield/Halt/UIAction).
//   - ToolManifest SSOT: ReadOnly, RequiresConfirmation, Dangerous, Idempotent, CompletionPolicy.
//   - Session: in-memory state via SetSessionState/GetSessionState; ExportSnapshot/ImportSnapshot;
//     StateCodecRegistry for typed snapshot roundtrips (see docs/migration-task28.md and docs/adr/adr-task28-hardening.md).
//   - RunCall + ToolOutcome + DecodeOutcomeAs: sync aggregation; business errors in ExecutionError (Error-as-Value).
//   - ValidateManifestContract + ManifestSet: declarative contract checks without Registry.Build.
//   - NewTypedTool, NewDynamicToolFromSpec, ManifestSet, ToolRequirements.
//   - RunEnv: Put/Require/Lookup (deps); SetState/GetState delegate to bound Session;
//     NewRunEnv(session, opts...); session may be nil for DI-only usage.
//   - ToolError: structured errors with Code and Retryable (replaces ClientError/SystemError).
//   - CallParser + DecodeChunkAs for typed host integration (see docs/migration-task28.md).
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
// The primary synchronous host path is typed tool + registry view + RunCall.
//
//	type Args struct { City string `json:"city" jsonschema:"City name"` }
//	type Out struct { Temp float64 `json:"temp"` }
//	type Subject struct { ID string }
//	type Scope struct { Workspace string }
//	tool, _ := toolsy.NewTypedTool(toolsy.TypedToolSpec[Subject, Scope, Args, Out, struct{}]{
//		Name: "weather",
//		Description: "Get weather",
//		Handler: func(_ context.Context, _ toolsy.TypedCallContext[Subject, Scope], _ *toolsy.RunEnv, a toolsy.ValidatedArgs[Args]) (toolsy.ToolResult[Out, struct{}], error) {
//			return toolsy.NewToolResult[Out, struct{}](Out{Temp: 22.5}), nil
//		},
//	})
//	reg, _ := toolsy.NewRegistryBuilder().Add(tool).Build()
//	view, _ := reg.View(toolsy.RegistryViewSpec{
//		ToolNames: []string{"weather"},
//		RequiredToolNames: []string{"weather"},
//	})
//	sess, _ := view.NewSession()
//	call := toolsy.ToolCall{
//		ToolName: "weather",
//		Input:    toolsy.ToolInput{CallID: "1", ArgsJSON: []byte(`{"city":"Moscow"}`)},
//		Env:      toolsy.NewRunEnv(sess),
//		CallContext: toolsy.NewCallContext(
//			Subject{ID: "user-1"},
//			Scope{Workspace: "default"},
//		),
//	}
//	outcome, err := sess.RunCall(ctx, call)
//	if err != nil { /* infrastructure */ }
//	if outcome.ExecutionError != nil { /* business — toolsy.AsToolError */ }
//	result, err := toolsy.DecodeOutcomeAs[Out](outcome)
package toolsy
