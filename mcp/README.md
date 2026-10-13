# MCP integration through the official Go SDK

`github.com/skosovsky/toolsy/mcp` is an optional adapter over the published
`github.com/modelcontextprotocol/go-sdk v1.8.0`, replaced by
`github.com/skosovsky/go-sdk v1.8.1-0.20261013081052-4a7b89fe086f`.
Consumers must add the same `replace` to their main module: Go does not inherit
dependency replacements. The core module has no MCP
SDK dependency. Client import and server export can be used independently.
This is a breaking migration; it does not promise lossless wire preservation.

## Client import

1. Call `ComposeClient(hostOptions, ImportConfig)` before creating the SDK client.
2. Create `sdk.NewClient` with the returned options and call `hooks.Install(client)`
   before `Connect`. The host creates its SDK transport and owns the connection.
3. Bind the connected SDK session with a host-owned authenticated authority identity.
4. Call `importer.Discover(ctx)` and register `snapshot.Tools` in the ordinary core
   registry/view/session. Call `importer.ResourceTool()` for a resource-reading tool.
5. Consume `Updates()` and rebuild the registry when generation changes. Close the
   importer when disposed; the host must separately close its SDK session.

`ImportConfig.Classify` is a trusted host function returning core tool options.
It receives an owned SDK descriptor, never the SDK cache object. Remote annotations
are hints; they grant no permissions. The host must classify imported tools and
configure current core policy, approval, operation execution profiles and caches.
Classifier options must not weaken schema validation. Resource reads use the
core read-only classification; host authorization still applies to the URI.

Options are copied before composition. Adapter state updates before each original
host callback, which runs once with SDK failure/panic semantics. Callbacks must
return promptly. Installing hooks after Connect is unsupported and Bind rejects
sessions whose initial handshake was not observed. One client can bind multiple
sessions; notification routing uses session identity plus an opaque per-call token.

Each execution performs one SDK call. Automatic MRTR is disabled; an explicitly
automatic host setting is rejected. `NeedsInputError.Result` retains an owned SDK
result with request state/input requests. Only the host chooses continuation using
`WithRound(ctx, Round{RequestState: ..., InputResponses: ...})`. Host values whose serialized representation changes during SDK DTO ownership copying are rejected with ErrUnsupported before dispatch, including nonrepresentable int64/json.Number/raw numeric tokens; this applies to both tool and resource continuation. These values are
not approval grants and do not prove that no effect occurred. Direct prompts,
resources, templates, extensions and subscription operations use the host SDK
session; there is no second client facade.

Schemas received from the SDK enter `NewProxyTool` unchanged. Input validation
precedes dispatch; output validation precedes successful delivery/persistence.
`Chunk.TypedResult` retains the owned SDK result and ordered content blocks,
annotations, metadata, MIME and bytes. Structured content is the JSON business
projection; unstructured results retain the complete SDK wrapper as JSON.
`isError` becomes a core remote-execution error chunk. SDK faults, cancellation,
schema errors and input-required remain distinct; `BridgeError` safely formats
its category while retaining causes through `errors.Is/As`.

Snapshots own mutable SDK DTOs independently of compiled tools and the SDK cache.
Every successful discovery and notification revokes old generations, including
cached replay. Digest domain `toolsy/mcp/sdk-snapshot/v1` binds SDK-visible tools,
authority identity, generation and method. It is not the legacy raw wire digest.
The host's result-cache partition must also bind that identity/generation; remote
TTL/cacheScope is not execution authority or permission to cache results.

Adapter defaults: 128 pages, 10000 tools, 8 MiB serialized SDK descriptors,
32 concurrent calls, 64 progress events per call, 64 invalidation events.
Cursor loops/duplicate names/aggregate limits reject discovery without publication.
Progress overflow cancels only that child; invalidation overflow closes the importer
fail-closed. One serialized yield loop drains accepted progress before terminal.
Consumer failure cancels the child and leaves sibling calls/the SDK session alive.
Remote `_meta`, effects, controls or claimed provenance remain inert data.

## Server export

`Export(scopedView, ExportConfig)` returns an SDK server. An explicit `Names` list,
`Implementation` and trusted `Resolve` function are required. The host resolves
subject/scope from the transport context, not arguments or `_meta`. Use one SDK
server per immutable tenant/catalog boundary; an HTTP handler can select that
server from authenticated request context. Discovery and invocation authenticate
against the same boundary; invocation resolves current authority again.

Raw `Server.AddTool` preserves explicit input/output schemas and raw arguments.
Handlers execute through the scoped RegistryView or a trusted Session factory
built from that view. No handler directly calls a domain tool. Model-controlled
request state/input responses cannot create approval grants. Core owns current
policy, exact-argument approval binding, operation claims, replay and uncertainty.

The default projection accepts one model-audience terminal result: JSON business
value, text/image/audio, an intentional empty result, or a safe soft-error message.
Internal effects/controls/provenance are not serialized. Independent streams need
an explicit `Project`; confirmation tools need a host `Control` mapping before
registration. The mapping must preserve the core challenge/grant workflow; it is
not itself an approval engine. Binary/application attachments and other audiences
need an explicit projection. Collection is bounded (1024 chunks, 8 MiB data by
default). Delivery failure after an effect never causes adapter retry/resend.

## SDK limitations and upgrade contract

These are accepted dependency limitations, not claims of upstream fixes:

| Limitation in pinned SDK | Evidence / upstream |
|---|---|
| Schema and metadata numbers round | `TestCharacterizationSchemaActualWire`; [#1310](https://github.com/modelcontextprotocol/go-sdk/issues/1310), [#1311](https://github.com/modelcontextprotocol/go-sdk/pull/1311) |
| Generic AddTool numeric conversion | [#1201](https://github.com/modelcontextprotocol/go-sdk/issues/1201), [#1302](https://github.com/modelcontextprotocol/go-sdk/pull/1302); adapter uses raw Server.AddTool |
| Unknown result fields disappear; duplicate keys/tags are permissive | DTO and actual HTTP characterization tests; no separately verified issue |
| SDK TTL cache returns shared pointers, no public per-call bypass | `TestCharacterizationCatalogCacheAliasing`; adapter clones DTOs but Discover is not force-refresh |
| SDK mutates call parameters | `TestCharacterizationCallParamsMutation`; adapter passes owned arguments/rounds |
| No public pre-Await completion hook | Sending middleware retires after SDK method returns; progress scheduled after wire terminal but before that point can be accepted. No strict wire-terminal guarantee |
| No public subscription ACK/filter/ID/error handle | SDK owns automatic listen/ACK and reconnect behavior; public Subscribe/Unsubscribe manage SDK-owned listening, and unsupported filters may be narrowed without a typed capability error (SDK-only lifecycle characterization); adapter Updates is local revocation, not a subscription handle. Direct public Subscribe/Unsubscribe delegates resource subscriptions. A stream whose subscription handler rejects may still make public Subscribe return nil; the SDK marker remains and the next Subscribe can return nil without reopening; host Unsubscribe clears it (TestCharacterizationSubscriptionFailureIsNotReportedOrRetried). No separate upstream issue confirmed |
| Wrapped JSON-RPC error Data is omitted | `TestCharacterizationWrappedJSONRPCErrorDataLoss` uses SDK public Encode/DecodeMessage only; code/outer safe message survive. Explicit host `ExportConfig.ErrorMapper` returns an owned bounded top-level public jsonrpc.Error with approved data; default raw causes/data remain private. No separate upstream issue confirmed |
| OAuth connection closure can collapse typed authorization causes | `TestCharacterizationOAuthClosureCauseCollapse`; SDK ErrConnectionClosed survives but a callback cancellation cause may become text only. Host must track callback/context cancellation independently; no separate upstream issue confirmed |
| Wait has no cancellable observer variant | One observer per observed connection remains until host session closes, even after importer.Close; host must dispose the session |
| SDK CommandTransport owns child, not a process-tree supervisor | Hosts requiring descendant cleanup/stderr budgets must supply a supervisor through SDK IOTransport; no former tree-cleanup guarantee is claimed |

No adapter marshal can reconstruct numbers or fields lost before the SDK callback.
Do not feed numeric tokens requiring precision beyond the SDK's representation to
these paths. Input rejected before loss can be handled by host/domain validation;
already rounded inbound results cannot be reliably detected or repaired.

The pinned fork preserves large numeric JSON-RPC IDs, structured content numbers
and explicit structured-content nulls; SDK-only characterization covers these paths.

On upgrade: pin the SDK version/replacement in the optional module and test fixtures;
run the characterization suite first, inspect changed observations and upstream
issues, update accepted constraints deliberately, then run all adapter contracts,
GOWORK=off consumers, examples, root/nested tests and lint. A newly passing upstream
case should remove its limitation, not silently leave stale characterization.
Keep the SDK replacement explicit; do not add a parser, wrap Connection or install
a bypass transport.

## Migration and verification

See [migration guide](../docs/migration-mcp-sdk.md),
[design and coverage matrix](../docs/mcp-sdk-integration-design.md), and
[historical API/test inventory](../docs/mcp-sdk-api-inventory.md).
Runnable independent examples: `GOWORK=off go run ./examples/import` and
`GOWORK=off go run ./examples/export` from this module. Both print `{"n":7}`.
The adapter contract tests interoperate with real SDK in-memory/HTTP/STDIO transports;
no external service or release publication is required.

`make test` and `make lint` from the repository check root and all nested modules,
including the SDK-only characterization fixture and external consumer fixture.


## HTTP OAuth and identity composition

Runnable local synthetic example: `GOWORK=off go run ./examples/auth` prints
`{"n":7}`. It combines stock SDK AuthorizationCodeHandler on the host-owned
StreamableClientTransport, host HTTPClient settings, SDK RequireBearerToken and
ProtectedResourceMetadataHandler, and verified TokenInfoFromContext mapped into
export CallContext subject/tenant. The adapter owns no credentials or token store.
The example starts with a synthetic stored token; sdk_auth_test.go additionally
exercises real SDK challenge/discovery, PKCE/code exchange and refresh.

Host owns token persistence (NewTokenSource/InitialTokenSource), consent callbacks,
issuer/audience/signature verification, IdP registration and network budgets.
Verifier failures must use safe errors: SDK bearer middleware can write the verifier
error to its HTTP response. SDK OAuth errors may retain sensitive response bodies
in inspectable causes; use BridgeError for safe diagnostics and never log causes
wholesale. OAuth consent and access scope do not replace core operation approval.

Bind identity is immutable. On principal/credential change close BOTH importer and
host session, create a new OAuth/token partition and client session, then Bind again.
An SDK cacheScope or model _meta/arguments is never an authority source. Use one
immutable exported server/view per trusted tenant scope. See the
[auth contract](../docs/mcp-sdk-auth-contract.md) for the complete host boundary.

The stock transport may retry a request once after 401/403 and successful Authorize.
Bearer verification must run before business dispatch; do not return an auth challenge
after an effect. Adapter adds no retry, and uncertain effect outcomes must not be
resent. Synthetic tests count business handlers independently of OAuth HTTP rounds.
STDIO composition has host/process trust and requires no HTTP OAuth.


`ExportConfig.ErrorMapper` is an optional trusted host projection for necessary
outbound JSON-RPC diagnostics. It receives the local cause and SDK request and
returns a public `*jsonrpc.Error` with explicitly approved message/code/data.
Returning nil keeps normal safe errors. The adapter copies Data and bounds combined
message/data bytes; invalid JSON or over-budget diagnostics fail safely without
retry. Do not forward raw cause messages/data by default: they may contain secrets.
Inbound protocol errors retain public code/data in BridgeError's inspectable cause.
SDK v1.8.0 drops Data when an error is wrapped; its public top-level Error supports
safe host projection without a transport/parser fork. Local Go causes cannot be
serialized across a wire; hosts can inspect them inside ErrorMapper.
