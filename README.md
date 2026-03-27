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
		func(_ context.Context, _ toolsy.RunContext, a Args) (Out, error) {
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
		return json.Unmarshal(c.Data, &out)
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(out.Temp)
}
```

## v2 API contracts

- `Tool` interface: `Manifest() ToolManifest` and `Execute(ctx, run, input, yield)`.
- `ToolCall` carries `Input toolsy.ToolInput`; old `ToolCall.Args` is removed.
- `ToolInput` contains `CallID`, `ArgsJSON`, and optional `Attachments`.
- `Chunk` is MIME-aware payload envelope: `Event`, `Data`, `MimeType`, `IsError`, `Metadata`.
- `Chunk.Event` is strongly typed: `EventProgress`, `EventResult`, `EventSuspend`.
- `Chunk.RawData` is removed.
- Runtime `Registry` is immutable. Use `RegistryBuilder` to add tools and middleware before `Build()`.
- Runtime-aware handlers are the only builders in v2 (`NewTool`, `NewStreamTool`, `NewDynamicTool`, `NewProxyTool`).

## Registry setup

```go
reg, err := toolsy.NewRegistryBuilder(
	toolsy.WithDefaultTimeout(5*time.Second),
	toolsy.WithMaxConcurrency(8),
).Use(
	toolsy.WithRecovery(),
	toolsy.WithLogging(slog.Default()),
).Add(
	toolA, toolB,
).Build()
```

The built registry is read-only for runtime calls (`Execute`, `ExecuteIter`, `ExecuteBatchStream`).

## Tool manifest and metadata

`ToolManifest` contains:

- `Name`, `Description`, `Parameters`
- `Timeout`, `Tags`, `Version`
- `Metadata map[string]any`

Use metadata keys for orchestrator policy:

- `dangerous` (from `WithDangerous`)
- `read_only` (from `WithReadOnly`)
- `requires_confirmation` (set via `WithMetadata`)
- `sensitivity` (set via `WithMetadata`)

Example:

```go
tool, err := toolsy.NewTool(
	"delete_user",
	"Delete a user account",
	handler,
	toolsy.WithDangerous(),
	toolsy.WithMetadata(map[string]any{
		"requires_confirmation": true,
		"sensitivity":           "critical",
	}),
)
if err != nil {
	return err
}
meta := tool.Manifest().Metadata
_ = meta
```

## RunContext dependencies

`RunContext` carries runtime-only dependencies:

- `Credentials` (`CredentialsProvider`)
- `State` (`StateStore`)
- `Services` (`ServiceProvider`)

`ToolInput.Attachments` are exposed to handlers as `run.Attachments()`.

`ToolInput.CallID` is the orchestrator/LLM tool call identifier used for metadata tagging in `Registry`/`Session` execution paths and observability middleware.
Direct low-level `Tool.Execute(...)` does not auto-fill `Chunk.CallID`.

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
	run toolsy.RunContext,
	input toolsy.ToolInput,
	yield func(toolsy.Chunk) error,
) error {
	if !t.allow(ctx) {
		return ErrRateLimit
	}
	return t.next.Execute(ctx, run, input, yield)
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
- `Registry.ExecuteBatchStream(...)` converts non-suspend execution failures (including pre-tool failures like missing tool, validator rejection, and shutdown, plus tool/middleware failures) to `Chunk{IsError: true, MimeType: MimeTypeText}`, while `ErrStreamAborted` and context cancellation are returned as errors.

Recommended stack for enterprise policies (outer -> inner):

```go
reg, err := toolsy.NewRegistryBuilder().
	Use(
		toolsy.WithTruncation(8000),
		toolsy.WithErrorFormatter(),
		toolsy.WithIdempotentRetry(),
		toolsy.WithBudget(),
	).
	Add(tools...).
	Build()
```

Notes:

- `WithTruncation` truncates `text/plain` and `text/markdown` by default; `application/json` truncation is opt-in via `WithTruncationIncludeJSON(true)`.
- `WithBudget` is inside `WithIdempotentRetry`, so budget checks run before every physical retry attempt.
- `WithIdempotentRetry` retries only tools with `Metadata["read_only"] == true` and stops retrying after the first successfully delivered chunk.
- `WithErrorFormatter` may convert terminal errors into `Chunk{IsError: true}` and then return `nil` (soft error).
- `WithErrorFormatter` handles only errors from wrapped tool/middleware execution; pre-tool failures (e.g. `ErrToolNotFound`, `ErrMaxStepsExceeded`, shutdown/validator failures) remain hard errors.
- If you need to classify step success/failure in an orchestrator using `SessionTrack`, use `Chunk.IsError` as the failure signal; `SessionTrack` counts executions, not outcome status.

## ServiceProvider recipe

```go
if run.Services == nil {
	return Result{}, fmt.Errorf("service provider is not configured")
}
dbAny, ok := run.Services.Get("database")
if !ok {
	return Result{}, fmt.Errorf("database service not found")
}
db, ok := dbAny.(*sql.DB)
if !ok {
	return Result{}, fmt.Errorf("database service has unexpected type")
}
_ = db
```

## Streaming and iteration

- `Execute(ctx, call, yield)` for callback streaming.
- `ExecuteIter(ctx, call)` for Go 1.23+ `for range` iteration over `(Chunk, error)`.
- `ExecuteBatchStream(ctx, calls, yield)` runs calls in parallel and serializes yield delivery.

Yield errors are converted to `ErrStreamAborted`.

## Async tools

Use `AsAsyncTool(base, WithOnComplete(...))` for fire-and-forget execution with immediate accepted result (`AsyncAccepted` JSON payload in first result chunk).

When async tool is executed via `Registry`, background jobs are tracked by registry shutdown and concurrency controls.

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

## Migration notes (v1 -> v2)

- Replace `ToolCall.Args` with `ToolCall.Input.ArgsJSON`.
- Replace `ToolCall.ID` with `ToolCall.Input.CallID`.
- Replace runtime `reg.Register(...)` / `reg.Use(...)` with `RegistryBuilder`.
- Replace `ToolMetadata`-based logic with `tool.Manifest()`.
- Replace `NewClient + Initialize` in `mcp` with `Connect`.
- Replace all `RawData` assertions with decoding from `Chunk.Data` based on `Chunk.MimeType`.

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
