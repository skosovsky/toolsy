# MCP 2025-11-25 client for toolsy

`github.com/skosovsky/toolsy/mcp` is a strict MCP client and protocol bridge for `toolsy`.

The module supports exactly protocol revision `2025-11-25`. Older revisions, legacy HTTP+SSE endpoint discovery, compatibility aliases and automatic fallbacks are intentionally absent.

## Supported surface

- stdio and single-endpoint Streamable HTTP;
- bidirectional JSON-RPC requests, responses and notifications;
- strict lifecycle/version/capability negotiation;
- roots via `roots/list` using canonical `file://` URIs;
- tools with annotations, `outputSchema`, `structuredContent`, icons and task metadata;
- text, image, audio, resource-link and embedded-resource content blocks;
- text and binary resource contents;
- prompts with current tagged content blocks;
- fractional progress through `_meta.progressToken`;
- typed `RequestMeta` with lossless non-reserved extension fields on every request DTO;
- lossless non-reserved extension fields on every MCP result DTO through `Extra`;
- cancellation with the exact active JSON-RPC request ID, emitted only after the request reaches the wire and wins the atomic terminal race;
- tools/resources/prompts invalidation and URI-normalized subscribed resource updates, including sub-resources;
- structured logging notifications whose required `data` may contain any JSON value, including `null`;
- bounded reads and SSRF-safe Streamable HTTP defaults.
- schema-equivalent validation on both decode and encode for every exported wire DTO, including required fields, tagged unions and nested metadata.
- exact envelope/extension collision checks plus enum, URI, tool-name, full JSON Schema 2020-12 compilation and base64 validation; required strings remain empty-capable unless the schema defines a stronger constraint.
- icon sources restricted to HTTPS URLs with a hostname or RFC 2397 image data URIs, plus finite monotonic progress without an invented non-negative constraint.

Tasks, sampling, elicitation, MCP Apps, full OAuth orchestration and MCP server implementation are outside this module. They are not advertised as capabilities.

## Stdio

```go
transport := mcp.NewStdioTransport(
    "npx",
    []string{"-y", "@modelcontextprotocol/server-postgres", databaseURL},
    mcp.WithStdioMaxStreamBytes(16<<20),
)

client, err := mcp.Connect(
    ctx,
    transport,
    mcp.WithClientRoots([]string{workspace}),
    mcp.WithPaginationLimits(mcp.PaginationLimits{
        MaxPages:       100,
        MaxCursorBytes: 64 << 10,
    }),
)
if err != nil {
    return err
}
defer client.Close()
```

`Start` does not wait for unsolicited server output. The client sends `initialize` first, as required by MCP. Cancelling the handshake context after a successful connection does not kill the child; `Close` owns transport shutdown and terminates the complete child process tree. On Windows the child starts suspended, is attached to a kill-on-close Job Object, and only then resumes. A pre-cancelled request is rejected before allocating an ID.

Request IDs accept strings or mathematically integral JSON numbers; exact forms such as `1.0` and `1e0` correlate without a `float64` round-trip, while fractional IDs fail closed. Cancelled numeric request IDs are retained in an exact bounded range set, so long contiguous cancellation runs stay compact and one late terminal response is consumed safely; an actual duplicate response still fails closed. If pathological range fragmentation exhausts exact correlation state, the peer closes fail closed instead of guessing whether an old ID was cancelled. Cancellation completes every waiter and cancels the request-scoped transport operation. Delivery waits are bounded, and cancelling a queued stdio write cannot abort another request's active write. Shutdown cancels and joins active server-to-client request handlers before transport close returns.

## Streamable HTTP

```go
transport := mcp.NewStreamableHTTPTransport(
    "https://example.com/mcp",
    mcp.WithStreamableHTTPRequestDecorator(func(req *http.Request) error {
        token, err := tokens.Token(req.Context())
        if err != nil {
            return err
        }
        req.Header.Set("Authorization", "Bearer "+token)
        return nil
    }),
)

client, err := mcp.Connect(ctx, transport)
if err != nil {
    return err
}
defer client.Close()
```

The transport uses one endpoint for POST and optional GET polling. JSON POST responses must be terminal and correlated to the original request ID. A terminal SSE response cancels its request-scoped POST even if the server keeps the stream open. Interrupted POST SSE streams resume independently with their own `Last-Event-ID`, honoring SSE `retry` before reconnect; retry delays are clamped to 100 ms–5 min to prevent server-driven reconnect storms. POST cursors never leak into the independent operation-phase GET stream. The SSE parser accepts CR/LF/CRLF framing, one leading BOM, and the specified event-ID plus empty-`data` priming event. A terminal POST/GET contract failure autonomously cancels every active HTTP operation. The transport also handles `202 Accepted`, `MCP-Session-Id`, `MCP-Protocol-Version` and best-effort DELETE on close.

The custom HTTP client option only imports safe timeout settings; custom transports cannot replace the SSRF-safe dialer. Use the request decorator for authentication headers. The decorator receives a bodyless temporary request; method, URL, Host and body-related fields are protected, and the final request is detached from the callback's pointer. OAuth discovery, consent and token storage belong to the host.

`ContentBlock.Size` uses `JSONNumber`, preserving exact integral JSON number forms (including exponent notation) without a `float64` round-trip. Fractional values are rejected because the MCP JSON Schema defines resource size as an integer byte count.

## Registering tools

```go
builder := toolsy.NewRegistryBuilder()
for proxy, err := range client.GetTools(ctx) {
    if err != nil {
        return err
    }
    builder.Add(proxy)
}

resourceTool, err := client.GetResourceTool()
if err == nil { // resources capability is optional
    builder.Add(resourceTool)
}

registry, err := builder.Build()
```

MCP `inputSchema` becomes `ToolManifest.Parameters`; `outputSchema` becomes `ToolManifest.OutputSchema`. Both schemas default to JSON Schema 2020-12 and preserve numeric constraints exactly beyond IEEE-754 precision. Structured results are validated before delivery. `isError: true` becomes `CodeRemoteExecution`, distinct from schema, JSON-RPC and transport errors.

## Invalidation

Discovery snapshots are versioned. After `notifications/tools/list_changed`, proxies created from the previous tools snapshot fail with `StaleDiscoveryError` until the host reloads and rebuilds its registry.

```go
go func() {
    for event := range client.Invalidations() {
        scheduleRegistryReload(event)
    }
}()
```

`InvalidationGeneration` and `DiscoveryGeneration` are authoritative even if a slow event consumer overflows the bounded notification channel.
`Client.Close` is concurrent-safe and closes both `Invalidations()` and `LogMessages()` after transport dispatch has stopped, so range consumers terminate normally.

## Migration from the old module

This is a breaking migration:

- remove `NewSSETransport`, `SSETransport`, `SSETransportOption` and every `WithSSE*` option;
- replace remote setup with `NewStreamableHTTPTransport`;
- remove protocol negotiation for `2024-11-05`, `2025-03-26` and `2025-06-18`;
- remove top-level `roots` from initialize; roots are returned from `roots/list`;
- move progress tokens to `params._meta.progressToken`;
- use fractional `ProgressInfo.Current` and `ProgressInfo.Total`;
- replace content fields `base64`/`mediaType` with `data`/`mimeType`;
- consume `CallToolResult`, `ContentBlock` and `ResourceContents` instead of shape-sniffing raw JSON;
- use `PromptsGetResult`; the legacy `PromptMessageResult` alias is removed;
- handle typed `ProtocolVersionError`, `CapabilityError`, `UnsupportedFeatureError`, `StaleDiscoveryError`, `RPCError`, `HTTPError`, `RemoteToolError` and `ErrSessionExpired`.

No deprecated aliases or compatibility shims are provided.

## Limits

| Path | Default | Configuration |
| --- | ---: | --- |
| stdio JSON line | 1 MiB | fixed |
| stdio protocol stdout stream | 16 MiB | `WithStdioMaxStreamBytes` |
| stdio stderr logged line | 256 bytes | fixed; excess is discarded without closing transport |
| Streamable HTTP JSON/SSE response | 16 MiB | `WithStreamableHTTPMaxStreamBytes` |
| discovery pages | 1000 | `WithPaginationLimits` |
| cumulative discovery cursor bytes | 1 MiB | `WithPaginationLimits` |

Context interruption wins over read-limit mapping. Errors never include Authorization headers, full binary blocks or unbounded response bodies.
