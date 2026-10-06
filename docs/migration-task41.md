# Task 41 API migration

## HTTP redirects (R01)

Agents, OpenAPI and GraphQL no longer transparently reroute RPCs. Configure the
final endpoint in `agents.NewClient`, `openapi.Options.BaseURL` / source servers,
or `graphql.Introspect`. The specification URL itself must remain within its
original origin on GET redirects.

Their shared `httptool.CheckRedirectRemote` accepts only requests that started
as GET/HEAD and remain within the original scheme, hostname and effective port.
Different ports and subdomains are different origins; HTTPS downgrade and HTTP
upgrade both cross that boundary. Same-origin GET/HEAD redirects retain headers.
The default web scraper and remote document reader use the same helper, so
their GET redirects must also remain within the original origin; supply the
final URL explicitly. Their initial URL validation remains correctable; a
refused redirect does not. MCP HTTP also uses this helper: POST RPCs and DELETE
session termination do not redirect, GET/HEAD stay in the original origin, and
method-changing RPC redirects use the same typed refusal contract.

`httptool.CheckRedirectAllowed` still permits whitelisted GET/HEAD redirects,
removing Authorization, Cookie and Proxy-Authorization on origin changes.
Both helpers now refuse every redirect for an initial POST/PUT/PATCH/DELETE or
other method, even if HTTP would rewrite it to GET. GraphQL queries and
introspection use POST, so neither redirects. HTTP toolkit POST also refuses
redirects; configure its final URL in the authorized call.

A refused redirect is inspectable with `errors.As` as `*httptool.RedirectError`.
The outer `ToolError` uses nonretryable `CodeRemoteExecution`, including URL,
IP, whitelist and blacklist refusals during redirect validation. An underlying
validation cause remains inspectable, but the outer classification does not
authorize correction/retry.
The first request was already dispatched and may have produced effects; use the
host's reconciliation contract before deciding on a new request. Agent create
continues to wrap the error in its unknown-outcome envelope.

`NewSafeHTTPClient` accepts an explicit host redirect callback; hosts supplying
their own callback own its method/origin/header/body semantics. Nil disables
redirects. Consumer custom clients continue to merge timeout only; they cannot
override these adapter policies.

## Host allow/deny patterns (R02 / D17)

`MatchHost`, `HostBlocked`, `HostMatchesAllowedDomains`, `SafeDialOptions` and
toolkit domain options now share one syntax:

| Entries | Matches |
|---|---|
| `example.com` | Exact apex only |
| `.example.com` | Descendants only, including nested subdomains |
| `example.com` and `.example.com` | Apex plus all descendants |

Previously bare entries also matched descendants. Update any allowlist that
intended that behavior to include the leading-dot entry explicitly. **Update
blacklists too:** replacing an old `"evil.com"` entry with
`"evil.com", ".evil.com"` preserves the previous apex-plus-descendants denial.
This applies to `web.WithBlockedDomains` and to shared HTTP library consumers.
Leaving only the bare entry intentionally denies the exact apex alone.

Case and surrounding whitespace are ignored. A terminal DNS root dot is
normalized on both configured patterns and request hosts, so `EXAMPLE.com.` and
`example.com` have identical host policy. A leading-dot pattern does not match
the apex or a name such as `evil-example.com`. IP checks still apply at dialing.

Every matching deny entry takes precedence, including broad allow plus narrow
deny and identical suffix policies. This replaces the precomputed exact
`conflictDeny` map. Denials happen before DNS lookup or dialing. A configured
nonempty allowlist that normalizes to no usable entries remains deny-all; it
does not silently switch to permissive blacklist mode.

## Exact input numbers and structure (R03)

Validated typed, dynamic and proxy inputs now use the same lossless JSON parser
and exact-number JSON Schema compiler. Dynamic `ValidateArgs`/`Handler` maps and
numbers inside typed `any`/map/interface fields contain `json.Number`. Replace
`value.(float64)` assertions with `value.(json.Number)` and explicit `Int64` or
`Float64` conversion, handling errors. Prefer declared `int64` fields for IDs;
explicit Go float fields still choose floating-point semantics. Values already
rounded by the host before constructing a schema/input cannot be recovered.
Use `json.Number` or integral Go values for exact numeric schema constraints.

Minimum, maximum and enum validation preserves distinctions beyond 2^53,
including max int64; prepared snapshots and cache identities retain those digits.
Schema normalization preserves property names such as `id` and literal
const/enum/default objects. Internal input compilation uses the same default
JSON Schema draft 2020-12 engine as proxy/output validation; explicit supported
dialects are honored and external schema loading stays disabled.

All validated input paths reject duplicate object keys recursively (including
escaped spellings of the same key), trailing JSON documents and malformed JSON.
Root depth is zero; values deeper than 128 or documents above 100,000 value
nodes are rejected before dispatch. These are structure limits, not byte budgets;
hosts still bound input bytes and string lengths. This intentionally replaces
typed/dynamic last-key-wins behavior. Declared custom `UnmarshalJSON` methods
remain host-owned after the shared structure/schema check; they can choose their
own Go representation. Typed field-name matching follows encoding/json rules.

## Post-handler validation and claimed operations (R04 / D12)

Typed `ResultValidator`, `EffectValidator` and `Postcondition` failures now have
an outer nonretryable `CodeInternal` error with no fixable arguments. Use
`errors.As` for `*ResultContractError`: its `Kind` identifies the failing phase
(`result_validator`, `effect_validator`, `postcondition`), and `Unwrap` retains
the exact original cause. Even a callback-supplied correctable/retryable ToolError
cannot override the known post-handler classification. Inspect the **outer**
ToolError for routing; finding a validation sentinel deeper in the cause chain
does not authorize argument correction or redispatch. Registry/Session timeout
normalization also preserves this classification when the callback cause is
context.DeadlineExceeded.

Pre-handler argument/schema errors keep their existing correction semantics.
A post-handler rejection emits no result and does not undo external effects.
See the runnable [contract_recovery example](../examples/contract_recovery/main.go).

`OperationOutcomeError` now says “after claim; dispatch may have occurred”. Its
`DispatchInvoked` field records whether this profile invoked its continuation;
false is local diagnostic evidence, not a fenced proof of externally not-started
and not permission to roll back the claim. Cancellation, lease/approval expiry
before invoke and post-handler failures all leave an unfinished claimed attempt
unknown. Reconcile through the bound operation reference; repeated delivery does
not blindly dispatch it. A persisted completed result stays completed if delivery
later fails. Use keyed error literals when constructing OperationOutcomeError.

The public error-chunk formatter preserves nonretryable INTERNAL and emits host
reconciliation guidance for these known failures. WithErrorFormatter leaves them
as hard errors. Batch/iterator/control routing cannot treat a diagnostic timeout,
stream-abort or control sentinel in their cause as permission to suppress the
contract failure or pause the host. errors.Is still exposes the original cause.
Shared web/RAG/SQL host result validators use the same result_validator phase;
this does not claim that a read performed an external write. Formatter callbacks
and output byte-cap policies are separate contracts.

## Release preparation (R05 / D37)

`make release-patch` and `make release-break` now build the host CLI and prepare
committed HEAD in a private clone. Linux and macOS are supported. The source
checkout must have no tracked changes. Untracked and ignored files remain local;
they never become candidate inputs. The source branch, index, files and refs are
preserved on success, failure and cancellation. Effective Git identity plus commit.gpgsign, gpg.format and user.signingkey
are copied into the private clone; hooks are disabled there. Other host Git
settings remain inherited from the host, so repository-specific signing programs
may need host configuration. Signing failure aborts before publication.

The inventory comes from all committed `go.mod` paths. An optional legacy module
list must equal that inventory. All module paths must follow the root module plus
the relative directory. Internal dependencies must form a DAG; cycles and missing
owned modules fail explicitly. Manifest edits use `go mod edit`, align owned
requirements and remove owned development replaces. External local replaces and
nonregular manifests/checksum files are rejected. Only expected `go.mod`/`go.sum`
files are staged. Before checkout, a private index and cached attribute check
reject attributes that transform checkout/archive bytes: filter, working-tree-encoding,
export-ignore, export-subst, ident, crlf, text and eol (absent or explicitly unset
attributes are allowed). Ordinary diff/merge/linguist metadata is supported. Host
global attributes are included in this check. Checkout uses LF and real symlinks.
Bootstrap applies the same attribute preflight before building its Git archive.

For each module, in dependency order, preparation tidies its release manifest,
creates the Go module ZIP with `x/mod/zip` (including inherited root LICENSE),
downloads that ZIP through an invocation-owned file proxy, and compiles its packages
and tests with `GOWORK=off` and `-mod=readonly`. Peer ZIP checksums are final before
preparing dependents. Each module has a ten-minute verification deadline. The CLI
then runs `make lint test` in the private checkout and the existing break preflight,
when present, with a thirty-minute deadline. Tracked changes from checks abort the
release. This verification checks the exact artifact graph; it does not execute
all dependency tests from their downloaded ZIPs.

The private module cache and GOPATH (including checksum-database state) are removed
on exit. Existing downloaded archives provide a read-only fallback before the
configured GOPROXY. Existing external GONOSUMDB policy is retained. If GONOPROXY
matches any owned module, verification sets that bypass to `none` and uses only
cached archives or direct VCS for external dependencies, preventing private module
names from reaching a public proxy. This case requires cached dependencies or
working VCS access; a proxy-only installation may fail explicitly. Existing Go
build/lint caches may still be reused.

To verify without publication, run
`bash scripts/release.sh -prepare-only patch`. The host wrapper archives committed runner code into a private bootstrap directory
and builds with
`GOWORK=off`, `-mod=readonly` and `-buildvcs=false`. SIGINT/SIGTERM cancel preparation
and terminate CLI command process groups before private-directory cleanup. The
bootstrap build has its own job process group; cancellation sends TERM, waits at
most two seconds, then sends KILL to that group and removes its directory.
Publication confirmation owns its input reader; Close must unblock Read.

The wrapper clears Git repository-selector environment variables and rejects
counted/serialized inline Git configuration before bootstrap; configure bootstrap
authentication in normal Git config or SSH facilities. The native CLI supports
counted authentication/identity configuration, strips counted repository, hook,
fsmonitor and checkout overrides, and rejects GIT_CONFIG_PARAMETERS. These
overrides cannot redirect its private index/worktree or change verified bytes.

Publication uses explicit root/submodule tag refspecs from this invocation, with
--atomic and --no-follow-tags; mirror configuration is disabled for that command.
Unrelated local tags never enter the train. Exactly one push destination is
required; fetch and push destinations may differ. Local and push-remote collisions
are checked before manifest preparation and again after confirmation. Existing
refs are never deleted or forced. A server without atomic push support or a
rejected tag fails the whole train; there is no sequential fallback.

Preflight cannot lock remote refs. Git arbitrates concurrent conflicting updates
at atomic push; an identical concurrent tag may be reported up-to-date. A transport
failure after the server accepted a push leaves publication outcome uncertain:
inspect the remote before retrying. Local cleanup does not roll back remote tags.
All release regressions use disposable local bare remotes; no production
publication is used for verification.

## HTTP pool ownership and explicit settings (R07 / D18 / D19)

Replace `WithHTTPClient` / `Options.HTTPClient` and `MergeHTTPClient` with explicit
`httptool.ClientSettings{Timeout: ..., TLSConfig: ...}` through
`WithHTTPSettings` / `Options.HTTPSettings`. MCP uses
`WithStreamableHTTPSettings` instead of `WithStreamableHTTPClient`; invalid
settings fail Start, and transport Close releases its owned idle pool. Custom Do, transport, proxy, roots or
tracing clients are no longer silently accepted. Roots/client certificates belong
in TLSConfig and are applied to the pinned safe transport; custom Do/proxy/dial
ports are unsupported. TLSConfig is cloned. Referenced root pools, certificate
slices/private keys and callbacks remain immutable host-owned state.

`agents.NewClient` now returns `(*Client, error)`. Each client owns one reusable
pool for its REST/SSE lifetime. At disposal stop initiating calls, then invoke
`CloseIdleConnections`. `httptool.AsToolsWithCleanup`, `web.AsToolsWithCleanup`,
`document.AsToolWithCleanup`, `openapi.ParseURLWithCleanup` and
`graphql.IntrospectWithCleanup` return `(tools, closeIdle, error)` (document
returns a single tool). The closer releases only owned idle resources and leaves
active calls unaffected; it is not terminal Close. Compatibility factories keep
the same simpler return shape and automatic bounded idle expiry. One-shot
`web.ScrapePage` disposes its own pool on return. No caller HTTP client is accepted
or closed. Pools retain at most 32 idle connections overall, two per host, for
at most 90 seconds. These are idle bounds, not request/concurrency limits.

Timeout must be nonnegative. Zero leaves the request under its context deadline;
HTTP probe tools preserve their 30-second default. Agent SSE has a separate
logical StreamPolicy deadline; a positive HTTP timeout also limits each response
body lifetime. TLS handshake and dial have finite transport deadlines.

DNS lookup and sequential attempts share a total dial deadline. Validate every
resolved IP before the first attempt; then dial only those addresses, without
re-resolving. Allocate each attempt a share of remaining time and stop on caller
cancellation. Host deny precedence, DNS pinning and redirect credential/effect
policies remain. No environment proxy bypass is introduced.

Stream byte caps retain stop-after-budget semantics. An exactly exhausted cap
without EOF on that Read produces a limit error on the next nonempty Read; the
reader cannot prove exact EOF without probing past the cap. Empty reads consume
nothing, and cancellation takes precedence. No speculative EOF read is made.

## Concurrent session rebinding (R08)

Session registry and binding now occupy one immutable configuration snapshot.
Rebind computes the target outside configuration locks, then atomically validates
against the current binding and publishes with CAS. Incompatible targets leave
configuration and state unchanged. Public Binding returns a detached clone.
Execute and RunCall capture one registry per call; manifest/completion policy and
dispatch stay on that registry even if a host callback rebinds the session. Later
calls see the replacement. Existing calls are not canceled or migrated.

ExportSnapshot captures one binding and copies state-map slots, then invokes
codecs/MarshalJSON without holding state/configuration locks. ExportCheckpoint
uses that snapshot's binding for its outer metadata, so the two cannot diverge.
Map replacement on ImportSnapshot remains atomic; decode callbacks run before
replacement without locks. Snapshot export does not deep-copy host values; their
referenced data must remain immutable while encoding, or host-synchronized by the
codec. Registry configuration must remain stable after setup; codec registrations
are finalized automatically by NewSession as described below.
The exported state map is a captured set of slots, not a transaction across
external mutable objects or concurrent handler effects.

## State codec lifecycle and checkpoint scope (R09, D07, D08)

Register every state slot before constructing the first session. `NewSession`
validates the run policy and registry binding, then freezes the supplied shared
`StateCodecRegistry` before computing its state schema digest. Registration and
freeze linearize under the builder lock: a concurrent registration either becomes
part of that fixed schema or returns `ErrStateCodecRegistryFrozen`. All registrar
paths enforce this rule, including required slots and prototype codecs. Explicit
`Freeze()` is nil-safe and idempotent; schema changes require a new builder.
Invalid policy/registry constructor validation leaves an unfrozen builder mutable.
`NewSessionFromCheckpoint` finalizes the builder before checkpoint compatibility
and hydration checks, even if those later checks fail.

Required slots must be present when exporting and importing. Missing or untyped
nil required values fail export. A registered non-nullable slot encoding JSON
null also fails export; nullable slots may roundtrip typed nil through their codec.
The library fixes slot metadata, not arbitrary host callback behavior. Custom
codecs must roundtrip their values, use stable schema IDs, and support concurrent
calls; referenced callback state remains host-owned.

`SessionCheckpoint` is a state-plus-binding checkpoint. It does not save RunPolicy,
maxCalls, consumed call-attempt count, dependencies, StateStore contents, external effects,
or a workflow continuation. Restoring supplies current host authority/configuration
and starts fresh in-memory counters. Hosts must enforce durable budgets and restore
workflow position themselves before dispatch.

Set/Get synchronize map slots and preserve BYOT references. Pointers, maps, and
slices remain aliased; the host must synchronize mutation or keep values immutable
during access/encoding. Codec/MarshalJSON callbacks run outside state/configuration
locks and may reenter the session. Import replaces the map only after successful
hydration; callback writes and external side effects are not rolled back when
hydration fails. This is not a deep-copy or transactional callback contract.

## RunPolicy snapshots and call admission (R10, D06)

`WithRunPolicy` copies AllowedTools and CatalogRequiredTools at capture and at each
materialization. NewSession owns independent execution/options snapshots. Mutating
original slices after option creation or construction cannot change validation or
admission; reused options are independent. Do not mutate a slice concurrently
with the initial WithRunPolicy capture itself. Dynamic policy updates are not
provided; construct a new session with current authority.

Clear break: replace `RequiredTools` with `CatalogRequiredTools` for presence in
the visible session catalog. Missing names fail construction with
TOOLS_CONTRACT_MISSING before codec freeze. It never restricts calls. To preserve
the old RequiredTools-only whitelist behavior, migrate that list to AllowedTools.
Catalog requirements may be outside AllowedTools and ForcedTool. ForcedTool still
must be in AllowedTools when the latter is nonempty. Registry/View capability
policy remains an independent boundary.

Replace `WithMaxSteps`, `Track().MaxSteps()`, `Track().ExecutionCount()` with
`WithMaxCalls`, `Track().MaxCalls()`, `Track().CallAttempts()`. Error APIs are now
ErrMaxCallsExceeded / CodeMaxCallsExceeded / NewMaxCallsExceededError, and the
serialized code is `MAX_CALLS_EXCEEDED` instead of `MAX_STEPS_EXCEEDED`. Update host
wire-code switches/readers; the obsolete code no longer maps to the budget
sentinel/classification in the new reader.
Zero remains unlimited; a negative limit now fails construction.

Accounting order is unchanged: nil registry and RunPolicy rejection consume no
attempt; otherwise atomically increment CallAttempts and reject attempts above the
limit. Budget-rejected attempts remain counted. Environment binding, registry
capability/policy, argument validation, cancellation, tool failure and result replay
all occur after admission and consume that attempt. Internal middleware retries
consume one admission; each nested Session.Execute and each fresh RunCall consumes
its own admission. At most maxCalls attempts can pass this gate under concurrency.
This counter does not measure handler effects or successful results and does not
model LLM/agent iterations. Checkpoints do not persist it; the host enforces durable
budgets and supplies current policy/limits when restoring.

## Empty delivery and result algebra (R11, D16)

Empty results now always build their declared success envelope. Audience, delivery
class and metadata survive direct execution, registry delivery, RunCall and result
replay. Typed Value, effects and controls remain present; Data and MIME are absent.
No fake JSON zero value is created.

| Declaration | Typed/wire payload | Effects | Controls |
|---|---|---|---|
| Ordinary Value | Value plus JSON encoding | Allowed | Allowed |
| Nonempty Raw | Value retained; Raw overrides wire only | Allowed | Allowed |
| Empty | Value retained; wire absent | Allowed | Allowed |
| Noop | Value retained; wire absent | Forbidden | Allowed |

Empty and Noop are mutually exclusive. Either with nonempty Raw is invalid; Noop
with Effects is invalid. RawMimeType without nonempty Raw is invalid. Raw defaults
to application/octet-stream; an empty Raw slice alone is not a wire override.
Use Empty for a result without a payload. Delivery class defaults to structured
and audience to model when unspecified, including results without wire bytes.

Clear break: NewNoopToolResult no longer serializes a zero TResult
into wire bytes; its typed Value remains available. DecodeOutcomeAs returns
an available typed Value; when absent, neither status has wire bytes to decode.
ResultValidator is skipped for Empty/Noop; EffectValidator and
Postcondition still run for valid declarations. Generic result chunks and replay
validation reject contradictory flag/wire/effect combinations too. An error
result cannot declare Empty/Noop success status. Invalid
result algebra is an INTERNAL ResultContractError with Kind result_algebra and
cause; the handler may already have caused effects, so this never permits blind
redispatch. Noop is a host declaration, not proof of absence of external effects.

Existing persisted results with Empty/Noop plus old wire bytes are incompatible.
Typed values without wire bytes remain supported, including MCP protocol DTOs.
Version host cache partitions/codecs or migrate records deliberately. Do not
evict durable completed-operation/idempotency records to force a new dispatch;
use trusted host migration/reconciliation while retaining the operation boundary.

Generated nested json.RawMessage output now maps to an unrestricted JSON schema
(true), accepting object/array/string/number/boolean/null instead of object only.
Arguments retain their documented object default. Explicit host SchemaRegistry
mappings override defaults at both boundaries; defaults are applied to a local
schema-map copy and never mutate the shared registry. WithOutputSchema overrides
automatic output inference. Top-level json.RawMessage/JSON marshaler/WireJSONResult
wire shapes remain uninferred: set WithOutputSchema when shape constraints matter.
Arbitrary custom nested marshaler shapes also need an explicit schema; reflection
cannot infer them from storage types. Only JSON wire bytes are schema-validated;
typed Value stays BYOT and may intentionally differ from Raw representation.

Reflective snapshot cloning copies supported exported struct fields and
pointer/map/slice/array values, preserving cycles and repeated references within
one cloned value. Opaque private fields, functions/channels and pointer-map-key
identity remain host-owned. Overlapping subslices with different lengths and
separately cloned components do not promise a shared alias graph, backing-array
capacity or universal identity preservation. Delivery is not a universal deep
copy of every host object: keep borrowed typed values/opaque data immutable while
they are consumed, and use a complete trusted ResultCodec for persistence. No
JSON roundtrip is introduced to simulate cloning arbitrary BYOT types.

## R12 / D13 / D15 — explicit reuse and replay source

Clear break: NewResultCache takes `(store, eligibility, partition, codec, maxBytes)`.
CacheEligibility is `func(context.Context, PreparedCall) (bool, error)`; it runs
on every currently authorized attempt, including hits. False invokes the handler
without partition/storage/codec work. Idempotent and ReadOnly are classifications,
not freshness proofs. Do not migrate by blindly returning true for every tool.
Approve reuse only for a domain result whose freshness, principal/scope and relevant
input/dependency revisions are bound to the trusted partition/store expiry policy.
Concurrent misses may dispatch twice; use OperationProfile for durable effect
identity/recovery rather than the cache.

Business-error terminal chunks keep their delivery and RunCall classification;
they are never persisted. Missing/multiple terminals, limits and host cache
infrastructure errors now fail INTERNAL, nonretryable and not input-correctable.
Causes remain available to host diagnostics. After dispatch such failure does not
prove that an effect was rolled back or authorize automatic redispatch.

Clear break: remove CacheReplayMetadata / `toolsy.cache_replay` boolean checks.
ReplaySourceMetadata (`toolsy.replay_source`) stores a string:
ReplaySourceCache (`result_cache`) for reusable data, or ReplaySourceOperation
(`completed_operation`) for an already-completed durable logical operation.
Reducers skip effect application for either source. Current CallID/ToolName remain
rebound on delivery. Policy configuration cannot set this reserved key; repeated
metadata overlays retain it and keep private replay audiences private.
A trusted profile stamps the actual source, overwriting any stored source. The
marker is provenance, not a permission grant or authenticity proof for arbitrary
custom-tool output.

Old encoded successful records may retain their old metadata, but current replay
adds the new authoritative source. Host reducers must use the new key; do not
interpret absence of the old key as a fresh dispatch. If custom codecs reject the
new metadata or rely on a boolean schema, deliberately migrate those records.
Never evict durable completed-operation records to force redispatch. Cache entries
may be invalidated under the host's ordinary reuse policy; journal outcomes require
fenced reconciliation. Codec and opaque host value ownership remain unchanged.


## R13 / D20 / D21 / D22 — MCP correlation and bounded discovery

Clear break: WithLogger becomes WithStdioLogger. Replace the old combined
WithStdioMaxStreamBytes / WithStreamableHTTPMaxStreamBytes options with
WithStdioLimits / WithStreamableHTTPLimits using TransportLimits. Frame/queue/
in-flight/lifetime bounds have separate meanings. Default frames are 1MiB,
retained outgoing bytes 16MiB and in-flight requests 64; lifetime zero is unlimited.
Set MaxLifetimeBytes explicitly if process/per-response total traffic must be
bounded. Set a larger frame cap explicitly for large responses or requests.
Negative/nil transport/client options fail early. FrameByteCapTransport.MaxFrameBytes
replaces the old StreamByteCapTransport.MaxStreamBytes custom facet.

Clear break: Client.ListTools becomes ListToolsPage; it never updates tool routing
authority, even for a single complete page. Use DiscoverTools for a full typed
validated snapshot and transactional publication, or Discover (formerly GetTools)
for generated proxies. Full discovery bounds aggregate raw bytes/items/pages/
cursors before accumulating descriptors/compiling schemas; input and output schemas
must validate before authority publication. Original page cache hints remain in
ToolDiscovery.Pages, with no fabricated aggregate TTL. Update manual HTTP callers
that previously relied on the single-page ListTools side effect. Host authorization
and cache freshness are still separate from remote descriptors/mapper properties.

Locally canceled subscriptions retain bounded ID correlation (64 active/1024
tracked IDs/one minute by default). Capacity reserves eventual retirement space;
new Listen fails at saturation instead of evicting live/retired routes. Late ACKs
and notifications for retained locally canceled IDs do not alter generations or
cancel unrelated subscriptions. Outside the configured time window the strict
unknown-ID policy applies again. Truly unknown/malformed traffic remains a protocol
error. Configure SubscriptionLimits for the trusted deployment's cancellation and
latency envelope; custom transports must not reuse IDs in that window. This is
bounded correlation, not a claim to accept arbitrary infinitely delayed traffic.

See [MCP limits and minimal transport](../mcp/README.md#transport-discovery-and-subscription-limits)
for supported callback, queue, overflow, framing and cancellation guarantees.

R13 publication checks cancellation under the authority lock immediately before
starting the synchronous commit. Once `ReplaceToolHeaderBindings` starts, later
cancellation cannot roll back a successful commit. This trusted facet must replace
atomically (errors retain old bindings), finish in bounded time, and must not
reenter Client methods. Holding the authority lock preserves matching client and
transport snapshots; a reentrant transaction would require a different port
contract. Descriptor mapper callbacks retain their existing reentry behavior.
Transport queue/count refusals expose `TransportLimitError` with
`ErrTransportLimitExceeded`; frame failures preserve `ErrReadLimitExceeded`.

## R14 / D28 — host descendants and failed cleanup outcomes

Unix host execution now launches the runtime directly and uses a separate /bin/sh
anchor with a private hold pipe. It stops owned group descendants before workspace removal
on every completion path, signaling only while the group leader is unreaped. Guest start errors and exit statuses retain Go exec semantics. Escaped groups and
non-Unix process-tree termination are not covered. Cleanup confirmation is bounded;
failed confirmation retains the workspace and exposes CleanupError.ResourceID.

exectool failed execution retains the sandbox-returned RunResult in RunOutcomeError
reachable through errors.As; the cause and error classification remain inspectable.
There is no successful result chunk on failure. A returned zero/partial result is
not automatically complete, and cleanup failure must not trigger a blind retry.

## R15 / D29 — exact Starlark output and focused Docker configuration

Starlark RunResult.Stdout now retains every printed newline on both exit0 and
exit1, including trailing empty lines. Consumers choose presentation trimming.
The internal finalizer trim flag is removed across all sandbox adapters.

Docker WithClient accepts exported Client, the focused adapter lifecycle port;
its mandatory context/reader/capability contract is in the adapter README. The
constructor-validated policy is the only source of output/log-timeout bounds.
Starlark fs.read limit errors intentionally remain guest exit1 with bounded stderr;
stdout/stderr collection overflow remains an infrastructure/output error.
