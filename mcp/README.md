# MCP 2026-07-28 client for toolsy

`github.com/skosovsky/toolsy/mcp` is a strict MCP client and protocol bridge. It supports exactly `2026-07-28`; there is no legacy negotiation, fallback, compatibility mode or MCP server implementation.

All transport implementations and test doubles use `NotificationHandler` with
`json.RawMessage` parameters and the `PrepareRequest` delivery boundary. A
legacy `Call` method is not a substitute for preparation: request-correlated
state must be registered before delivery. The package owns one shared JSON-line
scanner cap; transports must not redeclare it or introduce compatibility shims.

`NewSSETransport` and its `WithSSE*` options are removed. Select
`NewStreamableHTTPTransport` for POST responses that contain request-scoped SSE.
There is no GET/endpoint-discovery stream, legacy initializer, session recovery
or server-request handler compatibility path. Migration retains applicable
transport safety checks; obsolete positive protocol scenarios become explicit
rejection/no-traffic checks rather than restoring unsupported features.

## Connect

Once `Connect` attempts `Transport.Start`, it owns cleanup on failure, including
a partially successful start. It calls `Close` and preserves the original start
error. Invalid client options rejected before `Start` do not transfer ownership.

```go
transport := mcp.NewStdioTransport("my-mcp-server", nil)
// or: transport := mcp.NewStreamableHTTPTransport("https://example.com/mcp")

client, err := mcp.Connect(ctx, transport)
if err != nil {
	return err
}
defer client.Close()
```

`Connect` sends `server/discover` first and accepts the server only when `supportedVersions` contains exact `2026-07-28`. The discovery result includes capabilities, extensions, cache metadata and optional server identity from result `_meta`. `initialize` and `notifications/initialized` do not exist in this implementation, and a failed discovery never falls back to them.

Every request carries the protocol version, actual client capabilities and client identity in reserved `_meta.io.modelcontextprotocol/*` fields. Caller `Meta.Extra` cannot create a MetaObject key whose second DNS label is `mcp` or `modelcontextprotocol`; the protocol builder exclusively owns those namespaces. Duplicate JSON keys and malformed tagged unions fail closed.

Set `RequestMeta.LogLevel` to opt a single request into bounded `notifications/message` diagnostics; no session-wide logging state exists. `ExtensionRegistry` and `ExtensionCodec` are the BYO-types boundary for typed extension payloads. Vendor IDs and official capability extension IDs such as `io.modelcontextprotocol/*` may be registered. Unknown declarations remain lossless and inert; registration does not advertise or enable a capability by itself. `logging`, `completions` and `experimental` advertisements are likewise inert data and expose no legacy runtime API.

## Results

Client discovery, call/read and list boundaries map transport read-limit failures
to `toolsy.CodeValidationFailed` with the actual transport byte cap and operation
subject. The underlying cause remains inspectable. Cancellation, deadlines and
timeout causes take precedence and do not become validation failures.

The SSE scanner has one byte of token headroom beyond the stream budget, so a
long line cannot replace the reader's budget failure with a scanner-only error.
The bounded stream reader remains authoritative: requesting more bytes after
exhausting its budget fails closed, including an unfinished line exactly at the
cap. An unfinished line below the cap instead fails the terminal/framing contract.
Scanner headroom does not increase the readable byte budget.

Every wire result requires `resultType`. `complete` is returned as a method-specific typed result. `input_required` is accepted only from `tools/call`, `resources/read` and `prompts/get` and is surfaced distinctly; the library does not automatically answer or retry MRTR rounds. A host-driven retry uses a fresh request ID, current-round `inputResponses` and the server's byte-exact opaque `requestState`. Use `CallTool`, `ReadResourceRound` and `GetPromptRound` for explicit rounds; convenience APIs return `InputRequiredError` rather than hiding interim results.

Discovery and cacheable list/read results expose typed `ttlMs` and `cacheScope` (`public` or `private`). Missing, null, fractional, negative or unknown values are protocol errors. Cache hints do not enable hidden caching or refresh.

Use `ComputeSnapshotDigest` when a host needs a stable identity for a discovery, tools, resources, resource-templates, resource-read or prompts snapshot. It first runs the strict wire encoder, rejects encodings above 8 MiB before canonical decoding, then hashes a bounded canonical JSON representation with explicit version and snapshot-type domain separation. The digest includes `ttlMs`, `cacheScope`, ordered list entries and all lossless wire metadata; pointer and value forms are identical. `SnapshotDigest.String` returns the 64-character lowercase SHA-256 hex value.

Tool schemas default to JSON Schema 2020-12 when `$schema` is absent and respect explicitly supported dialects. Local `$ref` is supported with bounded evaluation; arbitrary network fetch is not. `structuredContent` may be any JSON value. Content and resource unions, binary Base64 data and output schemas are validated symmetrically.

## Streamable HTTP

Streamable HTTP uses one endpoint and POST only. Each request includes:

- `MCP-Protocol-Version: 2026-07-28`;
- `Mcp-Method`, equal to the JSON-RPC method;
- `Mcp-Name` exactly for `tools/call`, `prompts/get` and `resources/read`, derived from `params.name` or `params.uri` so its decoded value matches the body value;
- `Mcp-Param-*` for a present non-null tool argument whose static top-level property has a valid `x-mcp-header` annotation.

`x-mcp-header` supports `string`, `integer` and `boolean`; integers must fit the JSON safe-integer range. Header suffixes are non-empty RFC 9110 `tchar` strings and case-insensitively unique. A malformed annotation removes that tool from a `tools/list` snapshot without discarding valid siblings. Unsafe values and values resembling the sentinel are UTF-8/Base64 encoded as `=?base64?{value}?=`. Base64 provides no confidentiality.

Responses may be terminal JSON or request-scoped SSE. Cancelling an HTTP request closes that request's response stream; it does not send `notifications/cancelled`. A broken stream is not resumed or automatically retried. Sessions, `Mcp-Session-Id`, GET polling, DELETE-on-close, `Last-Event-ID` and SSE redelivery are absent. A correlated HTTP 400 JSON-RPC `HeaderMismatch` (`-32020`) is returned as a typed protocol error.

SSE events are dispatched only at the terminating empty line. EOF discards an
unfinished event; a truncated terminal result cannot complete a request, and
truncated acknowledgements/notifications cannot trigger handlers. This follows
the [SSE framing rules](https://html.spec.whatwg.org/multipage/server-sent-events.html#interpreting-an-event-stream),
not the removed GET/reconnect transport behavior.

The request decorator is for authentication and trace headers. It cannot replace protocol-derived `Mcp-*`/`Mcp-Param-*`, method, URL, Host or body fields. The transport retains SSRF-safe dialing and redirects, bounded responses and secret-safe diagnostics.

The HTTP transport permits only same-origin GET/HEAD redirects. POST RPCs and DELETE session termination never redirect, including same-origin 307/308 body replay; method-changing redirects are also rejected. Configure the final MCP endpoint explicitly. A redirect failure occurs after the original dispatch and does not establish rollback or permission to repeat an RPC. `httptool.RedirectError` remains inspectable when the shared redirect policy refuses the request.

## Stdio and cancellation

Stdio uses one JSON-RPC message per line and sends `server/discover` first. Writes are serialized; stdout is protocol-only and stderr is bounded logging. After a request reaches the wire, context cancellation sends `notifications/cancelled` with the exact raw request ID. Cancellation before delivery sends no notification. Process failure unblocks all waiters, and `Close` terminates the complete child process tree.

Tool progress is request-scoped and strictly increasing. Its completion callback
is installed before `Deliver`: the wire-terminal boundary retires the route, so
late notifications cannot reach the consumer. Accepted buffered progress is
drained before delivering the terminal result. A custom transport must expose
`CompletionPendingRequest` for proxy tool progress; otherwise the prepared
request is aborted before any delivery.

Proxy tool calls own a child cancellation context for preparation and Await.
Returning from the invocation cancels that context, including after a consumer
abort with an uncancelled caller. Cleanup does not require the optional local
pending-cancellation interface or global transport Close. The proxy loop owns
cancellation notification dispatch; the background Await worker does not send
another cancellation. A completed wire request does not receive a consumer-abort
notification. Completion still retires progress before Await is released.

An intentional empty tool result retains its typed MCP value and result
envelope, sets `EmptyResult`, and has neither payload bytes nor MIME type, in
accordance with the core chunk contract. Consumer failures preserve their cause
alongside the stream-aborted marker.

This is intentionally different from HTTP cancellation: stdio has no per-request response stream, while HTTP does.

## Subscriptions and invalidation

Use `subscriptions/listen` for list changes and resource updates. The first SSE message must acknowledge the subscription with the effective filter and `_meta.io.modelcontextprotocol/subscriptionId`; notifications before acknowledgment are rejected. Later notifications must match both the active subscription ID and effective filter. The library does not silently reconnect a failed listen stream. Discovery generation counters remain authoritative if a bounded consumer channel overflows.

Resource subscription matching normalizes scheme/hostname case, HTTP/HTTPS
default ports and unreserved percent-encoded path octets. Reserved escapes and
repeated slashes remain distinct; dot segments are rejected. Queries and
fragments permit only the same normalized URI, not descendant matches. These
rules route opted-in notifications; they do not confer filesystem or host rights.
Normalization follows [RFC 3986](https://www.rfc-editor.org/rfc/rfc3986#section-6.2.2).

## Tool registration

```go
builder := toolsy.NewRegistryBuilder()
for proxy, err := range client.GetTools(ctx) {
	if err != nil {
		return err
	}
	builder.Add(proxy)
}
registry, err := builder.Build()
```

`inputSchema` maps to `ToolManifest.Parameters`; `outputSchema` maps to `ToolManifest.OutputSchema`. `isError: true` becomes a remote execution error, distinct from JSON-RPC, schema and transport errors. Annotations remain hints, not authorization policy.

## Host classification and current authorization

Remote `annotations` are untrusted hints. The default proxy is `Dangerous: true`,
`ReadOnly: false`, `Idempotent: false`, irrespective of `readOnlyHint`,
`destructiveHint` or `idempotentHint`. `ListTools` preserves the original typed
annotations and extension metadata for display and diagnostics; the manifest
contains only host execution properties, not server claims.

`WithToolPolicyMapper` is an explicit host decision during `GetTools`. It receives
that call's host context and an owned `MCPTool` descriptor snapshot. For example:

```go
client, err := mcp.Connect(ctx, transport, mcp.WithToolPolicyMapper(
    func(ctx context.Context, source mcp.MCPTool) (mcp.ToolExecutionProperties, error) {
        // This allowlist is host-owned and scoped to the authenticated connection.
        if source.Name == "catalog.lookup" {
            return mcp.ToolExecutionProperties{ReadOnly: true, Idempotent: true}, nil
        }
        return mcp.ToolExecutionProperties{Dangerous: true}, nil
    },
))
```

The mapper classifies the discovery snapshot; it does not authenticate a caller
or issue a grant. Current per-invocation authorization remains the Registry or
typed host policy's responsibility. A mapper error prevents that proxy from
being delivered. Remote metadata, model text and claimed tool names cannot
establish authenticated subject/scope or connection trust.

The optional `ResultCache` runs after current host authorization and requires a
host-owned eligibility predicate, freshness partition and complete codec. Server
hints and a mapper's `Idempotent: true` alone never enable reuse or promise remote
exactly-once execution. The per-attempt host predicate must approve reusable data. Cached delivery and dispatch both reject stale
MCP discovery generations, and revoking the current Registry policy prevents
both handler execution and replay. A policy must enforce consent explicitly;
`Dangerous` is a classification field, not an automatic approval mechanism.
The generation guard rejects stale proxy instances; it does not invalidate stored
cache entries when the host builds a new proxy after rediscovery. The host's cache
partition must bind the authenticated connection and relevant discovery/dependency
freshness, for example `client.DiscoveryGeneration(mcp.InvalidationTools)`, to
prevent reuse across generations or connections with identical manifests.

## Capability boundary

The client remains pinned to MCP `2026-07-28`. It advertises no optional
server-to-client capabilities: no elicitation, sampling, roots service, Tasks
runtime or extension runtime. An `input_required` result with supported opaque
`requestState` is returned to the host without automatic continuation. An
`inputRequests` requirement for an undeclared capability is a typed
`UnsupportedFeatureError`; the client sends no hidden follow-up or retry.

`ExtensionRegistry` is a host-owned JSON codec registry. Registering a codec
permits explicit encode/decode of that identifier only. It does not advertise a
capability, install protocol handlers or execute a remote extension. Unknown
identifiers are typed refusals. OAuth discovery/refresh, elicitation UI, persistent
remote scheduling and new protocol profiles require a separate consumer-driven
activation, as described in [Task35 activation](../docs/task35-activation.md).

The trust/cache/policy, stale-generation, input-required and extension assertions
are fixture and local integration tests. They do not constitute live remote
interoperability verification.

## HTTP authentication failures

HTTP 401/403 returns `*HTTPError` with bounded authentication `Challenges` for
explicit host inspection. Safe formatting and structured logging omit challenge
values. The narrow projection retains scheme, known Bearer error codes and safe
HTTPS `resource_metadata` hints; raw headers, credentials and free-text parameters
are discarded. Hints remain untrusted, and no discovery, refresh or POST retry
is triggered. See [the projection contract](docs/http-auth-challenges.md) for
bounds, unsupported parameters and the host's network/identity responsibilities.

## Removed APIs

The clear break removes initialize DTOs/lifecycle, `WithClientRoots`, `WithRoots`, roots handlers, `logging/setLevel`, base `ping`, `ErrSessionExpired`, server-request dispatch, resource subscribe/unsubscribe, session/GET/resume internals and every older protocol revision. No deprecated aliases are provided.

See [migration-task34.md](../docs/migration-task34.md) for the full migration checklist.

## Limits and verification

I/O, pagination, schema composition and diagnostics remain bounded. Context cancellation takes precedence over transport/read-limit mapping. Errors do not expose authorization/cookie headers, full binary blocks or unbounded bodies.

The Contract-First test anchor is the byte-exact official `2026-07-28` schema at `testdata/task34/schema/mcp-2026-07-28.schema.json`, pinned to upstream commit `271ecc9accafdd9b83a3c869fa67c22953b2af80` with SHA-256 `ef70b61f99b6d2e5e3b46863822eab08dff6a45bedc7a08914e0e5b133f40203`. Its provenance manifest cites all 19 wire fixtures, and `make task34-preflight` verifies the trust anchor without network access.

```text
go test -race ./...
make test
make lint
make task34-preflight
```

`make release-break` is the actual destructive release workflow: after lint, tests and preflight it may create a release commit, create tags and push tags. Run it only when intentionally publishing the clear break.

`WithStreamableHTTPSettings(httptool.ClientSettings{Timeout: ..., TLSConfig: ...})`
replaces the removed `WithStreamableHTTPClient` accept-and-ignore option. Settings
apply to one owned safe pool; invalid settings fail Start before dispatch. Close
releases owned idle connections after active posts finish. TLSConfig is cloned;
referenced certificates, root pools and callback state remain immutable host
state. A positive timeout also bounds each SSE response; zero uses caller context.
