# toolsy

Universal AI Tool Engine for Go: build tools from typed handlers, expose JSON Schema to LLM providers, validate arguments, and execute with streaming.

[![Go Reference](https://pkg.go.dev/badge/github.com/skosovsky/toolsy.svg)](https://pkg.go.dev/github.com/skosovsky/toolsy)
[![Build Status](https://github.com/skosovsky/toolsy/workflows/Go/badge.svg)](https://github.com/skosovsky/toolsy/actions)

Go 1.26+ · [License](LICENSE)

## Quick start

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/skosovsky/toolsy"
)

func main() {
	type Args struct {
		City string `json:"city" jsonschema:"City name"`
	}
	type Out struct {
		Temp float64 `json:"temp"`
	}

	tool, err := toolsy.NewTool(
		"weather",
		"Get temperature for city",
		func(_ context.Context, _ *toolsy.RunEnv, a Args) (Out, error) {
			return Out{Temp: 22.5}, nil
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	reg, err := toolsy.NewRegistryBuilder().Add(tool).Build()
	if err != nil {
		log.Fatal(err)
	}

	call := toolsy.ToolCall{
		ToolName: "weather",
		Input: toolsy.ToolInput{
			CallID:   "1",
			ArgsJSON: []byte(`{"city":"Moscow"}`),
		},
	}

	var out Out
	err = reg.Execute(context.Background(), call, func(c toolsy.Chunk) error {
		decoded, decErr := toolsy.DecodeChunkAs[Out](c)
		if decErr != nil {
			return decErr
		}
		out = *decoded
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(out.Temp)
}
```

### Sync agent loop

For synchronous host loops, prefer `Session.RunCall` + `DecodeOutcomeAs` instead of manual chunk assembly:

```go
sess, _ := toolsy.NewSession(reg)
call.Env = toolsy.NewRunEnv(sess)
outcome, err := sess.RunCall(ctx, call)
if err != nil { /* infrastructure */ }
if outcome.ExecutionError != nil { /* business — toolsy.AsToolError */ }
result, _ := toolsy.DecodeOutcomeAs[Out](outcome)
```

See `examples/run_call/main.go`. The quick start above uses low-level `Registry.Execute` for streaming adapters.

## v1.0 API contracts

- `Tool` interface: `Manifest() ToolManifest` and `Execute(ctx, env, input, yield)`.
- `ToolCall` carries `Input toolsy.ToolInput`; old `ToolCall.Args` is removed.
- `ToolInput` contains `CallID`, `ArgsJSON`, and optional `Attachments`.
- `Chunk` data-plane: `Event`, `Data`, `MimeType`, `IsError`, `Progress`.
- `Chunk` control-plane: `EventControl` + typed `ControlSignal` (`PauseSignal`, `YieldSignal`, `HaltSignal`, `UIActionSignal`).
- `Chunk.Event` values: `EventProgress`, `EventResult`, `EventControl`.
- `Chunk.RawData` is removed.
- Runtime `Registry` is immutable. Use `RegistryBuilder` to add tools and middleware before `Build()`.
- Runtime-aware handlers: `NewTool`, `NewTypedTool`, `NewStreamTool`, `NewDynamicToolFromSpec`, `NewProxyTool`.

## Architecture

vNext core is a **stateless tool execution engine**: typed manifests, middleware, streaming chunks, and session policies. Orchestrators (for example `flowy`) own the agent loop, chat persistence, and routing after `CompletionPolicy`. `toolsy` executes tools and emits control signals; the orchestrator applies manifest policy and stores history (`historycodec` wire format).

## Registry setup

Timeouts, retries, and concurrency limits are **not** configured on the registry.
Apply them outside `toolsy` (for example with [`github.com/skosovsky/routery`](https://github.com/skosovsky/routery)) by wrapping tool execution; see `examples/resiliency/main.go` (host loop uses `Session.RunCall`).

The registry recovers panics from tools by default; avoid `WithRecovery()` in `Use()` (it runs before the registry hook and is deprecated for registry stacks).

```go
reg, err := toolsy.NewRegistryBuilder().Use(
	toolsy.WithLogging(slog.Default()),
).Add(
	toolA, toolB,
).Build()
```

The built registry is read-only for runtime calls (`Execute`, `ExecuteIter`, `ExecuteBatchStream`).

### Contract scoping and validation

```go
// Lightweight manifest-only check (no Registry.Build required):
ms, err := toolsy.NewManifestSet(toolA, toolB)
if err != nil {
    return err
}
if err := toolsy.ValidateManifestContract(ms, []string{"book_appointment", "list_slots"}); err != nil {
    return err
}

// Capability: static tool visibility (saves tokens; tools not in subset have no schema in the manifest).
profileReg, err := reg.Subset("book_appointment", "list_slots")
if err != nil {
    return err
}

ms, err := profileReg.ManifestSet()
if err != nil {
    return err
}
if err := toolsy.ValidateManifestContract(ms, []string{"book_appointment", "list_slots"}); err != nil {
    return err
}
```

- **`Subset`**: creates a capability view with only the named tools. Duplicate names are ignored. Unknown names return an error. Subset shares runtime state (shutdown and in-flight tracking) with the root registry.
- **`ValidateManifestContract`**: returns `*ToolError` with `CodeToolsContractMissing` when required tools are missing (`AsToolError` + `FixableArgs` lists missing names). Duplicate names in `requiredNames` are deduplicated. Works with `NewManifestSet` or `reg.ManifestSet()` — no runtime readiness required.
- **`ToolNames`**, **`Has`**, **`GetAllTools`**, **`GetTool`**: map-view introspection only (tool names / membership in the current view). They do not validate runtime readiness; use `ValidateManifestContract` or `Execute` before running tools. A nil `*Registry` is safe for these helpers (empty/false results, no panic).

**Capability vs runtime authorization:** use `Subset` for which tools a profile may use at all. Use middleware for per-call checks (tenant, role, payload) that depend on `context` or arguments.

**Shutdown:** call `Shutdown` only on the root registry owner (for example your app on SIGTERM). Subset views share lifecycle: `subset.Shutdown()` stops the entire registry tree, not just one agent request.

## Tool manifest and policy fields

`ToolManifest` contains:

- `Name`, `Description`, `Parameters`
- `Tags`, `Version`
- `Requirements` (`ToolRequirements`: memory access, session need, permissions)
- `ReadOnly`, `RequiresConfirmation`, `Dangerous`, `Idempotent`
- `CompletionPolicy` (`continue`, `silent_yield`, `halt`)

Built-in `toolkits/*` set policy flags (`ReadOnly`, `Dangerous`, …) on each tool; `toolkits/memory` declares `ToolRequirements` (session + read/write memory). Hosts should add `WithRequirements` to custom tools. `exectool` marks `exec_code` as `Dangerous` by default. MCP proxy tools map server `annotations` hints into the same manifest fields; `read_mcp_resource` is `ReadOnly`.

Example:

```go
tool, err := toolsy.NewTool(
	"delete_user",
	"Delete a user account",
	handler,
	toolsy.WithDangerous(),
	toolsy.WithRequiresConfirmation(),
	toolsy.WithCompletionPolicy(toolsy.CompletionHalt),
	toolsy.WithRequirements(toolsy.ToolRequirements{
		MemoryAccess: toolsy.MemoryAccessReadWrite,
		Permissions:  []toolsy.Permission{"admin"},
	}),
)
if err != nil {
	return err
}
m := tool.Manifest()
_ = m.ReadOnly
_ = m.RequiresConfirmation
```

## toolsy-gen: Contract-First Generator

`toolsy-gen` generates typed DTOs, handler interfaces, and `New...Tool` factories from YAML/JSON manifests for internal core tools.

```bash
go run github.com/skosovsky/toolsy/cmd/toolsy-gen ./tools
```

**Clean-break rules (generation fails on violation):**

- Every parameter in `parameters.properties` must have a non-empty `description`.
- Nested objects (`type: object` inside `properties`) are not supported in v1.
- Unsupported JSON Schema keywords (`$ref`, `oneOf`, `allOf`, `anyOf`, `not`, `patternProperties`) are rejected.

**Supported schema subset:**

- Root `parameters.type` must be `object`.
- Type mapping:
  - `string` -> `string`
  - `string` + `format: date-time` -> `time.Time`
  - `integer` -> `*int64` (top-level; pointer distinguishes missing key from numeric zero)
  - `boolean` -> `*bool` (top-level)
  - `array` -> `[]T` (single level only; no nested arrays)

**Complex payloads (nested objects):**

- Nested `type: object` inside `properties` is rejected.
- For structured payloads, split into multiple flat tools or model a single `string` field that carries JSON text validated in handler code.

**Generated validation (zero-dependency):**

- Required fields from schema `required` are enforced in generated `Validate()` with explicit Go checks (no `validator/v10`, no `validate` struct tags).
- Top-level `integer`/`boolean` use pointers so `Validate()` can detect absent keys without rejecting legitimate `0`/`false` values.
- Parse/validate failures in the factory return `*ToolError` (`CodeValidationFailed` / `CodeSchemaInvalid`) for LLM self-correction.

**Stream tools (`stream: true`):**

- Handler interface uses `ExecuteStream(...) iter.Seq2[string, error]`.
- Factory wraps the proxy tool with `toolsy.AsAsyncTool` (immediate `AsyncAccepted` chunk, stream runs in background).
- Argument parse/validate errors from the embedded proxy surface as tool `Execute` errors when they occur in the background goroutine; the accepted chunk is returned first.

## Session state and RunEnv (DI)

In-memory mutable state lives on `*Session` (`SetSessionState`, `GetSessionState`, `ExportSnapshot`, `ImportSnapshot`).
`*RunEnv` is shared via `ToolCall.Env` for DI and handler access:

- `StateStore` — persisted key/value state (optional)
- `Put` / `Require` / `Lookup` — dependencies (`deps` map, not serialized)
- `SetState` / `GetState` — delegate to the bound `Session` when `NewRunEnv(session)` was used

```go
codecs := toolsy.NewStateCodecRegistry()
_ = toolsy.RegisterJSONCodec[MyState](codecs, "agent")
sess, _ := toolsy.NewSession(reg, toolsy.WithStateCodecRegistry(codecs))
env := toolsy.NewRunEnv(sess, toolsy.WithStateStore(store))
toolsy.Put(env, "db", db)
toolsy.SetSessionState(sess, "trace_id", traceID) // or SetState(env, ...)

call.Env = env
sess.Execute(ctx, call, yield) // validates env is bound to sess
```

Do not pass `Env: nil` on `Session.Execute` if tools use `SetState` — in-memory state will not persist.

### RunCall (sync agent loops)

For synchronous tool calls, `Session.RunCall` aggregates chunks into a `ToolOutcome`:

```go
outcome, err := sess.RunCall(ctx, call)
if err != nil {
    // infrastructure — not found, shutdown, max steps, control signals (partial outcome preserved)
    if toolsy.IsControlError(err) {
        _ = outcome.Controls // Pause/Yield/Halt/UIAction collected before err
    }
    return err
}
if outcome.ExecutionError != nil {
    // business failure — validation, handler errors (Error-as-Value)
    te, _ := toolsy.AsToolError(outcome.ExecutionError)
    _ = te.Code
    return outcome.ExecutionError
}
result, err := toolsy.DecodeOutcomeAs[MyResult](outcome)
```

Business failures must be read from `outcome.ExecutionError`, not only `err != nil`, so progress chunks before the error are preserved.
Legacy text error chunks (`MimeTypeText` + `IsError`) are normalized to structured wire with `CodeInternal`; `RunCall` returns them as **infrastructure** `error`, not `outcome.ExecutionError` (see migration guide).
`WithErrorFormatter` emits structured `ToolError` JSON in error chunks; `RunCall` restores `Code` / `Retryable` / `FixableArgs`.

See [docs/migration-task28.md](docs/migration-task28.md), [docs/adr/adr-task28-hardening.md](docs/adr/adr-task28-hardening.md), and `examples/run_call/main.go`.

### StateCodecRegistry

Register typed codecs for checkpoint roundtrips:

```go
codecs := toolsy.NewStateCodecRegistry()
if err := toolsy.RegisterJSONCodec[MyState](codecs, "agent"); err != nil {
    return err
}
sess, err := toolsy.NewSession(reg,
    toolsy.WithStateCodecRegistry(codecs),
    toolsy.WithStrictStateCodecs(true),
)
snap, _ := sess.ExportSnapshot()
raw, _ := json.Marshal(snap)
restored, _ := toolsy.NewSessionSnapshotFromJSON(raw)
_ = sess.ImportSnapshot(restored)
```

See [docs/migration-task28.md](docs/migration-task28.md) for strict codecs, error chunk normalization, and snapshot hydration. Runnable snapshot example: `examples/session_snapshot/main.go`.

`ToolInput.Attachments` are exposed to handlers as `env.Attachments()` (cloned per call).

`ToolInput.CallID` is the orchestrator/LLM tool call identifier used for metadata tagging in `Registry`/`Session` execution paths and observability middleware.
Direct low-level `Tool.Execute(...)` does not auto-fill `Chunk.CallID`.

## Semantic history truncation (BYOT)

`toolsy` core does not store chat history and does not provide a built-in agent runtime.
For orchestrators, use `github.com/skosovsky/toolsy/history`:

- `history.ApplySemanticTruncation[T]` for dependency-free semantic compression.
- BYOT contracts: `TokenCounter[T]`, `ContextSummarizer[T]`, `MessageInspector[T]`.
- `history.SemanticTruncationReport` for observability handoff.

Minimal flow:

```go
out, report, err := history.ApplySemanticTruncation(
	ctx,
	historySlice,
	maxTokens,
	myCounter,
	mySummarizer,
	myInspector,
	history.WithMinRecentMessages[MyMessage](2),
)
if err != nil {
	return err
}
_ = out
_ = report
```

When output changes, `ApplySemanticTruncation` builds a new result slice with a new backing array.
If no changes are required, it may return the original slice.

OTel recipe for `SemanticTruncationReport` lives in extension docs:
`ext/toolsyotel/README.md`.

See runnable core example: `examples/semantic_truncation/main.go`.

## Policy middleware recipe

Use middleware to stop execution before tool handler code runs:

```go
var ErrRateLimit = errors.New("rate limit exceeded")

type rateLimitTool struct {
	next  toolsy.Tool
	allow func(context.Context) bool
}

func (t *rateLimitTool) Manifest() toolsy.ToolManifest { return t.next.Manifest() }

func (t *rateLimitTool) Execute(
	ctx context.Context,
	env *toolsy.RunEnv,
	input toolsy.ToolInput,
	yield func(toolsy.Chunk) error,
) error {
	if !t.allow(ctx) {
		return ErrRateLimit
	}
	return t.next.Execute(ctx, env, input, yield)
}

func WithRateLimit(allow func(context.Context) bool) toolsy.Middleware {
	return func(next toolsy.Tool) toolsy.Tool {
		return &rateLimitTool{next: next, allow: allow}
	}
}
```

Error propagation differs by execution path:

- `Registry.Execute(...)` returns middleware/tool error directly.
- `Registry.ExecuteIter(...)` emits the error as iterator error.
- `Registry.ExecuteBatchStream(...)` converts non-suspend execution failures (including pre-tool failures like missing tool, validator rejection, and shutdown, plus tool/middleware failures) to `Chunk{IsError: true, MimeType: MimeTypeToolErrorJSON}`, while `ErrStreamAborted` and context cancellation are returned as errors.

Recommended stack for enterprise policies (outer -> inner):

```go
reg, err := toolsy.NewRegistryBuilder().
	Use(
		toolsy.WithTruncation(8000),
		toolsy.WithErrorFormatter(),
		toolsy.WithBudget(),
	).
	Add(tools...).
	Build()
```

Notes:

- `WithTruncation` truncates `text/plain` and `text/markdown` by default; `application/json` truncation is opt-in via `WithTruncationIncludeJSON(true)`.
- Transient retries, timeouts, and bulkheads belong outside `toolsy` (for example `github.com/skosovsky/routery` wrapping tool execution). See `examples/resiliency/main.go`.
- `WithErrorFormatter` may convert terminal errors into `Chunk{IsError: true}` and then return `nil` (soft error).
- `WithErrorFormatter` handles only errors from wrapped tool/middleware execution; pre-tool failures (e.g. `ErrToolNotFound`, `ErrMaxStepsExceeded`, shutdown/validator failures) remain hard errors.
- If you need to classify step success/failure in an orchestrator using `SessionTrack`, use `Chunk.IsError` as the failure signal; `SessionTrack` counts executions, not outcome status.

## Control flow (typed suspend/yield)

Tools emit control signals via `toolsy.YieldControl`:

```go
return toolsy.YieldControl(yield, &toolsy.PauseSignal{Reason: payloadJSON})
```

Orchestrators should treat `ErrPause`, `ErrYield`, `ErrHalt`, and `ErrUIAction` as control-plane outcomes (`toolsy.IsControlError`), not tool failures.
Set manifest policy for routing after successful completion:

```go
toolsy.WithCompletionPolicy(toolsy.CompletionSilentYield) // or CompletionContinue, CompletionHalt
```

## Authorization and idempotency

- Registry-level: `WithAuthorizer` or middleware `WithAuthorization`.
- Idempotent tools: mark with `WithIdempotent()` and wrap registry with `WithIdempotency(store, keyFn)`.

### Session tool choice (RunPolicy)

`RunPolicy` is validated and enforced only on `Session.Execute`. Direct `Registry.Execute` does not apply run policy; use `Registry.Subset` for static tool visibility.

```go
sess, err := toolsy.NewSession(reg, toolsy.WithRunPolicy(toolsy.RunPolicy{
	AllowedTools: []string{"weather", "search"},
}))
if err != nil {
	return err
}
err = sess.Execute(ctx, call, yield)
```

## Canonical history codec and text utilities

Use `github.com/skosovsky/toolsy/historycodec` for wire-format serialization of `ToolCall` and delivered `Chunk` results.
Use `github.com/skosovsky/toolsy/textprocessor` for standalone UTF-8 truncation without a registry.
Semantic chat truncation (BYOT) remains in `github.com/skosovsky/toolsy/history` — see [Semantic history truncation](#semantic-history-truncation-byot).

## Budget middleware

```go
env := toolsy.NewRunEnv(nil)
toolsy.Put(env, toolsy.DepKeyBudget, tracker)
call.Env = env
reg.Execute(ctx, call, yield)
```

## Streaming and iteration

- `Execute(ctx, call, yield)` for callback streaming.
- `ExecuteIter(ctx, call)` for Go 1.23+ `for range` iteration over `(Chunk, error)`.
- `ExecuteBatchStream(ctx, calls, yield)` runs calls in parallel and serializes yield delivery.

Yield errors are converted to `ErrStreamAborted`.

## Async tools

Use `AsAsyncTool(base, WithOnComplete(...))` for fire-and-forget execution with immediate accepted result (`AsyncAccepted` JSON payload in first result chunk).

When registered via `RegistryBuilder`, global middleware from `Use()` runs **inside the background goroutine** (not during the synchronous accept path). Use `WithBackgroundTimeout` on `AsAsyncTool` to cap background work independently of the caller context.

Manual middleware applied before `RegistryBuilder.Add` must implement `toolsy.ChainUnwrapper` so `Build` can detect invalid nested `AsAsyncTool` chains (see `ext/toolsyotel` for an example).

When async tool is executed via `Registry`, background jobs are tracked so `Shutdown` can wait for them to finish. Registry hooks such as `WithOnAfterExecute` run when the synchronous `Execute` path returns (for async tools that is usually right after `AsyncAccepted`), not when background work finishes — use `WithOnComplete` for background completion.

`WithOnComplete` buffers chunks in memory for the completion callback (default cap: 1000). Override with `WithMaxCollectedChunks(n)`. The cap applies in the background collector even without `WithOnComplete`, protecting memory during async execution. When the cap is exceeded, collection stops and `ErrAsyncCollectedLimitExceeded` is passed to `WithOnComplete` even if the base tool ignores yield errors. For very chatty streams, raise the limit or consume chunks via synchronous yield instead of relying on the callback buffer.

### Note on resiliency with async tools

Background execution uses `context.WithoutCancel` on the parent context: cancellation and deadlines from the caller (e.g. a short HTTP request from the LLM) do **not** propagate to the background goroutine, while `context.Value` (tracing, loggers) still does.

Implications for external executors such as [`routery.Timeout`](https://github.com/skosovsky/routery):

- `routery.Timeout(toolsy.AsAsyncTool(tool), d)` limits how long the orchestrator waits for the tool to return the **accepted** response (enqueue is usually fast). It does **not** cap how long the **background** work runs.
- To cap background work, use `WithBackgroundTimeout` on `AsAsyncTool`, or wrap the base tool: `toolsy.AsAsyncTool(routery.Timeout(baseTool, workBudget), toolsy.WithBackgroundTimeout(d))`.
- If you also need a short limit on the accept phase, compose both: e.g. `routery.Timeout(toolsy.AsAsyncTool(routery.Timeout(baseTool, workBudget), ...), acceptBudget)`.

## MCP integration

Use eager lifecycle:

```go
client, err := mcp.Connect(ctx, transport, mcp.WithClientRoots([]string{"/workspace"}))
if err != nil {
	return err
}
defer client.Close()
```

`Connect` performs handshake during creation and returns ready client.

## Migration notes (v1 -> v2 -> vNext)

- Replace `ToolCall.Args` with `ToolCall.Input.ArgsJSON`.
- Replace `ToolCall.ID` with `ToolCall.Input.CallID`.
- Replace runtime `reg.Register(...)` / `reg.Use(...)` with `RegistryBuilder`.
- Replace `ToolManifest`-based logic with `tool.Manifest()` and `ToolRequirements`.
- Replace `NewClient + Initialize` in `mcp` with `Connect`.
- Replace all `RawData` assertions with decoding from `Chunk.Data` based on `Chunk.MimeType`.
- `exectool.WithTimeout` and `RunRequest.Timeout` are removed; pass execution deadlines on the `context` used for `Run` / `Execute` (or use `routery.Timeout` on the tool).

**vNext breaking changes:**

- `Chunk.Metadata` removed — use `Progress` for data-plane progress and `Control` for orchestrator signals.
- System manifest flags moved out of `Metadata`: use `ReadOnly`, `RequiresConfirmation`, `Dangerous`, `Idempotent`, `CompletionPolicy`.
- Human-in-the-loop tools yield `EventControl` + `ErrPause`.
- `EventSuspend` / `ErrSuspend` / `ServiceProvider` removed.
- `NewSession` returns `(*Session, error)` when `RunPolicy` is invalid.
- `RunContext` → `*RunEnv` on `ToolCall.Env`; `BindEnv` → `Put` / `Require` / `Lookup`.
- `ClientError` / `SystemError` → `*ToolError` with `Code` + `Retryable`.
- See [docs/migration-task28.md](docs/migration-task28.md) for CallParser, `DecodeChunkAs`, and dual-namespace RunEnv.

## Zero-resiliency core (post v2)

The registry no longer applies default execution timeouts, concurrency limits, built-in retry middleware, or per-tool `WithTimeout` manifest deadlines. Removed APIs include `WithDefaultTimeout`, `WithMaxConcurrency`, `WithTimeoutMiddleware`, `WithIdempotentRetry`, `ToolOption` `WithTimeout`, and `ToolManifest.Timeout`. Use `context` deadlines and wrap execution with [`routery`](https://github.com/skosovsky/routery) (or your own middleware) instead; see `examples/resiliency/main.go`. Sandbox adapters honor only the `context` passed to `Run` (no separate `RunRequest` timeout field); limit `exec_code` runtime via the execution `ctx` or wrappers like `routery.Timeout` around the tool.

gRPC reflection helpers take an injected `grpc.ClientConnInterface` (no dial inside `toolsy`). HTTP toolkits (`httptool`, `web`, `document`) use `httptool.SafeDialTransport` by default; pass `WithHTTPClient` to merge only `Timeout`. See [docs/migration-task29.md](docs/migration-task29.md) for enterprise toolkit IoC and SSRF unification.

## Contracts modules

`contracts/openapi`, `contracts/graphql`, `contracts/grpc` return `[]toolsy.Tool`.

Register tools at setup time through builder:

```go
builder := toolsy.NewRegistryBuilder()
builder.Add(openapiTools...)
builder.Add(graphqlTools...)
builder.Add(grpcTools...)
reg, err := builder.Build()
```

## Testing helpers

`testutil.MockTool` provides configurable `ManifestVal` and `ExecuteFn`.
`testutil.NewTestRegistry(...)` builds a registry with test-safe defaults.
