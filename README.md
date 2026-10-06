# toolsy

Universal AI Tool Engine for Go: build tools from typed handlers, expose JSON Schema to LLM providers, validate arguments, and execute with streaming.

[![Go Reference](https://pkg.go.dev/badge/github.com/skosovsky/toolsy.svg)](https://pkg.go.dev/github.com/skosovsky/toolsy)
[![Build Status](https://github.com/skosovsky/toolsy/workflows/Go/badge.svg)](https://github.com/skosovsky/toolsy/actions)

Go 1.27.1+ · [License](LICENSE)

## Quick start

For ordinary host execution with bound approval and durable replay, see
[approval_journal](examples/approval_journal). Prepared execution contracts and
clear-break migration are documented in [execution-contract](docs/execution-contract.md)
and [migration-task35](docs/migration-task35.md). Input-number, structure and HTTP
policy changes are in [migration-task41](docs/migration-task41.md).

```go
package main

import (
	"context"
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
	type Subject struct {
		ID string
	}
	type Scope struct {
		Workspace string
	}

	tool, err := toolsy.NewTypedTool(toolsy.TypedToolSpec[Subject, Scope, Args, Out, struct{}]{
		Name:        "weather",
		Description: "Get temperature for city",
		Handler: func(
			_ context.Context,
			_ toolsy.TypedCallContext[Subject, Scope],
			_ *toolsy.RunEnv,
			_ toolsy.ValidatedArgs[Args],
		) (toolsy.ToolResult[Out, struct{}], error) {
			return toolsy.NewToolResult[Out, struct{}](Out{Temp: 22.5}), nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	reg, err := toolsy.NewRegistryBuilder().Add(tool).Build()
	if err != nil {
		log.Fatal(err)
	}
	view, err := reg.View(toolsy.RegistryViewSpec{
		ToolNames:         []string{"weather"},
		RequiredToolNames: []string{"weather"},
		Reason:            "weather profile",
		Owner:             "agent",
	})
	if err != nil {
		log.Fatal(err)
	}
	sess, err := view.NewSession()
	if err != nil {
		log.Fatal(err)
	}

	call := toolsy.ToolCall{
		ToolName: "weather",
		Input: toolsy.ToolInput{
			CallID:   "1",
			ArgsJSON: []byte(`{"city":"Moscow"}`),
		},
		Env: toolsy.NewRunEnv(sess),
		CallContext: toolsy.NewCallContext(
			Subject{ID: "user-1"},
			Scope{Workspace: "default"},
		),
	}

	outcome, err := sess.RunCall(context.Background(), call)
	if err != nil {
		log.Fatal(err)
	}
	if outcome.ExecutionError != nil {
		log.Fatal(outcome.ExecutionError)
	}
	out, err := toolsy.DecodeOutcomeAs[Out](outcome)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(out.Temp)
}
```

### Sync agent loop

For synchronous host loops, use `Session.RunCall` + `DecodeOutcomeAs` instead of manual chunk assembly:

```go
sess, _ := toolsy.NewSession(reg)
call.Env = toolsy.NewRunEnv(sess)
outcome, err := sess.RunCall(ctx, call)
if err != nil { /* infrastructure */ }
if outcome.ExecutionError != nil { /* business — toolsy.AsToolError */ }
result, _ := toolsy.DecodeOutcomeAs[Out](outcome)
```

See `examples/run_call/main.go`. Use low-level `Registry.Execute` only inside streaming adapters or transport glue that must consume chunks directly.

## API contracts

Output schemas are executable contracts for successful JSON result bytes. Builders compile schemas before execution and validate before persistence/delivery and on replay. Text/binary, progress, controls, intentional empty/noop and business-error outputs have distinct semantics. Pre-encoded JSON must be valid; formatter wire limits reject oversized values without slicing JSON. See [result contract](docs/result-contract.md).

- `Tool` interface: `Manifest() ToolManifest` and `Execute(ctx, env, input, yield)`.
- `ToolCall` carries `Input toolsy.ToolInput` and optional `CallContext` for typed subject/scope.
- `ToolInput` contains `CallID`, `ArgsJSON`, and optional `Attachments`.
- `Chunk` data-plane: `Event`, `Data`, `MimeType`, `IsError`, `Progress`, `TypedResult`, `EmptyResult`, `Noop`, `Effects`.
- `Chunk` control-plane: `EventControl` + typed `ControlSignal` (`PauseSignal`, `YieldSignal`, `HaltSignal`, `UIActionSignal`).
- `Chunk.Event` values: `EventProgress`, `EventResult`, `EventControl`.
- `Chunk.RawData` is removed.
- Runtime `Registry` is immutable. Use `RegistryBuilder` to add tools and middleware before `Build()`.
- Production agent handlers: `NewTypedTool` and `NewPolicyToolFromSpec`.
- Existing generic tools can be hardened with `NewPolicyTool`.
- Policy-aware generic tools require an `ArgsBinder` that returns canonical raw bytes for the wrapped raw handler.
- Low-level constructors: `NewTool`, `NewStreamTool`, `NewDynamicToolFromSpec`, `NewProxyTool`.

## Architecture

Core is a **stateless tool execution engine**: typed manifests, middleware, streaming chunks, call context, registry views, and session policies. External orchestrators own the agent loop, chat persistence, and routing after `CompletionPolicy`. `toolsy` executes tools, enforces its configured policy/capability boundary, and emits typed results, effects, and control signals.

## Registry setup

Timeouts, retries, and concurrency limits are **not** configured on the registry.
Apply them outside `toolsy` by wrapping tool execution; see `examples/resiliency/main.go` (host loop uses `Session.RunCall`).

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

// Capability view: static tool visibility plus optional execution policy.
profileView, err := reg.View(toolsy.RegistryViewSpec{
    ToolNames: []string{"book_appointment", "list_slots"},
    Reason: "booking profile",
    Owner: "agent-profile",
    PolicyID: "booking-profile-policy",
    Policy: toolsy.NewRequirementsPolicy(func(ctx context.Context, req toolsy.RequirementsPolicyRequest[UserSubject, WorkspaceScope]) toolsy.Decision {
        if !req.Context.Subject.Can(req.Requirements.Permissions...) {
            return toolsy.DenyDecision("missing permission", "permissions")
        }
        return toolsy.AllowDecision()
    }),
})
if err != nil {
    return err
}

ms, err := profileView.ManifestSet()
if err != nil {
    return err
}
if err := toolsy.ValidateManifestContract(ms, []string{"book_appointment", "list_slots"}); err != nil {
    return err
}
```

- **`Registry.View`**: creates a first-class capability object with tool names, manifest set, durable snapshot identity, optional policy, execution enforcement, and shared root lifecycle. Calls to tools outside the view manifest return `CodeCapabilityDenied`.
- **`Subset`**: view-backed alias for a named tool set. Prefer `Registry.View` when the scope needs snapshot identity, required tool validation, policy, prompt contract, or restore requirements.
- **`ValidateManifestContract`**: returns `*ToolError` with `CodeToolsContractMissing` when required tools are missing (`AsToolError` + `FixableArgs` lists missing names). Duplicate names in `requiredNames` are deduplicated. Works with `NewManifestSet` or `reg.ManifestSet()` — no runtime readiness required.
- **`ToolNames`**, **`Has`**, **`GetAllTools`**, **`GetTool`**: map-view introspection only (tool names / membership in the current view). They do not validate runtime readiness; use `ValidateManifestContract` or `Execute` before running tools. A nil `*Registry` is safe for these helpers (empty/false results, no panic).

**Capability vs runtime authorization:** use `Registry.View` for which tools a profile may use at all, `NewRequirementsPolicy` / `WithRequirementsPolicy` for manifest requirements against typed subject/scope, and typed tool policy for per-call args checks. Root registry policies require a stable policy ID through `WithPolicy`/`WithRequirementsPolicy`; that ID is part of `SessionBinding` for checkpoint/rebind safety.

**Shutdown:** call `Shutdown` only on the root registry owner (for example your app on SIGTERM). Registry views share lifecycle: `view.Shutdown()` stops the entire registry tree, not just one agent request.

## Tool manifest and policy fields

`ToolManifest` contains:

- `Name`, `Description`, `Parameters`
- `Tags`, `Version`
- `Requirements` (`ToolRequirements`: memory access, session need, permissions)
- `ReadOnly`, `RequiresConfirmation`, `Dangerous`, `Idempotent`
- `CompletionPolicy` (`continue`, `silent_yield`, `halt`)

Built-in `toolkits/*` set policy flags (`ReadOnly`, `Dangerous`, …) on each tool; `toolkits/memory` declares `ToolRequirements` (session + read/write memory). Custom tools should declare `WithRequirements`, then attach `WithRequirementsPolicy("stable-policy-id", ...)` or `RegistryViewSpec.Policy: NewRequirementsPolicy(...)` with a stable `PolicyID` so registry/session execution enforces requirements before validators and handlers run.

Example:

```go
tool, err := toolsy.NewTypedTool(toolsy.TypedToolSpec[UserSubject, WorkspaceScope, DeleteUserArgs, DeleteUserResult, struct{}]{
	Name:        "delete_user",
	Description: "Delete a user account",
	Handler: func(
		ctx context.Context,
		call toolsy.TypedCallContext[UserSubject, WorkspaceScope],
		env *toolsy.RunEnv,
		args toolsy.ValidatedArgs[DeleteUserArgs],
	) (toolsy.ToolResult[DeleteUserResult, struct{}], error) {
		return deleteUserHandler(ctx, call, env, args.Value)
	},
	Options: []toolsy.ToolOption{
		toolsy.WithDangerous(),
		toolsy.WithRequiresConfirmation(),
		toolsy.WithCompletionPolicy(toolsy.CompletionHalt),
		toolsy.WithRequirements(toolsy.ToolRequirements{
			MemoryAccess: toolsy.MemoryAccessReadWrite,
			Permissions:  []toolsy.Permission{"admin"},
		}),
	},
})
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
- Nested objects and nested arrays are unsupported.
- Unknown or inapplicable JSON Schema keywords are rejected, including references and composition.

**Supported schema subset:**

- Root `parameters.type` must be `object`.
- Type mapping:
  - `string` -> `string`
  - `string` + `format: date-time` -> `string` (schema annotation)
  - `integer` -> `*json.Number` (top-level; exact integer and presence)
  - `boolean` -> `*bool` (top-level)
  - `array` -> `[]T` (single level only; no nested arrays)
  - nullable top-level union -> `json.RawMessage` (omitted/null/value)

Every DTO includes `RawJSON` with the complete accepted arguments, including
undeclared keys when the source schema permits them. See the full
[generator contract](docs/generator-contract.md) for constraints and limits.

**Complex payloads (nested objects):**

- Nested `type: object` inside `properties` is rejected.
- For structured payloads, split into multiple flat tools or model a single `string` field that carries JSON text validated in handler code.

**Schema validation:**

- Generated factories use the existing bounded schema validator; DTOs have no separate `Validate()` method.
- `required` enforces presence. Empty strings/arrays, zero and false remain valid unless the source schema adds constraints.
- Parse/validate failures in the factory return `*ToolError` (`CodeValidationFailed` / `CodeSchemaInvalid`) for LLM self-correction.

**Stream tools (`stream: true`):**

- Handler interface uses `ExecuteStream(...) iter.Seq2[string, error]`.
- Factory returns an ordinary synchronous proxy: caller Execute observes progress, terminal result and errors. Invalid arguments fail before handler dispatch; caller cancellation remains attached.
- Async execution is explicit host composition using `AsAsyncTool(base, WithBackgroundTimeout(...), WithMaxCollectedChunks(...), WithOnComplete(...))`. Accepted acknowledges scheduling; background terminal/errors arrive through the completion hook, not the returned caller. See [the runnable example](examples/generated_stream/main.go) and [generator recovery/lifecycle contract](docs/generator-contract.md).

## Session state and RunEnv (DI)

In-memory mutable state lives on `*Session` (`SetSessionState`, `GetSessionState`, `ExportSnapshot`, `ImportSnapshot`).
Registry and binding are one immutable session configuration. `Rebind` validates
and publishes atomically; in-flight `Execute`/`RunCall` retain their captured
registry, while later calls observe the new configuration. Checkpoints export one
binding for both outer metadata and inner snapshot. Codecs and `MarshalJSON`
callbacks run without state/configuration locks; state-map slots are copied before
encoding, while referenced host values must remain immutable during encoding.
Registry configuration must remain stable after setup. `NewSession` freezes its
`StateCodecRegistry`: register every slot before constructing the first session.
Later registration returns `ErrStateCodecRegistryFrozen`. Codec callbacks and
referenced pointers/maps/slices remain host-owned; callers must synchronize their
access. See [task41 migration](docs/migration-task41.md).

`SessionCheckpoint` persists state plus binding. It excludes `RunPolicy`, call
limits/counts, dependencies, and workflow continuation. Restore supplies current
host authority and fresh session counters; durable budgets belong to the host.
`*RunEnv` is shared via `ToolCall.Env` for DI and handler access:

- `StateStore` — persisted key/value state (optional)
- `Put` / `Require` / `Lookup` — dependencies (`deps` map, not serialized)
- `SetState` / `GetState` — delegate to the bound `Session` when `NewRunEnv(session)` was used

Subject, scope, and request-local policy data belong in `ToolCall.CallContext`, not in string-keyed `RunEnv` state:

```go
call.CallContext = toolsy.NewCallContext(
    UserSubject{ID: "u1"},
    WorkspaceScope{ID: "w1"},
)
```

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
    // infrastructure — not found, shutdown, max calls, control signals (partial outcome preserved)
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
if outcome.Status == toolsy.OutcomeEmptySuccess {
    return nil
}
result, err := toolsy.DecodeOutcomeAs[MyResult](outcome)
effects, err := toolsy.DecodeOutcomeEffectsAs[MyEffect](outcome)
_ = effects
```

Business failures must be read from `outcome.ExecutionError`, not only `err != nil`, so progress chunks before the error are preserved.
Legacy text error chunks (`MimeTypeText` + `IsError`) are normalized to structured wire with `CodeInternal`; `RunCall` returns them as **infrastructure** `error` with `OutcomeInfrastructureError`, not `outcome.ExecutionError` (see migration guide).
`WithErrorFormatter` emits structured `ToolError` JSON in error chunks; `RunCall` restores `Code` / `Retryable` / `FixableArgs`.

See [docs/migration-task31.md](docs/migration-task31.md), [docs/migration-task28.md](docs/migration-task28.md), [docs/adr/adr-task28-hardening.md](docs/adr/adr-task28-hardening.md), and `examples/run_call/main.go`.

### StateCodecRegistry

Register typed codecs for checkpoint roundtrips:

The first `NewSession` finalizes the shared registry after constructor validation.
Explicit `codecs.Freeze()` is also available and idempotent. Build a new registry
for schema changes. Required slots must be present at export and import; registered
non-nullable slots cannot encode JSON null. Custom codecs must honor their
roundtrip/schema contract and be safe for concurrent calls. Library map replacement
is atomic on import; host callback effects are not rolled back on decode failure.

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

## Conversation compaction

Hosts own conversation retention, token budgets and summarization. Compose
contexty in the host when needed; toolsy has no mandatory contexty dependency.
The generic `toolsy/history` package has been removed. See
[capability mapping and migration](docs/history-compaction-migration.md).
Tool-result transcripts and `historycodec` remain available independently.

## Policy and capability recipe

Use registry policy to stop execution before validators and tool handler code run:

```go
reg, err := toolsy.NewRegistryBuilder(
    toolsy.WithPolicy("dangerous-tool-policy", toolsy.PolicyFunc(func(ctx context.Context, req toolsy.PolicyRequest) toolsy.Decision {
        if req.Manifest.Dangerous {
            return toolsy.DenyDecision("dangerous tool requires a narrower capability")
        }
        return toolsy.AllowDecision()
    })),
).Add(tools...).Build()
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
- Transient retries, timeouts, and bulkheads belong outside `toolsy` as execution wrappers. See `examples/resiliency/main.go`.
- `WithErrorFormatter` may convert terminal errors into `Chunk{IsError: true}` and then return `nil` (soft error).
- `WithErrorFormatter` handles only errors from wrapped tool/middleware execution; pre-tool failures (e.g. `ErrToolNotFound`, `ErrMaxCallsExceeded`, shutdown/validator failures) remain hard errors.
- For call outcomes, inspect the Execute error and result chunks, or use RunCall's ToolOutcome. `SessionTrack.CallAttempts` counts admission attempts, including budget rejection; it does not classify outcomes.

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

- Registry-level: prefer `WithPolicy`; `WithAuthorizer` and `WithAuthorization` accept `AuthorizationRequest` with manifest, input, call context, and view identity.
- Result cache: supply a per-attempt host `CacheEligibility` predicate and create `NewResultCache(store, eligibility, partition, codec, maxBytes)` and install it with `WithExecutionProfile`. Binding and current typed policy run before replay; the host provides a trusted freshness partition and complete outcome codec. Idempotent/ReadOnly hints alone never enable reuse. This cache does not guarantee atomic duplicate dispatch. See [execution contract](docs/execution-contract.md).

### Typed result representations

`ToolResult` has explicit payload states. Ordinary Value is JSON encoded;
nonempty Raw replaces only its wire representation and retains the typed Value.
RawMimeType applies only with nonempty Raw (default application/octet-stream).
Empty and Noop omit wire bytes while retaining typed Value and delivery
metadata/controls, and are mutually exclusive. Empty may report effects; Noop
cannot declare effects. Contradictory declarations return an INTERNAL
ResultContractError after the handler, without permission to retry its effects.

Nested json.RawMessage in generated output schemas accepts any JSON value;
explicit SchemaRegistry type mappings override that default. Input RawMessage
keeps its object default. Top-level RawMessage/custom encoders need
WithOutputSchema to constrain their wire shape. Explicit output schemas take
precedence; JSON wire bytes are validated without re-encoding arbitrary BYOT
values. See [task41 migration](docs/migration-task41.md).

### Session tool choice (RunPolicy)

`RunPolicy` is captured by value: AllowedTools and CatalogRequiredTools slices
are copied at option creation and session construction. Register/catalog builders
remain stable after setup; caller mutations after capture cannot change admission.
`AllowedTools` and `ForcedTool` restrict session calls. `CatalogRequiredTools`
requires names in the visible catalog at construction; it does not require calls
or act as another whitelist. Direct `Registry.Execute` does not apply RunPolicy;
use `Registry.View` for static visibility and capability policy.

`WithMaxCalls(n)` limits outer `Execute`/`RunCall` admissions, with zero unlimited
and negatives rejected by construction. `Track().CallAttempts()` counts attempts
that pass session selection, including budget rejection, environment/argument
errors, cancellation and replay. Policy rejection and nil registry consume nothing.
Internal retries count once; nested Session calls count separately. This is a
session call limit; the host owns agent iterations and durable budgets. See
[task41 migration](docs/migration-task41.md) for the API and wire-code break.

```go
sess, err := toolsy.NewSession(reg, toolsy.WithRunPolicy(toolsy.RunPolicy{
	AllowedTools: []string{"weather", "search"},
}))
if err != nil {
	return err
}
err = sess.Execute(ctx, call, yield)
```

## Transcript codec and text utilities

Use `github.com/skosovsky/toolsy/historycodec` for strict version 2 raw transcripts with explicit delivery/audience and replay metadata. Typed values, effects, controls, runtime context and attachments fail explicitly; project an execution record deliberately before encoding. See [supported transcript contract](historycodec/README.md). For complete typed cache/journal persistence use `ResultCodec`, not the transcript codec. Version 1 is unsupported.
Use `github.com/skosovsky/toolsy/textprocessor` for standalone UTF-8 truncation without a registry.
Conversation compaction belongs to the host/contexty — see [migration](docs/history-compaction-migration.md).

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

Implications for external executor wrappers:

- A timeout wrapper around `toolsy.AsAsyncTool(tool)` limits how long the orchestrator waits for the **accepted** response (enqueue is usually fast). It does **not** cap how long the **background** work runs.
- To cap background work, use `WithBackgroundTimeout` on `AsAsyncTool`, or wrap the base tool before converting it to async.
- If you also need a short limit on the accept phase, compose both limits explicitly.

## MCP integration

The MCP bridge supports exactly protocol revision `2026-07-28`. `Connect` performs strict `server/discover` up front and returns a ready client only when the server's `supportedVersions` contains that exact revision.

```go
transport := mcp.NewStreamableHTTPTransport("https://example.com/mcp")
client, err := mcp.Connect(ctx, transport)
if err != nil {
	return err
}
defer client.Close()
```

There is no `initialize` fallback, session ID, HTTP GET/resume/DELETE path or automatic retry. HTTP uses POST with exact version/method routing headers; `Mcp-Name` is emitted for tool calls, prompt gets and resource reads, and `x-mcp-header` tool arguments are mirrored as validated `Mcp-Param-*` fields. Stdio cancellation sends `notifications/cancelled` after delivery; HTTP cancellation closes the request-scoped response stream. Results are tagged with `resultType`, cacheable results expose `ttlMs`/`cacheScope`, and invalidations use explicit `subscriptions/listen`.

For stable host-side cache identity, `mcp.ComputeSnapshotDigest` validates and hashes supported discovery/list/read snapshots using canonical, snapshot-type-separated encoding that includes cache metadata and ordered entries. Official MCP capability extensions are preserved inert and may use explicit BYO codecs; caller `_meta` still cannot forge MCP-reserved MetaObject namespaces.

See [the module README](mcp/README.md) and [task34 migration guide](docs/migration-task34.md).

Remote annotations remain untrusted hints. Use `mcp.WithToolPolicyMapper` for
explicit host classification; current authorization and discovery generation
protect cached delivery as well as dispatch. HTTP authentication failures expose
bounded challenge diagnostics without automatic authentication or retries.

The separate [agents bridge](agents/README.md) reports confirmed terminal outcomes
and accepted background task references. Hosts own persistence and continuation;
see the [remote bridge contract](docs/remote-bridge-contract.md).

## Historical Migration Notes

- Replace `ToolCall.Args` with `ToolCall.Input.ArgsJSON`.
- Replace `ToolCall.ID` with `ToolCall.Input.CallID`.
- Replace runtime `reg.Register(...)` / `reg.Use(...)` with `RegistryBuilder`.
- Replace `ToolManifest`-based logic with `tool.Manifest()` and `ToolRequirements`.
- Replace `NewClient + Initialize` in `mcp` with `Connect`.
- Replace MCP `2025-11-25`, roots/session/GET-resume assumptions with the strict `2026-07-28` contract in [docs/migration-task34.md](docs/migration-task34.md).
- Replace all `RawData` assertions with decoding from `Chunk.Data` based on `Chunk.MimeType`.
- `exectool.WithTimeout` and `RunRequest.Timeout` are removed; pass execution deadlines on the `context` used for `Run` / `Execute` (or use `routery.Timeout` on the tool).

**Breaking changes:**

- `Chunk.Metadata` removed — use `Progress` for data-plane progress and `Control` for orchestrator signals.
- System manifest flags moved out of `Metadata`: use `ReadOnly`, `RequiresConfirmation`, `Dangerous`, `Idempotent`, `CompletionPolicy`.
- Human-in-the-loop tools yield `EventControl` + `ErrPause`.
- `EventSuspend` / `ErrSuspend` / `ServiceProvider` removed.
- `NewSession` returns `(*Session, error)` when `RunPolicy` is invalid.
- `RunContext` → `*RunEnv` on `ToolCall.Env`; `BindEnv` → `Put` / `Require` / `Lookup`.
- `ClientError` / `SystemError` → `*ToolError` with `Code` + `Retryable`.
- `ToolCall.CallContext` carries typed subject/scope; `RunEnv` string keys are only an escape hatch for DI/session state.
- `WithPolicy`, typed tool policy, and `Registry.View` are the primary policy/capability path; policy denial returns `CodePolicyDenied`, while calls outside a view return `CodeCapabilityDenied`.
- `NewTypedTool` uses `TypedToolSpec[TSubject, TScope, TArgs, TResult, TEffect]`; handlers receive `ValidatedArgs[TArgs]` and return `ToolResult[TResult, TEffect]`.
- `ToolOutcome` carries `Status`, `TypedResult`, `EmptyResult`, `Noop`, and `Effects`.
- See [docs/migration-task28.md](docs/migration-task28.md) for CallParser, `DecodeChunkAs`, and dual-namespace RunEnv.
- See [docs/migration-task31.md](docs/migration-task31.md) for typed call context, registry views, policy, effects, and streaming continuation normalization.
- See [docs/migration-task32.md](docs/migration-task32.md) for args binders, session checkpoints, snapshot slot policy, delivery envelopes, and policy-aware generic tools.

## Zero-resiliency core

The registry no longer applies default execution timeouts, concurrency limits, built-in retry middleware, or per-tool `WithTimeout` manifest deadlines. Removed APIs include `WithDefaultTimeout`, `WithMaxConcurrency`, `WithTimeoutMiddleware`, `WithIdempotentRetry`, `ToolOption` `WithTimeout`, and `ToolManifest.Timeout`. Use `context` deadlines and external execution wrappers instead; see `examples/resiliency/main.go`. Sandbox adapters honor only the `context` passed to `Run` (no separate `RunRequest` timeout field); limit `exec_code` runtime via the execution `ctx` or wrappers around the tool.

gRPC reflection helpers take an injected `grpc.ClientConnInterface` (no dial inside `toolsy`). HTTP toolkits (`httptool`, `web`, `document`) use one owned `httptool.SafeDialTransport` pool per tool set; configure timeout/TLS through `WithHTTPSettings(httptool.ClientSettings{...})`. Cleanup-returning factories release owned idle connections at disposal; ordinary factories retain bounded idle expiry. See [task41 migration](docs/migration-task41.md) for current settings and lifecycle, [docs/migration-task29.md](docs/migration-task29.md) for enterprise toolkit IoC and SSRF unification, and [docs/migration-task30.md](docs/migration-task30.md) for fail-closed read I/O (`ErrReadLimitExceeded`, transport vs display tiers).

## Contracts modules

`contracts/openapi`, `contracts/graphql`, `contracts/grpc` return `[]toolsy.Tool`.
Each adapter publishes a bounded supported subset and rejects unsupported
contracts during discovery. Hosts provide output selections, credentials,
connections and business policy. See [adapter contracts](contracts/README.md)
and [generator contract](docs/generator-contract.md).

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
