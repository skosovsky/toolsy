# Migration guide: task22 (Typed E2E Pipeline)

## ToolError (replaces ClientError / SystemError)

Use a single structured envelope:

```go
te, ok := toolsy.AsToolError(err)
if ok && te.Code == toolsy.CodeValidationFailed {
    // orchestrator routing
}
```

| Before                            | After                                                             |
| --------------------------------- | ----------------------------------------------------------------- |
| `*ClientError`                    | `*ToolError` with `CodeValidationFailed` / `CodeSchemaInvalid`    |
| `*SystemError`                    | `*ToolError` with `CodeInternal`                                  |
| `IsClientError` / `IsSystemError` | `AsToolError` + `ClientCorrectable(te.Code)` or compare `te.Code` |

Factories: `NewValidationError`, `NewSchemaError`, `NewJSONParseError`, `NewInternalError`, `NewDependencyMissingError`, `NewToolNotFoundError`, `NewTimeoutError`, `NewShutdownError`, `NewMaxStepsExceededError`, `NewRegistryStateError`, `NewToolsContractMissingError`.

`NewJSONParseError` wraps invalid argument or result JSON with `CodeSchemaInvalid`, `Reason: "invalid JSON"`, and parse details in `Unwrap` (same contract as internal `wrapJSONParseError` in `NewTool` / `NewDynamicTool`).

`ValidateContract` returns `*ToolError` (not a separate missing-tools type). Use `errors.As` → `*ToolError` and `te.Code == CodeToolsContractMissing`.

Orchestrator routing codes (non-exhaustive):

| Code                       | When                                                                              |
| -------------------------- | --------------------------------------------------------------------------------- |
| `CodeToolNotFound`         | Unknown tool name                                                                 |
| `CodeTimeout`              | Context deadline / execution timeout (`Retryable` may be true)                    |
| `CodeShutdown`             | Registry shut down                                                                |
| `CodeMaxStepsExceeded`     | Session step budget exhausted                                                     |
| `CodeRegistryNotReady`     | Registry not built / runtime state missing                                        |
| `CodeInternal`             | Unexpected failure (`NewInternalError`; details via `Unwrap` only)                |
| `CodeToolsContractMissing` | `ValidateContract`: required tools not registered (`FixableArgs` = missing names) |

Use `toolsy.WithSafeMessage(te, "user-safe text")` for LLM-facing copy; `WithErrorFormatter` prefers `SafeMessage` over `Reason`.

Control-plane signals (`ErrPause`, `ErrHalt`, …) stay separate — use `toolsy.IsControlError`, not `ToolError.Code`.

## RunEnv (replaces RunContext + BindEnv)

Since v0.9.0, `NewRunEnv` takes a `*Session` as the first argument. See [migration-task23.md](migration-task23.md) for in-memory state on `Session`.

```go
env := toolsy.NewRunEnv(nil, toolsy.WithStateStore(store))
toolsy.Put(env, toolsy.DepKeyBudget, tracker) // dependencies — init only

reg.Execute(ctx, toolsy.ToolCall{
    ToolName: "my_tool",
    Input:    toolsy.ToolInput{ArgsJSON: raw},
    Env:      env,
}, yield)
```

For mutable in-memory state between tool calls, bind a session: `sess, _ := toolsy.NewSession(reg)` then `env := toolsy.NewRunEnv(sess)` and use `SetSessionState` / `SetState(env, ...)`.

| Before              | After                                         |
| ------------------- | --------------------------------------------- |
| `RunContext`        | `*RunEnv`                                     |
| `ToolCall.Run`      | `ToolCall.Env`                                |
| `BindEnv(ctx, app)` | `Put(env, key, dep)` on shared `*RunEnv`      |
| `run.State`         | `env.StateStore`                              |
| `SetState` for DI   | **Do not** — use `Put` / `Require` / `Lookup` |

## DecodeChunkAs

```go
out, err := toolsy.DecodeChunkAs[MyResult](chunk) // JSON result chunks only
text, err := toolsy.DecodeChunkAsText(chunk)      // text/plain
```

## CallParser

Map LLM SDK parts to `[]toolsy.ContentPart`, then:

```go
// OpenAI-style: one tool_call with streaming argument deltas (same tool_call.id)
parts := []toolsy.ContentPart{
    {Type: toolsy.ContentTypeText, Text: "I'll search."},
    {Type: toolsy.ContentTypeToolCall, ToolCallID: "call_abc", ToolName: "search", Args: `{"q":`},
    {Type: toolsy.ContentTypeToolCall, ToolCallID: "call_abc", ToolName: "search", ArgsChunk: `"golang"}`},
}

// Anthropic-style: tool_use block (map content block → ContentPart; Args holds full JSON when not streamed)
parts = append(parts, toolsy.ContentPart{
    Type: toolsy.ContentTypeToolCall, ToolCallID: "toolu_01", ToolName: "search",
    Args: `{"q":"golang"}`,
})

raw, err := toolsy.StandardCallParser{}.ExtractExactlyOne(parts, "search")
```

Parts with the same non-empty `ToolCallID` are merged (`Args` + `ArgsChunk`).

## Output schema

Typed `NewTool[T, R]` fills `ToolManifest.OutputSchema` automatically. Override with `toolsy.WithOutputSchema`.

`NewStreamTool` does **not** auto-generate output schema (no single result type `R`). Set `WithOutputSchema` explicitly or describe streamed chunk shapes in the tool description.

## RunEnv nil safety

`Put` and `SetState` on a nil `*RunEnv` are intentional no-ops. Always create `env := toolsy.NewRunEnv(nil, ...)` (or `NewRunEnv(session, ...)`) and pass the same pointer on `ToolCall.Env`. `Require` returns `CodeDependencyMissing` when env is nil.

## Host orchestrator (outside toolsy)

toolsy provides `AsToolError`, `ToolError.Code`, `ToolError.Retryable`, and `ClientCorrectable`. The host application (e.g. kosmify/flowy) should:

- Route retries and escalation using `te.Code` and `te.Retryable`, not `strings.Contains` on error text.
- Use `DecodeChunkAs[T]` / typed tool handlers for results instead of `map[string]any`.
- Share one `*RunEnv` per session via `ToolCall.Env` (`Put` deps at init). For in-memory state use `SetSessionState(sess, ...)` or `SetState` on `NewRunEnv(sess)` — see [migration-task23.md](migration-task23.md).

This migration is a separate task in the host repository; it is not part of the toolsy library release.
