# MCP (Model Context Protocol) integration for toolsy

This module bridges MCP servers to [toolsy](https://github.com/skosovsky/toolsy)'s `Tool` and `Registry` interface. It provides a transport layer (stdio and SSE) and maps MCP tools, resources, and prompts into types the orchestrator can use.

## Features

- **Transports**: `StdioTransport` (child process via stdin/stdout) and `SSETransport` (HTTP Server-Sent Events with dynamic POST endpoint).
- **Client**: `Initialize` handshake (sends `notifications/initialized` per MCP spec after success), `GetTools` (iterator with cursor pagination), `GetResourceTool` (single tool for `resources/read`), `GetPrompts` / `GetPrompt`. `GetPrompt` returns `*PromptMessageResult` (description and list of prompt messages; compatible with MCP `prompts/get` result).
- **Thread-safe**: Safe for concurrent use (e.g. `Registry.ExecuteBatchStream`). Request IDs are generated with `atomic.Uint64`; pending responses are correlated via `sync.Map`.
- **Resilience**: Context cancellation and yield errors trigger `notifications/cancelled` and return `toolsy.ErrStreamAborted`. Process crash (stdio) unblocks all pending `Call`s with an error.

## Example

```go
package main

import (
	"context"
	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/mcp"
)

func main() {
	ctx := context.Background()
	reg := toolsy.NewRegistry()

	// 1. Initialize transport (e.g. Postgres MCP)
	transport := mcp.NewStdioTransport("npx", []string{"-y", "@modelcontextprotocol/server-postgres", "postgres://localhost/db"})

	// 2. Create client with Roots (local folders the agent allows the MCP server to access)
	client := mcp.NewClient(transport, mcp.WithClientRoots([]string{"/my/workspace"}))
	defer client.Close()

	if err := client.Initialize(ctx); err != nil {
		panic(err)
	}

	// 3. Register all tools from the server into the toolsy registry
	for tool, err := range client.GetTools(ctx) {
		if err != nil {
			panic(err)
		}
		reg.Register(tool)
	}

	// 4. Add the system tool for reading resources from this server
	resTool, _ := client.GetResourceTool()
	reg.Register(resTool)

	// reg is ready for the LLM: it will generate JSON Schema, handle validation, streaming, progress, and timeouts.
}
```

## Transport interface

The `Transport` interface provides `Call(ctx, method, params) (result []byte, requestID string, err error)`. The **requestID** is the JSON-RPC request id used for the call; it is returned so the client can send `notifications/cancelled` with that id when the request is aborted (e.g. context cancellation or yield error). Implementations are thread-safe.

## Stdio transport

- `NewStdioTransport(executable string, args []string, opts ...StdioTransportOption)` — executable, **args as a slice** (convenient for programmatic command building, e.g. conditionally appending `--debug` or `--path`), and optional options. The spec document may show a variadic example; the implementation uses a slice and options (WithLogger, WithStdioFirstLineTimeout) for consistency and programmatic use.
- Options: `mcp.WithLogger(logger *slog.Logger)` (stderr forwarded to logger; default `slog.Default()`); `mcp.WithStdioFirstLineTimeout(d time.Duration)` (max wait for first stdout line after start; default 30s).

## SSE transport

- `NewSSETransport(initialURL string)` — only the initial URL (e.g. `http://localhost:3001/sse`) is fixed. The server must send an event with type `endpoint` first; the `data` field is the URL used for POST (Call/Notify). All JSON-RPC requests are sent to that URL.

## Content formatting

Tool and resource results are converted to LLM-friendly text via **`mcp.FormatContent`**: text parts are concatenated, images are embedded as Markdown `![image](data:<mediaType>;base64,...)`. When the server sets `isError: true`, the chunk is prefixed with "Tool error: " and `Chunk.IsError` is set. You can override formatting by providing your own logic and calling `FormatContentItems` or parsing results yourself.

## Requirements

- Go 1.26+
- Only standard library and `github.com/skosovsky/toolsy` (no heavy third-party MCP SDK).
