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
        City string `json:"city" jsonschema:"required,description=City name"`
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

    result := reg.Execute(context.Background(), toolsy.ToolCall{
        ID: "1", ToolName: "weather", Args: []byte(`{"city":"Moscow"}`),
    })
    if result.Error != nil {
        panic(result.Error)
    }
    var out Out
    if err := json.Unmarshal(result.Result, &out); err != nil {
        panic(err)
    }
    // out.Temp == 22.5
}
```

## Concept

- **Single Source of Truth**: One set of struct tags (e.g. `jsonschema:"required"`, `json:"field"`) drives both the schema you send to the LLM and the validation of incoming JSON. No duplicate schemas.
- **Partial Success**: `ExecuteBatch` runs multiple tool calls in parallel; each result is independent. One failure does not cancel others.
- **Self-Correction**: `ClientError` returns human-readable validation messages (e.g. "field 'city' is required") so the LLM can fix and retry.

## Error Handling

Tool execution can fail in two ways:

- **ClientError** — invalid input (bad JSON, schema validation, bad enum). Safe to return the message to the LLM for self-correction. Optionally wraps a sentinel (e.g. `ErrValidation`) and supports `Retryable` for transient cases.
- **SystemError** — internal failure (panic, timeout, DB down). Do not expose the underlying error or stack to the LLM.

Use the provided helpers and standard library:

```go
result := reg.Execute(ctx, call)
if result.Error != nil {
    if toolsy.IsClientError(result.Error) {
        // Send result.Error.Error() to the LLM so it can fix and retry
        return sendToLLM(result.Error.Error())
    }
    if toolsy.IsSystemError(result.Error) {
        // Log internally, return generic message to user
        log.Error("tool failed", "err", result.Error)
        return "Something went wrong, please try again."
    }
    if errors.Is(result.Error, toolsy.ErrValidation) {
        // Validation failed (also implies ClientError)
    }
}
```

**Self-correction loop**: LLM calls tool → gets `ClientError` with reason → adjusts arguments → calls again. Do not use `ClientError` for internal/transient errors; use `SystemError` or wrap with `ErrTimeout` etc.

## Custom Validation (Validatable)

After JSON Schema validation, you can add cross-field or business rules by implementing `Validatable` on your args struct:

```go
type CreateOrderArgs struct {
    Quantity int    `json:"quantity" jsonschema:"minimum=1"`
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
// After Shutdown returns, Execute/ExecuteBatch return ErrShutdown
```

## API Overview

| Symbol | Description |
|--------|-------------|
| [Tool](https://pkg.go.dev/github.com/skosovsky/toolsy#Tool) | Interface: Name, Description, Parameters (schema), Execute |
| [ToolMetadata](https://pkg.go.dev/github.com/skosovsky/toolsy#ToolMetadata) | Optional: Timeout, Tags, Version, IsDangerous (for tools created with NewTool) |
| [ToolCall](https://pkg.go.dev/github.com/skosovsky/toolsy#ToolCall) / [ToolResult](https://pkg.go.dev/github.com/skosovsky/toolsy#ToolResult) | Request/response for one call |
| [NewTool](https://pkg.go.dev/github.com/skosovsky/toolsy#NewTool) | Build a Tool from a typed function `func(ctx, T) (R, error)` |
| [NewRegistry](https://pkg.go.dev/github.com/skosovsky/toolsy#NewRegistry) | Create a registry; use [Register](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.Register), [GetTool](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.GetTool) / [GetAllTools](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.GetAllTools), and [Execute](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.Execute) / [ExecuteBatch](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.ExecuteBatch) |
| [Validatable](https://pkg.go.dev/github.com/skosovsky/toolsy#Validatable) | Optional Layer 2 validation: implement `Validate() error` on your args struct |
| [Middleware](https://pkg.go.dev/github.com/skosovsky/toolsy#Middleware) | WithLogging, WithRecovery, WithTimeoutMiddleware; [Registry.Use](https://pkg.go.dev/github.com/skosovsky/toolsy#Registry.Use) to apply |
| [IsClientError](https://pkg.go.dev/github.com/skosovsky/toolsy#IsClientError) / [IsSystemError](https://pkg.go.dev/github.com/skosovsky/toolsy#IsSystemError) | Classify errors for LLM vs internal handling |

## Installation

```bash
go get github.com/skosovsky/toolsy
```

## Dependencies

**Runtime** (required when using the library):

- **github.com/invopop/jsonschema** — generate JSON Schema from Go structs
- **github.com/santhosh-tekuri/jsonschema/v6** — validate JSON against schema

**Development only** (tests and examples):

- **github.com/stretchr/testify** — assert/require in tests
- **go.uber.org/goleak** — goroutine leak detection in tests

Minimum Go version: **1.26**.

## License

See [LICENSE](LICENSE).
