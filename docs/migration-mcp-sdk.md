# Migrating toolsy/mcp to the official SDK

This candidate intentionally breaks the former public MCP API. No deprecated
aliases or compatibility protocol backend remain. Pin toolsy/core and this optional
module together; source consumers before publication use explicit local replacements
as in `mcp/testdata/consumer/go.mod`, with `GOWORK=off`.

| Former API family | Replacement |
|---|---|
| Connect, ClientOption/WithX settings | Host sdk.NewClient / sdk.ClientOptions, ComposeClient then Install before Connect |
| custom Transport, Peer, PreparedRequest, HTTP/STDIO constructors | Official SDK transports; host HTTPClient or supervisor + SDK IOTransport |
| Discover/DiscoverTools/ToolDiscovery | Importer.Discover; owned descriptors + proxies, generation and new digest domain |
| MCPTool, content, resource, prompt, result and wire DTOs | SDK types directly; result Chunk.TypedResult retains owned SDK DTO |
| CallTool / explicit round methods | Core registry/view/session execution; NeedsInputError and host WithRound for continuation |
| GetResourceTool | Importer.ResourceTool; core validates URI arguments and applies policy |
| ListResources/Templates, Get/ListPrompts, iterators | SDK ClientSession methods and iterator APIs |
| WithToolPolicyMapper | ImportConfig.Classify returning explicit trusted core options |
| DiscoveryGeneration, Invalidations | Snapshot.Generation and Importer.Updates; stale proxies reject dispatch and replay |
| ComputeSnapshotDigest | Snapshot.Digest: new SDK-visible domain, host identity + generation; discard old wire digest identities |
| Listen, Subscription handles, ACK/filter retirement | SDK subscription methods and notification options; no equivalent public ACK/filter/error handle in v1.8.0 |
| ExtensionRegistry/Codec | Host-owned domain codecs and SDK-supported extension/metadata APIs; no duplicate wire registry |
| HTTPError/auth decorators | SDK OAuth or host HTTPClient; BridgeError retains causes without printing remote secrets |
| server direction (new) | Export(scopedView, ExportConfig) returns official SDK Server |

Create the host client from ComposeClient's returned options. Install hooks before
the handshake, then Bind a session with authenticated identity. If authority changes,
close that binding and create a new connection/binding; never derive identity from
remote TTL, cacheScope or _meta. If invalidation occurs, rebuild the registry;
rediscovery revokes old proxies and replay, but does not bypass SDK TTL. Close the
importer and then the host-owned SDK session. Importer.Close alone does not join
SDK's connection-lifetime Wait observer.

Current authorization remains a core policy concern at every invocation, including
cache replay. Classification is not authorization or automatic consent. Bind result
cache partitions to authenticated connection and snapshot generation. MRTR is manual:
handle NeedsInputError, validate host responses and use WithRound for one next attempt.
A partial/intermediate/failed result does not prove rollback and never permits
automatic effect retry. The core operation profile owns claims and uncertainty.

Export an explicit scoped selection with a trusted transport resolver. For HTTP,
select a separate SDK server per immutable tenant boundary. An optional Session
factory must use the same view. Confirmation requires a host Control mapping;
independent streams require an explicit projection before registration. Revalidate
current core grants against exact arguments. RequestState, InputResponses and
remote metadata never confer authority.

Former exact-version-only, strict wire decoding, POST-only HTTP, custom SSE,
no-session/no-fallback and process-tree guarantees now belong to the SDK/host
transport contract. SDK may negotiate fallback and manage HTTP session/listen
streams. Do not assume the former restrictions still apply. Host HTTP clients must
set desired timeout/network/redirect/security budgets; SDK ownership does not imply
httptool SSRF policy. SDK CommandTransport controls its child, not descendants.
A host supervisor may use SDK IOTransport for required tree/stderr/total-byte limits.

See [accepted SDK limitations and upgrade steps](../mcp/README.md#sdk-limitations-and-upgrade-contract).
Numbers, explicit null/raw tokens and unknown wire fields are not losslessly retained
on v1.8.0; progress retirement is after SDK method completion, and subscription ACK
lifecycle has no equivalent public handle. These are dependency constraints, not
permission to weaken core execution boundaries or silently discard SDK-visible data.

Run both independent examples, the SDK-only fixture and external consumer with
GOWORK=off, then root/nested race tests and lint. Upgrades require reviewing any
changed characterization instead of making it pass by weakening assertions.


## Auth migration

Replace legacy auth decorators with host composition of SDK auth and oauthex.
Use AuthorizationCodeHandler in StreamableClientTransport.OAuthHandler; supply
HTTPClient, preregistration, consent and token storage from the host. Protect HTTP
export with RequireBearerToken and publish ProtectedResourceMetadataHandler.
Resolve identity from TokenInfoFromContext into immutable scoped CallContext,
never from model arguments or _meta. OAuth consent/access scopes are separate from
exact-argument operation approval. On principal changes retire importer AND SDK
session and rebuild token/cache partitions. See [auth contract](mcp-sdk-auth-contract.md)
and the runnable `mcp/examples/auth` example. Auth HTTP retries belong to SDK;
verification before dispatch and core uncertainty handling preserve effect contracts.
Public jsonrpc.Error is retained as inspectable BridgeError cause with code/data;
its message/data can be untrusted secrets and must not be logged automatically.

For outbound protocol Data use the trusted ExportConfig.ErrorMapper to return
a top-level public jsonrpc.Error with safe message/data. Default safe wrapping
retains code but SDK v1.8.0 omits wrapped Data. Raw domain causes are never
published automatically; mapped diagnostics are owned and bounded.
