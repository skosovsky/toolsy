# toolsy

**Universal AI Tool Engine for Go** — register typed functions as tools, get JSON Schema for LLMs, validate and execute calls with a single pipeline.

[![Go Reference](https://pkg.go.dev/badge/github.com/skosovsky/toolsy.svg)](https://pkg.go.dev/github.com/skosovsky/toolsy)
[![Build Status](https://github.com/skosovsky/toolsy/workflows/Go/badge.svg)](https://github.com/skosovsky/toolsy/actions)
[![Coverage](https://img.shields.io/badge/coverage-go%20test-green)](https://github.com/skosovsky/toolsy)
Go 1.26+ · [License](LICENSE)

**TL;DR** — toolsy is a type-safe framework for creating, routing, and executing LLM tools (Function Calling) in Go. It generates JSON Schema from Go structs, validates input from the LLM, and supports middleware.

## Quick Start (AI-Friendly: GetWeather full cycle)

```go
package main

import (
    "context"
    "github.com/skosovsky/toolsy"
)

func main() {
    type Args struct {
        City string `json:"city" jsonschema:"City name"`
    }
    type Out struct {
        Temp float64 `json:"temp"`
    }
    tool, err := toolsy.NewTool("weather", "Get temperature for a city", func(ctx context.Context, a Args) (Out, error) {
        return Out{Temp: 22.5}, nil
    })
    if err != nil {
        panic(err)
    }
    reg := toolsy.NewRegistry()
    reg.Register(tool)

    // Send schema to your LLM provider (OpenAI, Anthropic, etc.) so it can call the tool
    _ = tool.Parameters() // JSON Schema map; pass to your LLM SDK (do not mutate—shallow copy)

    var out Out
    err = reg.Execute(context.Background(), toolsy.ToolCall{
        ID: "1", ToolName: "weather", Args: []byte(`{"city":"Moscow"}`),
    }, func(c toolsy.Chunk) error {
        out = c.RawData.(Out)
        return nil
    })
    if err != nil {
        panic(err)
    }
    // out.Temp == 22.5 (zero-cost: no json.Unmarshal)
}
```

## Key concepts (architecture)

- **Schema Extractor**: Go types become the tool spec. Use struct tags: `json:"field"` (name, omitempty), `jsonschema:"description"`, `description:"..."`, `enum:"a,b,c"`. `Extractor.Schema()` and `Tool.Parameters()` return a shallow copy—do not mutate. Generated schema uses [github.com/google/jsonschema-go](https://github.com/google/jsonschema-go).
- **Registry**: Holds tools; look up by name with `GetTool(name)` or list all with `GetAllTools()`. `Execute(ctx, call, yield)` runs one call; `ExecuteBatchStream(ctx, calls, yield)` runs many in parallel. Tool errors go into the stream as `Chunk{IsError: true}`; the method returns an error only for critical failures (context cancelled, shutdown). Yield calls are serialized by the library so your callback need not be thread-safe.
- **Middleware**: Signature `func(Tool) Tool`. Apply with `Registry.Use(WithLogging(...), WithTimeoutMiddleware(...))`; first in the list is the outermost. Use replaces the chain if called again.
- **Validation**: Two layers before your Go function runs: (1) JSON Schema validation, (2) optional `Validatable.Validate()` on the args struct. Validation failures become `ClientError` so the LLM can self-correct.

**Streaming**: All tools use `Execute(ctx, call, yield func(Chunk) error)`. `Chunk` has `CallID`, `ToolName`, `Event` (e.g. `EventProgress`, `EventResult`), `Data`, `RawData`, `IsError`, `Metadata`. `NewTool` calls yield once; `NewStreamTool` and `NewDynamicTool` can call it multiple times. If yield returns an error (e.g. client disconnected), execution stops and the tool returns `ErrStreamAborted`. For iteration over chunks (Go 1.23+), use `ExecuteIter(ctx, call) iter.Seq2[Chunk, error]`: `for chunk, err := range reg.ExecuteIter(ctx, call) { ... }`; on `break`, the context is cancelled and the tool exits (push-to-push, no extra goroutines).

**ExecuteIter (Go 1.23+)** — Use a `for range` over `reg.ExecuteIter(ctx, call)` to consume chunks; when you `break`, the child context is cancelled and the tool’s execution stops without leaving extra goroutines:

```go
for chunk, err := range reg.ExecuteIter(ctx, toolsy.ToolCall{ID: "1", ToolName: "my_tool", Args: args}) {
    if err != nil {
        // final error from Execute (e.g. tool error, timeout)
        return err
    }
    // use chunk.RawData or chunk.Data
}
```

**RawData and zero-cost** — For tools built with `NewTool` or `NewStreamTool`, the core does **not** call `json.Marshal`; the typed result is in `Chunk.RawData`, and `Data` is nil. **Local agent**: use a type assertion with zero CPU cost, e.g. `out := chunk.RawData.(MyStruct)`. **External client (MCP/HTTP)**: serialize at the boundary with `json.Marshal(chunk.RawData)` when sending over the wire. Use `Data` only for raw byte streams (e.g. file download, streaming text) where the tool writes bytes into `Data` and leaves `RawData` nil.

## toolsy-gen

`toolsy-gen` generates Go DTOs, handler interfaces, and `toolsy.Tool` factories from YAML or JSON tool manifests:

```bash
go run github.com/skosovsky/toolsy/cmd/toolsy-gen ./tools
```

- Input: recursive scan of `*.yaml`, `*.yml`, and `*.json`; if no path is passed, the current directory is scanned.
- Supported schema subset: root `parameters.type` must be `object`; top-level properties may be `string`, `string` with `format: date-time`, `integer`, `boolean`, or arrays of those scalar types.
- Clean-break validation: missing `description`, nested objects, arrays of objects, `$ref`, `oneOf` / `allOf` / `anyOf`, `not`, and `patternProperties` are hard errors.
- Required semantics: generated DTOs include `validate:"required"` on required fields, generated `Validate()` performs zero-dependency post-unmarshal checks where possible, and raw JSON Schema remains the source of truth through `NewProxyTool`.
- Stream semantics: generated streaming tools emit `EventProgress` for intermediate chunks and a terminal `EventResult` for the last successful item; empty successful streams emit an empty terminal result chunk.

## Error handling

**For AI agents**: Classify errors and pass the right message back to the LLM.

- **Invalid JSON from LLM** → `ClientError` (parse error). Use `IsClientError(err)` or `errors.Is(err, toolsy.ErrValidation)`; send `err.Error()` to the LLM so it can fix the payload.
- **Tool not found** → `ErrToolNotFound`. Do not send tool calls for unregistered names; or return a generic message.
- **Self-correction**: When `IsClientError(err)` is true, return `err.Error()` to the LLM so it can adjust arguments and retry. When `IsSystemError(err)` or `errors.Is(err, toolsy.ErrTimeout)` etc., do **not** expose the underlying message or stack to the LLM—log internally and return a generic user message.

Tool execution fails in two broad ways:

- **ClientError** — invalid input (bad JSON, schema validation, bad enum). Safe to return the message to the LLM for self-correction. Optionally wraps a sentinel (e.g. `ErrValidation`) and supports `Retryable` for transient cases.
- **SystemError** — internal failure (panic, timeout, DB down). Do not expose the underlying error or stack to the LLM.

**Sentinel errors** (use `errors.Is`): `ErrToolNotFound`, `ErrTimeout`, `ErrValidation`, `ErrShutdown`, `ErrStreamAborted`. Helpers: `IsClientError(err)`, `IsSystemError(err)`.

Use the provided helpers and standard library:

```go
err := reg.Execute(ctx, call, func(c toolsy.Chunk) error {
    // c.RawData for typed result (NewTool); c.Data for raw bytes or error text; c.IsError true means error in Data
    return nil
})
if err != nil {
    if toolsy.IsClientError(err) {
        // Send err.Error() to the LLM so it can fix and retry
        return sendToLLM(err.Error())
    }
    if errors.Is(err, toolsy.ErrStreamAborted) {
        // Client closed stream; not a tool logic error
    }
    if toolsy.IsSystemError(err) {
        // Log internally, return generic message to user
        log.Error("tool failed", "err", err)
        return "Something went wrong, please try again."
    }
    if errors.Is(err, toolsy.ErrValidation) {
        // Validation failed (also implies ClientError)
    }
}
```

**Self-correction loop**: LLM calls tool → gets `ClientError` with reason → adjusts arguments → calls again. Do not use `ClientError` for internal/transient errors; use `SystemError` or wrap with `ErrTimeout` etc.

## Tool Security and Validation

`toolsy` supports runtime guards without coupling to external policy engines.

- **Validator**: `WithValidator(v)` runs before unmarshaling (fail-closed). Implement `Validator` with `Validate(ctx, toolName, argsJSON string) error`. On validation failure the error goes via the **error-path**: `Execute` returns `ClientError` + `ErrValidation` with message `tool execution failed: security validation failed: <details>. Please fix the arguments and try again.` In `ExecuteBatchStream`, the same error is mapped to a chunk with `IsError: true` and `Data` set to the error message (for self-correction).
- **Loop breaker**: `WithExecutionCounter(ctx)` tracks executions; `WithMaxSteps(n)` and `WithMaxRetries(n)` (if n>0) enforce limits and return `ErrMaxStepsExceeded` or `ErrMaxRetriesExceeded` when exceeded. Counters increment on every execution when present, even if limits are 0.
- **Security metadata**: tools expose `IsReadOnly`, `RequiresConfirmation`, and `Sensitivity` via `ToolMetadata` for orchestrators.

```go
validator := myGuardyValidator{}
reg := toolsy.NewRegistry(
    toolsy.WithValidator(validator),
    toolsy.WithMaxSteps(8),
    toolsy.WithMaxRetries(2),
)

sessionCtx := toolsy.WithExecutionCounter(context.Background())
if err := reg.Execute(sessionCtx, toolsy.ToolCall{
    ID: "1", ToolName: "write_invoice", Args: []byte(`{"amount":100}`),
}, func(c toolsy.Chunk) error { return nil }); errors.Is(err, toolsy.ErrMaxStepsExceeded) {
    // stop the agent loop
}
```

```go
tool, _ := toolsy.NewTool(
    "delete_user",
    "Delete a user account",
    func(ctx context.Context, a DeleteUserArgs) (struct{}, error) { return struct{}{}, nil },
    toolsy.WithRequiresConfirmation(),
    toolsy.WithSensitivity("critical"),
)
if meta, ok := tool.(toolsy.ToolMetadata); ok {
    _ = meta.RequiresConfirmation()
    _ = meta.Sensitivity()
}
```

## Sandboxed Code Execution

For agent-written code, use the root package [`github.com/skosovsky/toolsy/exectool`](./exectool/README.md). It builds a single `exec_code` tool with a dynamic `language` enum derived from the configured sandbox.

- **LLM-facing package:** `github.com/skosovsky/toolsy/exectool`
- **Adapters:** `github.com/skosovsky/toolsy/adapters/sandbox/host`, `starlark`, `wazero`, `docker`, `e2b`
- **Timeout model:** set only in Go with `exectool.WithTimeout(...)`; it is intentionally absent from the JSON Schema shown to the LLM.
- **Starlark model:** expose only `starlark`; do not advertise `python` unless there is a real Python runtime behind it.
- **Wazero model:** expose a text language such as `jq` or `rego`, not raw `wasm`.
- **E2B model:** keep the adapter transport-agnostic and forward `RunRequest.Env` into the remote process layer.
- **Cleanup model:** container and remote adapters use bounded cleanup timeouts so timeout paths cannot hang indefinitely.

```go
sb := starlarksandbox.New()
tool, err := exectool.New(
    sb,
    exectool.WithTimeout(2*time.Second),
    exectool.WithAllowedLanguages("starlark"),
)
if err != nil {
    panic(err)
}
```

## Testing (how to test tools)

The `testutil` package provides mocks so you can unit-test tool flows without calling a real LLM.

- **testutil.MockTool** — set `NameVal`, `DescVal`, `ParamsVal`, and `ExecuteFn` to control behavior. Use in tests that need a `Tool` implementation.
- **testutil.NewTestRegistry(tools...)** — returns a `*toolsy.Registry` with a long timeout and panic recovery, with the given tools already registered.

Example:

```go
import "github.com/skosovsky/toolsy/testutil"

func TestMyHandler(t *testing.T) {
    mock := &testutil.MockTool{
        NameVal: "echo",
        ExecuteFn: func(ctx context.Context, args []byte, yield func(toolsy.Chunk) error) error {
            return yield(toolsy.Chunk{Data: args})
        },
    }
    reg := testutil.NewTestRegistry(mock)
    var result []byte
    err := reg.Execute(ctx, toolsy.ToolCall{ID: "1", ToolName: "echo", Args: []byte(`{"x":1}`)}, func(c toolsy.Chunk) error {
        result = c.Data
        return nil
    })
    require.NoError(t, err)
    assert.Equal(t, []byte(`{"x":1}`), result)
}
```

## Custom Validation (Validatable)

After JSON Schema validation, you can add cross-field or business rules by implementing `Validatable` on your args struct:

```go
type CreateOrderArgs struct {
    Quantity int    `json:"quantity" jsonschema:"Number of items"`
    Coupon   string `json:"coupon"`
}

func (a CreateOrderArgs) Validate() error {
    if a.Coupon != "" && a.Quantity > 10 {
        return &toolsy.ClientError{Reason: "coupon not valid for more than 10 items", Err: toolsy.ErrValidation}
    }
    return nil
}
```

If `Validate()` returns an error, toolsy wraps it as `ClientError` when appropriate so the LLM receives a clear message.

## Extractor (Schema + Validation without Tool)

When you need JSON Schema and two-layer validation (schema + Validatable) but not the full Tool execute pipeline—e.g. in custom orchestrators that return `*Result` with UIAction instead of `[]byte`—use `Extractor[T]`:

```go
ext, err := toolsy.NewExtractor[MyArgs](false)
if err != nil { ... }
schema := ext.Schema()   // JSON Schema for LLM provider adapters
args, err := ext.ParseAndValidate(rawJSON)  // Layer 1 + Layer 2 validation, parse into T
```

**Schema / Parameters — shallow-copy contract**
Both `Extractor.Schema()` and `Tool.Parameters()` return a **shallow copy**: only the top-level map is copied; nested maps (e.g. under `"properties"`) are shared with the internal schema. **Do not mutate** the returned map or any nested map—otherwise you will alter the tool’s schema for all future callers. Treat the value as read-only, or clone deeply (e.g. with a JSON round-trip or a deep-copy helper) if you need to modify it for a specific export. Generated schemas are produced by [github.com/google/jsonschema-go](https://github.com/google/jsonschema-go); the root schema does not rely on top-level `$ref` for LLM compatibility.

`NewTool` is built on top of `Extractor`; both share the same schema generation and validation logic.

## Proxy Tools (NewProxyTool)

When you have a raw JSON Schema as bytes (e.g. from an MCP server or external spec) and want a Tool without Go struct reflection, use `NewProxyTool`. It parses and validates the schema, then runs your handler with validated raw args and `yield func(Chunk) error`. Options like `WithStrict` and `WithTimeout` apply. Tool errors are handled like `NewDynamicTool` (ClientError pass-through, yield errors → ErrStreamAborted).

```go
rawSchema := []byte(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`)
tool, err := toolsy.NewProxyTool("search", "Search", rawSchema, func(ctx context.Context, rawArgs []byte, yield func(toolsy.Chunk) error) error {
    // rawArgs is validated; stream result
    return yield(toolsy.Chunk{Data: result})
})
```

## Dynamic Tools (Runtime API Integration)

When you have a JSON Schema at runtime (e.g. from OpenAPI/Swagger or a remote spec) and no Go struct, use `NewDynamicTool`. The schema map and handler function must be **non-nil**. It performs **Layer 1 only** (schema validation); the handler receives raw `[]byte`. Error handling matches `NewTool`: `ClientError` is passed through for self-correction, other errors are wrapped as `SystemError`. `ToolOption` (e.g. `WithTimeout`, `WithStrict`) is supported. The schema map you pass is **not mutated**: the constructor takes a **deep copy** of it before applying strict mode or stripping `$id`/`id`, so even nested objects in your map stay unchanged. Note: `Tool.Parameters()` still returns a **shallow copy** of the tool’s internal schema (see “Schema / Parameters — shallow-copy contract” above); that contract applies to the runtime Tool object, not to the input of `NewDynamicTool`.

```go
// Example: register a tool from an OpenAPI-style schema
schemaFromAPI := map[string]any{
    "type": "object",
    "properties": map[string]any{
        "endpoint": map[string]any{"type": "string", "description": "API path"},
        "method":   map[string]any{"type": "string", "enum": []any{"GET", "POST"}},
    },
    "required": []any{"endpoint", "method"},
}
tool, err := toolsy.NewDynamicTool("http_call", "Call HTTP API", schemaFromAPI,
    func(ctx context.Context, argsJSON []byte, yield func(toolsy.Chunk) error) error {
        var args struct{ Endpoint, Method string }
        if err := json.Unmarshal(argsJSON, &args); err != nil { return err }
        // ... perform request, stream result via yield(toolsy.Chunk{Data: resultJSON})
        return yield(toolsy.Chunk{Data: resultJSON})
    },
    toolsy.WithTimeout(15*time.Second),
)
if err != nil { ... }
reg.Register(tool)
```

## Streaming Responses (NewStreamTool)

For tools that produce multiple chunks (e.g. RAG search, logs), use `NewStreamTool`. Same schema/validation as `NewTool`, but the handler receives `yield` and may call it zero or more times. If `yield` returns an error (e.g. client closed connection), the error is returned as `ErrStreamAborted`.

```go
tool, err := toolsy.NewStreamTool("search", "Search docs", func(ctx context.Context, q QueryArgs, yield func(toolsy.Chunk) error) error {
    for _, doc := range search(q.Query) {
        if err := yield(toolsy.Chunk{Data: mustMarshal(doc)}); err != nil { return err }
    }
    return nil
})
```

## Custom Type Mappings

By default, custom Go types (e.g. `uuid.UUID`, or your own `MyMoney` type) are reflected as objects or may not match what the LLM expects. Use `RegisterType` to map such types to a JSON Schema `type` and optional `format` so the generated schema is correct for those fields.

Call `RegisterType` at application startup, **before** the first `NewTool` or `NewExtractor`:

```go
import "github.com/google/uuid"

func init() {
    toolsy.RegisterType(uuid.UUID{}, "string", "uuid")
}
```

Pointer fields are supported automatically: if you register `MyMoney{}`, then a field `*MyMoney` will use the same mapping (no need to register `*MyMoney` separately).

Example for a custom “money” type:

```go
type MyMoney struct{}

func init() {
    toolsy.RegisterType(MyMoney{}, "number", "decimal")
}
```

After registration, any struct that has a `MyMoney` or `*MyMoney` field will get `type: number`, `format: decimal` in the schema for that field.

## Strict Mode

For OpenAI Structured Outputs (or when you want to reject extra properties), use `WithStrict()` when creating a tool. Strict mode sets `additionalProperties: false` for all objects in the generated schema and makes all properties required.

```go
tool, err := toolsy.NewTool("strict_tool", "Desc", fn, toolsy.WithStrict())
```

## Middleware

Middleware adds cross-cutting behavior (logging, panic recovery, per-tool timeout) to every tool execution. Apply globally with `Registry.Use()`:

```go
reg := toolsy.NewRegistry(
    toolsy.WithDefaultTimeout(10*time.Second),
    toolsy.WithRecoverPanics(true),
)
reg.Use(toolsy.WithRecovery(), toolsy.WithLogging(slog.Default()))
reg.Register(myTool)
```

- **WithLogging(logger)** — logs tool start, end, duration, and errors.
- **WithRecovery()** — recovers panics and returns a `SystemError` instead of crashing.
- **WithTimeoutMiddleware(d)** — shortens the effective timeout for the wrapped tool (minimum with registry default).

Order in `Use(...)` matters: the first middleware is the outermost (runs first). Calling `Use` multiple times replaces the chain; pass all middlewares in one call.

**Timeout hierarchy**: The effective timeout for a call is the **minimum** of: (1) registry default from `WithDefaultTimeout(d)`, (2) per-tool timeout from `WithTimeout(d)` (ToolOption), (3) middleware timeout from `WithTimeoutMiddleware(d)` if applied. The registry default is an upper bound; per-tool and middleware can only shorten it, never extend it.

## Graceful Shutdown

Call `Shutdown(ctx)` to stop accepting new executions and wait for in-flight calls to finish (or for `ctx` to cancel):

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
if err := reg.Shutdown(ctx); err != nil {
    log.Printf("shutdown: %v", err)
}
// After Shutdown returns, Execute/ExecuteBatchStream return ErrShutdown
```

## Advanced Usage

### Event-Driven Async Tools

For long-running tasks (reports, batch jobs), use `AsAsyncTool` so the LLM gets an immediate `task_id` and the work runs in a goroutine. When the task completes, `WithOnComplete` is called with all collected chunks and the final error. If the client's yield returns an error (e.g. stream closed), the goroutine is not started (yield-guard), and `Execute` returns that error wrapped as `ErrStreamAborted`. If the incoming context is already cancelled before yielding `accepted`, `Execute` returns the context error and does not start the background job. Background execution is protected by a local panic/timeout envelope: panics in the base tool are recovered and reported to `OnComplete` as a `SystemError`. When run via `Registry`, the registry's effective timeout (from `WithDefaultTimeout` or per-tool `ToolMetadata.Timeout`) applies to the whole call: **queue wait** (acquire semaphore) and **sync phase** are under that timeout, and the **async background run** uses the same effective timeout so long-running background work is bounded. If the base tool implements `ToolMetadata` with `Timeout() > 0`, it is still applied in the background when not run via Registry. Registry middlewares (e.g. `WithLogging`, `WithRecovery`) and global logging apply only to the synchronous "accepted" phase; the background phase does not go through the same middleware chain. When tools are executed via `Registry`, async background jobs are **tracked**: the execution slot (semaphore) is held until the background job finishes, and `Registry.Shutdown` waits for both in-flight synchronous executions and accepted async background jobs before returning.

```go
// Example: Sending task completion to an Event Bus
asyncTool := toolsy.AsAsyncTool(heavyReportTool, toolsy.WithOnComplete(
    func(ctx context.Context, taskID string, chunks []toolsy.Chunk, err error) {
        kafkaProducer.Publish("tool_events", map[string]any{
            "task_id": taskID,
            "status":  "completed",
            "error":   err,
        })
    },
))
reg.Register(asyncTool)
```

The first chunk the client receives has `RawData` of type `toolsy.AsyncAccepted` with `status: "accepted"` and `task_id` (hex string). `OnComplete` receives all chunks collected from the base tool's stream; for high-volume or long-running streaming tools, this buffers output in memory until the task finishes.

### Role-Based Registries (Dynamic Prompts)

Use `OverrideTool` to reuse the same tool logic with different names or descriptions for different agent roles, without mutating the original tool. When you override the name with `WithNewName`, runtime chunks emitted during `Execute` have `Chunk.ToolName` set to that name (alias consistency). Override schema passed to `WithNewParameters(map[string]any)` is stored as a defensive deep copy so later mutations of the caller's map do not affect the wrapper; `Parameters()` returns a shallow copy of the stored schema.

```go
// Reusing the same execution logic but changing the LLM instructions
seniorDBATool := toolsy.OverrideTool(sqlTool,
    toolsy.WithNewDescription("Execute complex JOINs. Only use if strictly necessary."),
)
juniorTool := toolsy.OverrideTool(sqlTool,
    toolsy.WithNewDescription("Fetch data. NEVER use DROP or DELETE."),
)
reg.Register(seniorDBATool)
// or reg.Register(juniorTool) depending on the active role
```

## API Overview

| Symbol                                                                                                                                                        | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [Tool](https://pkg.go.dev/github.com/skosovsky/toolsy#Tool)                                                                                                   | Interface: Name, Description, Parameters (schema), Execute(ctx, argsJSON, yield)                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| [ToolMetadata](https://pkg.go.dev/github.com/skosovsky/toolsy#ToolMetadata)                                                                                   | Optional: Timeout, Tags, Version, IsDangerous (for tools from NewTool or NewDynamicTool)                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| [ToolCall](https://pkg.go.dev/github.com/skosovsky/toolsy#ToolCall) / [Chunk](https://pkg.go.dev/github.com/skosovsky/toolsy#Chunk)                           | Request; Chunk is stream event (CallID, ToolName, Event, Data, IsError, Metadata)                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| [NewTool](https://pkg.go.dev/github.com/skosovsky/toolsy#NewTool)                                                                                             | Build a Tool from a typed function `func(ctx, T) (R, error)`; calls yield(Chunk) once                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| [NewStreamTool](https://pkg.go.dev/github.com/skosovsky/toolsy#NewStreamTool)                                                                                 | Build a Tool with streaming handler `func(ctx, T, yield func(Chunk) error) error`                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| [NewDynamicTool](https://pkg.go.dev/github.com/skosovsky/toolsy#NewDynamicTool)                                                                               | Build a Tool from a raw JSON Schema map; handler gets argsJSON and yield func(Chunk) error                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| [NewProxyTool](https://pkg.go.dev/github.com/skosovsky/toolsy#NewProxyTool)                                                                                   | Build a Tool from raw JSON Schema bytes (e.g. MCP); handler gets rawArgs and yield func(Chunk) error                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| [Extractor](https://pkg.go.dev/github.com/skosovsky/toolsy#Extractor) / [NewExtractor](https://pkg.go.dev/github.com/skosovsky/toolsy#NewExtractor)           | Schema + validation only (no Execute); use in custom orchestrators                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| [NewRegistry](https://pkg.go.dev/github.com/skosovsky/toolsy#NewRegistry)                                                                                     | Create a registry; [Execute](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.Execute)(ctx, call, yield), [ExecuteBatchStream](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.ExecuteBatchStream)(ctx, calls, yield), [GetTool](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.GetTool), [GetAllTools](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.GetAllTools), [Use](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.Use), [Shutdown](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.Shutdown) |
| Registry options                                                                                                                                              | WithDefaultTimeout, WithMaxConcurrency, WithRecoverPanics; WithOnBeforeExecute, WithOnAfterExecute (receives [ExecutionSummary](https://pkg.go.dev/github.com/skosovsky/toolsy#ExecutionSummary)), WithOnChunk                                                                                                                                                                                                                                                                                                                                         |
| ToolOption                                                                                                                                                    | WithStrict, WithTimeout, WithTags, WithVersion, WithDangerous (metadata for tools from NewTool/NewDynamicTool)                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| [Validatable](https://pkg.go.dev/github.com/skosovsky/toolsy#Validatable)                                                                                     | Optional Layer 2 validation: implement `Validate() error` on your args struct                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| [Middleware](https://pkg.go.dev/github.com/skosovsky/toolsy#Middleware)                                                                                       | WithLogging, WithRecovery, WithTimeoutMiddleware; [Registry.Use](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.Use) to apply                                                                                                                                                                                                                                                                                                                                                                                                                 |
| [AsAsyncTool](https://pkg.go.dev/github.com/skosovsky/toolsy#AsAsyncTool) / [OverrideTool](https://pkg.go.dev/github.com/skosovsky/toolsy#OverrideTool)       | Async: fire-and-forget with task_id and OnComplete; Override: replace Name, Description, or Parameters for role-based prompts                                                                                                                                                                                                                                                                                                                                                                                                                          |
| [IsClientError](https://pkg.go.dev/github.com/skosovsky/toolsy#IsClientError) / [IsSystemError](https://pkg.go.dev/github.com/skosovsky/toolsy#IsSystemError) | Classify errors; [ErrStreamAborted](https://pkg.go.dev/github.com/skosovsky/toolsy#ErrStreamAborted) when yield fails                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| [RegisterType](https://pkg.go.dev/github.com/skosovsky/toolsy#RegisterType)                                                                                   | Register a custom type → JSON Schema type/format; call at startup before first NewTool/NewExtractor                                                                                                                                                                                                                                                                                                                                                                                                                                                    |

## Contracts (OpenAPI, GraphQL, gRPC)

The `contracts/` directory contains three **isolated** submodules that translate external API contracts into `toolsy.Tool` instances. Each has its own `go.mod`; use them when you have an OpenAPI spec, a GraphQL endpoint, or a gRPC server with reflection.

- **contracts/openapi** — `ParseURL(ctx, specURL, opts)` loads an OpenAPI 3.x spec from a URL, filters by methods/tags, and returns one tool per operation. Options: `BaseURL`, `AuthHeader`, `AllowedMethods`, `AllowedTags`, `MaxResponseBytes`.
- **contracts/graphql** — `Introspect(ctx, endpoint, opts)` sends the standard introspection query, then builds one tool per root Query/Mutation. Request body is always `{"query": "<static>", "variables": <args>}` (no string concatenation from user input). Options: `AuthHeader`, `Operations` (e.g. `["query"]` for read-only), `MaxResponseBytes`.
- **contracts/grpc** — `ConnectAndReflect(ctx, target, opts)` dials the server, uses gRPC Server Reflection to discover services/methods, and returns one tool per RPC. Options: `DialOptions`, `Services` (allowlist), `MaxResponseBytes`.

Register the returned tools with your registry in a loop (one tool per call to `Register`):

```go
import (
    "context"
    "github.com/skosovsky/toolsy"
    openapi "github.com/skosovsky/toolsy/contracts/openapi"
    "github.com/skosovsky/toolsy/contracts/graphql"
    "github.com/skosovsky/toolsy/contracts/grpc"
)

func main() {
    reg := toolsy.NewRegistry()
    ctx := context.Background()

    // OpenAPI: tools from a spec URL
    openapiTools, err := openapi.ParseURL(ctx, "https://api.example.com/openapi.json", openapi.Options{
        BaseURL: "https://api.example.com", AuthHeader: "Bearer sk-...",
        AllowedMethods: []string{"GET", "POST"}, MaxResponseBytes: 512 * 1024,
    })
    if err != nil { panic(err) }
    for _, t := range openapiTools { reg.Register(t) }

    // GraphQL: tools from introspection
    gqlTools, err := graphql.Introspect(ctx, "https://api.example.com/graphql", graphql.Options{
        AuthHeader: "Bearer sk-...", Operations: []string{"query", "mutation"},
    })
    if err != nil { panic(err) }
    for _, t := range gqlTools { reg.Register(t) }

    // gRPC: tools from server reflection
    grpcTools, err := grpc.ConnectAndReflect(ctx, "localhost:50051", grpc.Options{})
    if err != nil { panic(err) }
    for _, t := range grpcTools { reg.Register(t) }
}
```

See [contracts/README.md](contracts/README.md) for more details and module-specific options.

## Installation

```bash
go get github.com/skosovsky/toolsy
```

## Dependencies

**Runtime** (required when using the library):

- **github.com/google/jsonschema-go** — JSON Schema inference from Go types and validation (single engine)

**Development only** (tests and examples):

- **github.com/stretchr/testify** — assert/require in tests
- **go.uber.org/goleak** — goroutine leak detection in tests

Minimum Go version: **1.26**.

## License

See [LICENSE](LICENSE).
