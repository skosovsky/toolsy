# Migration to MCP 2026-07-28 (task34)

Task34 is a major, clear-break migration. `toolsy/mcp` now speaks exactly MCP `2026-07-28`; it cannot be configured to speak `2025-11-25` or any earlier revision.

## Required server behavior

The server must implement `server/discover`. `Connect` requires the exact value `2026-07-28` in `supportedVersions`, validates the discovery `resultType` and cache fields, then snapshots capabilities. There is no downgrade or fallback. `-32601`, timeout, malformed response, EOF and a server that advertises only an older version all fail `Connect`.

Do not expect `initialize` or `notifications/initialized`: neither is sent. Do not return identity through the old discovery body field; the current location is result `_meta.io.modelcontextprotocol/serverInfo`.

## HTTP deployment changes

Streamable HTTP is stateless and POST-only:

- remove session affinity and `Mcp-Session-Id` handling;
- remove standalone GET polling, DELETE-on-close, `Last-Event-ID`, SSE resume and redelivery state;
- require `MCP-Protocol-Version` and `Mcp-Method` on every request;
- require `Mcp-Name` only for `tools/call`, `prompts/get` and `resources/read`; compare its decoded value with the request body because unsafe values use the Base64 sentinel representation;
- keep JSON/body data as the source of truth and treat mismatches as JSON-RPC `HeaderMismatch` (`-32020`).

Tool properties may declare `x-mcp-header`. Only static top-level `inputSchema.properties` of type `string`, `integer` or `boolean` are eligible; integers must fit `[-(2^53-1), 2^53-1]`. Suffixes must be non-empty RFC 9110 `tchar` strings and unique case-insensitively. A present non-null argument is mirrored into `Mcp-Param-{suffix}`. Values unsafe for a plain HTTP field, plus literal sentinel-shaped strings, use standard Base64 over UTF-8 in `=?base64?{value}?=`. Invalid annotations cause the malformed tool—not the full tools list—to be filtered.

HTTP cancellation closes the request-scoped response stream and does not emit `notifications/cancelled`. Broken streams are not resumed and tool calls are never retried automatically.

## Stdio changes

The first stdio request is `server/discover`; there is no initialize handshake. Stdio cancellation remains an explicit `notifications/cancelled` after successful wire delivery and carries the original raw request ID. A pre-delivery cancellation sends nothing. This differs intentionally from HTTP because stdio has no per-request response stream.

## Result and cache changes

All wire results require `resultType: "complete"` or a permitted `resultType: "input_required"`. Missing, null and unknown tags fail closed. `input_required` is valid only for `tools/call`, `resources/read` and `prompts/get`, and must carry interim `inputRequests` and/or `requestState`. The client does not auto-fulfill it. A host-managed next round gets a new request ID, current-round `inputResponses` and the server's byte-exact opaque `requestState`. Use `CallTool`, `ReadResourceRound` and `GetPromptRound` for explicit continuation.

Request-scoped logging is opt-in through `RequestMeta.LogLevel`; there is no session-wide logging state or `logging/setLevel` RPC.

`server/discover`, list results and `resources/read` now require `ttlMs` and `cacheScope`. Consume those typed fields when deciding whether to reuse a snapshot; the library does not refresh in the background. `structuredContent` may be any JSON value.

For persisted or compared snapshots, replace ad-hoc JSON hashing with `mcp.ComputeSnapshotDigest`. The accepted closed `mcp.Snapshot` set is `DiscoverResult`, `ToolsListResult`, `ResourcesListResult`, `ResourceTemplatesListResult`, `ResourcesReadResult` and `PromptsListResult`. Digests are deterministic across map order, insignificant JSON whitespace, equivalent exact number spellings and pointer/value forms, but differ across snapshot types, cache metadata and ordered entries. Invalid DTOs fail before hashing.

## Notification migration

Replace unsolicited list/resource notifications and `resources/subscribe`/`resources/unsubscribe` with `subscriptions/listen`. The stream begins with an acknowledgment containing the server's effective filter and `_meta.io.modelcontextprotocol/subscriptionId`. Reject data before acknowledgment and later events with the wrong subscription ID or outside the effective filter. Reconnect/resubscribe is an explicit host policy.

## Removed public surface

Remove usages of:

- `InitializeParams`, `InitializeResult` and any initialize-specific fields;
- `WithClientRoots`, `WithRoots`, `Root`, `RootsListResult` and roots request handlers;
- `logging/setLevel`, negotiated/session logging and base-protocol `ping`;
- `ErrSessionExpired` and session-expiry recovery;
- `RequestHandler`, `Transport.OnRequest` and server-to-client request dispatch;
- resource subscribe/unsubscribe APIs and state;
- any custom code that expects GET polling, session DELETE or SSE resume;
- protocol-version flags, aliases or fallback branches.

Keep `Connect`, stdio and Streamable HTTP constructors, typed tools/resources/prompts APIs, pagination limits, SSRF protections and request decorators. The exact exported replacement names are documented in package godoc; there are no deprecated shims.

## Schema and extension boundary

Schemas without `$schema` use JSON Schema 2020-12. Explicitly supported dialects remain explicit; unsupported dialects and remote `$ref` resolution fail closed. Protocol-owned `_meta` fields cannot be injected through caller `Meta.Extra`: a MetaObject namespace is reserved when its second DNS label is `mcp` or `modelcontextprotocol`. Non-reserved caller metadata remains lossless. This restriction is separate from capability extension declarations: official `io.modelcontextprotocol/*` extension IDs are valid, may be bound to an explicit codec, and remain inert without one. `logging`, `completions` and `experimental` advertisements are preserved as inert data but provide no removed runtime API. Treat server identity, request state, annotations and mirrored headers as untrusted metadata—not authorization evidence.

`ExtensionRegistry` and `ExtensionCodec` let hosts bind their own vendor or official extension types. Registration installs a codec but does not advertise or activate the extension capability.

## Release checklist

```text
cd mcp && go test -race ./...
make test
make lint
make test
```

The actual `make release-break` target is separate and destructive: after lint, tests and this preflight it can create a release commit, tag every module and push tags. Use it only to publish the release.

Also verify production code contains no `2025-11-25`, initialize lifecycle, session state, GET/DELETE transport paths, resume processing, roots, `logging/setLevel` or compatibility switches. Literal legacy header names may remain only in the request-decorator reserved-header denylist; they must never be emitted or parsed as protocol state. Older revision strings may remain only in migration documentation and explicit negative fixtures.
