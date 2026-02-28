# toolsy

**Universal AI Tool Engine for Go** — register typed functions as tools, get JSON Schema for LLMs, validate and execute calls with a single pipeline.

[![Go Reference](https://pkg.go.dev/badge/github.com/skosovsky/toolsy.svg)](https://pkg.go.dev/github.com/skosovsky/toolsy)  
Go 1.26+ · [License](LICENSE)

## Quick Start

```go
package main

import (
    "context"
    "encoding/json"
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

    var result []byte
    err = reg.Execute(context.Background(), toolsy.ToolCall{
        ID: "1", ToolName: "weather", Args: []byte(`{"city":"Moscow"}`),
    }, func(chunk []byte) error {
        result = chunk
        return nil
    })
    if err != nil {
        panic(err)
    }
    var out Out
    if err := json.Unmarshal(result, &out); err != nil {
        panic(err)
    }
    // out.Temp == 22.5
}
```

## Concept

- **Streaming**: All tools stream results via a `yield` callback. `NewTool` calls yield once with the marshalled result; `NewStreamTool` and `NewDynamicTool` can call it multiple times. If yield returns an error (e.g. client disconnected), the tool receives `ErrStreamAborted`.
- **Single Source of Truth**: One set of struct tags (`json:"field"` for names and omitempty; `jsonschema:"description text"` — the tag value becomes the schema `description` for the property) drives both the schema you send to the LLM and the validation of incoming JSON. No duplicate schemas.
- **Partial Success**: `ExecuteBatchStream` runs multiple tool calls in parallel; each chunk is tagged with `Chunk{CallID, ToolName, Data}`. The library serializes yield calls so your callback need not be thread-safe.
- **Self-Correction**: `ClientError` returns human-readable validation messages (e.g. "field 'city' is required") so the LLM can fix and retry.

## Error Handling

Tool execution can fail in two ways:

- **ClientError** — invalid input (bad JSON, schema validation, bad enum). Safe to return the message to the LLM for self-correction. Optionally wraps a sentinel (e.g. `ErrValidation`) and supports `Retryable` for transient cases.
- **SystemError** — internal failure (panic, timeout, DB down). Do not expose the underlying error or stack to the LLM.

Use the provided helpers and standard library:

```go
err := reg.Execute(ctx, call, func(chunk []byte) error {
    // handle chunk (e.g. send to client)
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
    func(ctx context.Context, argsJSON []byte, yield func([]byte) error) error {
        var args struct{ Endpoint, Method string }
        if err := json.Unmarshal(argsJSON, &args); err != nil { return err }
        // ... perform request, stream result via yield(resultJSON)
        return yield(resultJSON)
    },
    toolsy.WithTimeout(15*time.Second),
)
if err != nil { ... }
reg.Register(tool)
```

## Streaming Responses (NewStreamTool)

For tools that produce multiple chunks (e.g. RAG search, logs), use `NewStreamTool`. Same schema/validation as `NewTool`, but the handler receives `yield` and may call it zero or more times. If `yield` returns an error (e.g. client closed connection), the error is returned as `ErrStreamAborted`.

```go
tool, err := toolsy.NewStreamTool("search", "Search docs", func(ctx context.Context, q QueryArgs, yield func([]byte) error) error {
    for _, doc := range search(q.Query) {
        if err := yield(mustMarshal(doc)); err != nil { return err }
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
- **WithTimeoutMiddleware(d)** — overrides the registry default timeout for the wrapped tool.

Order in `Use(...)` matters: the first middleware is the outermost (runs first). Calling `Use` multiple times replaces the chain; pass all middlewares in one call.

**Timeout hierarchy**: The effective timeout for a call is the minimum of: (1) registry default from `WithDefaultTimeout(d)`, (2) per-tool timeout from `WithTimeout(d)` (ToolOption), (3) middleware timeout from `WithTimeoutMiddleware(d)` if applied. The innermost timeout wins.

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

## API Overview

| Symbol | Description |
|--------|-------------|
| [Tool](https://pkg.go.dev/github.com/skosovsky/toolsy#Tool) | Interface: Name, Description, Parameters (schema), Execute(ctx, argsJSON, yield) |
| [ToolMetadata](https://pkg.go.dev/github.com/skosovsky/toolsy#ToolMetadata) | Optional: Timeout, Tags, Version, IsDangerous (for tools from NewTool or NewDynamicTool) |
| [ToolCall](https://pkg.go.dev/github.com/skosovsky/toolsy#ToolCall) / [Chunk](https://pkg.go.dev/github.com/skosovsky/toolsy#Chunk) | Request; Chunk is stream event (CallID, ToolName, Data) for batch |
| [NewTool](https://pkg.go.dev/github.com/skosovsky/toolsy#NewTool) | Build a Tool from a typed function `func(ctx, T) (R, error)`; calls yield once |
| [NewStreamTool](https://pkg.go.dev/github.com/skosovsky/toolsy#NewStreamTool) | Build a Tool with streaming handler `func(ctx, T, yield) error` |
| [NewDynamicTool](https://pkg.go.dev/github.com/skosovsky/toolsy#NewDynamicTool) | Build a Tool from a raw JSON Schema map; handler gets argsJSON and yield |
| [Extractor](https://pkg.go.dev/github.com/skosovsky/toolsy#Extractor) / [NewExtractor](https://pkg.go.dev/github.com/skosovsky/toolsy#NewExtractor) | Schema + validation only (no Execute); use in custom orchestrators |
| [NewRegistry](https://pkg.go.dev/github.com/skosovsky/toolsy#NewRegistry) | Create a registry; [Execute](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.Execute)(ctx, call, yield), [ExecuteBatchStream](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.ExecuteBatchStream)(ctx, calls, yield) |
| [Validatable](https://pkg.go.dev/github.com/skosovsky/toolsy#Validatable) | Optional Layer 2 validation: implement `Validate() error` on your args struct |
| [Middleware](https://pkg.go.dev/github.com/skosovsky/toolsy#Middleware) | WithLogging, WithRecovery, WithTimeoutMiddleware; [Registry.Use](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.Use) to apply |
| [IsClientError](https://pkg.go.dev/github.com/skosovsky/toolsy#IsClientError) / [IsSystemError](https://pkg.go.dev/github.com/skosovsky/toolsy#IsSystemError) | Classify errors; [ErrStreamAborted](https://pkg.go.dev/github.com/skosovsky/toolsy#ErrStreamAborted) when yield fails |
| [RegisterType](https://pkg.go.dev/github.com/skosovsky/toolsy#RegisterType) | Register a custom type → JSON Schema type/format; call at startup before first NewTool/NewExtractor |

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
