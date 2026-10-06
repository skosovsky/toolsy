# MCP clear-break migration — work in progress

The user confirmed that clear break was authorized in the original task. This
migration does not introduce a new protocol profile or activate deferred cards.
It reconciles unfinished local files with the existing strict MCP contracts.

## Contract before implementation

- `PrepareRequest` exposes identity before delivery. No `Call`/`Request` shim:
  request-correlated state is registered before `Deliver` can send bytes.
- Notifications use `NotificationHandler` and `json.RawMessage` everywhere.
- HTTP is POST-only with one configured endpoint and request-scoped SSE.
  GET endpoint discovery, session/reconnect and server request dispatch are not
  supported capabilities; no obsolete positive fixture may resurrect them.
- Discovery replaces initialization. Negotiated capabilities, scope, exact
  correlation, safe URLs, bounded payloads, cancellation, close/process cleanup
  and lossless validated values remain executable requirements.
- All old test scenarios receive an explicit disposition: retained/ported,
  redundant with exact assertion evidence, or obsolete behavior replaced by a
  current negative scenario. Missing invariants block acceptance. Removing a test
  merely because it fails compilation is not a valid disposition.

## Recoverable source copy

Before clear-break removal, all 13 unfinished MCP source/test files were copied
to `/private/tmp/toolsy-mcp-pre-migration.MHKAqL`. This local recovery copy is not
part of the build, a contract implementation, or evidence that checks pass.

Removed runtime files: `mcp/transport_sse.go` and `mcp/httpclient.go`. Their old
GET/endpoint API is not adapted with aliases; the existing Streamable HTTP
implementation is the current runtime target. Shared scanner cap now belongs
to `mcp/transport.go`; stdio consumes that definition.

## Coverage and verification

Per-scenario independent inventory is maintained in
`task35-mcp-migration-audit.md`. The old tests remain under migration; production
and test builds, all-module race/lint and both final audits are not yet accepted.
Previous root execution audit results do not certify these new MCP changes.

Production-only `go build ./...` in MCP passes after removal of the obsolete
runtime. Full MCP tests still fail compilation on old test helper/DTO contracts;
none of those files have been silently excluded from the acceptance gate.

Interim correctness audit confirmed MCP-COR-001: current POST SSE dispatched a
pending event at EOF without its terminating empty line. The framing contract
was documented, new terminal regressions failed first, and EOF dispatch was then
removed. `transport_sse_eof_migration_test.go` checks unfinished terminal frames
with no ending/LF/CRLF and unfinished ACK/progress with positive framed controls.
`transport_http_migration_test.go` also checks no GET/endpoint downgrade, private
endpoint rejection before traffic, cumulative SSE limits and caller cancellation.

Scoped diagnostic `go test -race` with the platform's production GoFiles and
these two new test files passes. This temporarily isolates new assertions while
old test fixtures are migrated; it is explicitly NOT a package/all-module test
gate, not a hidden skip, and not final coverage acceptance. Both final audits
and full test/race/lint remain mandatory after every old scenario is reconciled.

## Protocol DTO migration checkpoint

### Subscription URI comparison contract

Preserve the original 14 URI fixtures during the current `subscriptions/listen`
migration. Before fixing the newly exposed false negatives, comparison is defined
as follows: scheme and hostname are case-insensitive; HTTP/HTTPS default ports
are equivalent to omitted ports; percent-encoded unreserved path octets normalize
to their literal form. Reserved escapes remain encoded (hex case normalized),
repeated slashes remain distinct, and dot segments are rejected rather than
resolved into a wider subtree. Query/fragment-bearing subscriptions match only
the same normalized URI, never descendants. Userinfo and non-default ports must
match. These are notification-routing rules, not authentication or filesystem ACLs.
Syntax/scheme normalization follows [RFC 3986 sections 6.2.2–6.2.3](https://www.rfc-editor.org/rfc/rfc3986#section-6.2.2);
dot-segment rejection is this client's stricter boundary, not RFC canonicalization.

All 20 original `protocol_contract_test.go` tests now have current-contract
counterparts. Independent preservation review passed 20/20 and scoped race/count3
passed after closing CP-MCP-003/004 assertion gaps.
This is not acceptance of the full MCP package or the overall goal.

Result fixtures carry the current explicit result tag and required cache fields.
Typed request metadata fixtures carry the current mandatory client identity,
capabilities and protocol marker. Invalid extension keys are tested with otherwise
valid metadata and the exact key-validation error, not a missing-field failure.
The original initialization-shaped null-metadata fixture remains a negative
case, and a pure current discovery null-metadata case is added independently.

All original progress-token lexemes and rejected composite/null inputs remain.
Resource-link sizes preserve large integer and exponent lexemes exactly; the
original fractional fixture now rejects on both decode and encode, per the current
integer contract. Unknown resource annotations remain inert lossless extensions,
not a typed API. The original roots-list result fixture becomes a forbidden
outgoing-method check before preparation or wire traffic; no roots service is
restored. Reserved-extension collisions, object/null validation, content variants,
metadata reset and additive capability metadata remain executable assertions.

## Client25 migration continuation

All 25 original `client_test.go` tests are independently reconciled (25/25).
Its capture transports use the real prepared/pending fake transport and explicit
Start/Prepare/Deliver lifecycle, not a legacy Call adapter. Post-discovery fixtures
explicitly arrange valid metadata, capabilities and client state so missing
metadata/capabilities cannot mask the intended failures.

Original read-limit/default/2048-cap checks, operation subjects, annotations and
manifest mapping, cancellation/deadline/timeout and joined-interrupt scenarios
remain. Removed handle-result helpers map to current common error boundaries and
`buildToolResultChunk`: the original permission-denied payload remains lossless
with a distinct typed remote-error envelope instead of flattened validation JSON.
The original unsupported discovery selection and `/workspace` input remain as
explicit exact-version and forbidden-roots negatives. Initialized notification
failure is replaced by absence of that obsolete notification. Cancellation retains
the exact raw req-1 identity and detached bounded context with trace propagation.

Migration exposed MCP-COR-005 P2: `Connect` skipped cleanup after partial Start
failure. Contract-before-code ownership documentation and Close-on-Start-failure
fix are independently verified by a public resource-acquiring transport proof:
one Close, released resource, preserved original cause.

MCP-COR-006 P2: resources, resource-templates and prompts list APIs skipped typed
read-limit mapping. The new public regression table was RED for all six
default/custom-cap cases before fixing these three boundaries. Common mapping
now retains the original diagnostic cause by cloning the typed error; bounded
Reason/Error remain redacted. Three joined-interrupt cases stay interrupts.
Independent public proofs and scoped race/count5 close the finding, including a
secret-bearing-cause non-disclosure check. Client25 scoped race/count3 passes.

CP-MCP-008 is independently closed: both exact stdio invalid-input fixtures now
call a parent-visible real peer send spy and assert zero unsolicited replies.
StdioContract completeness is 15/15, not merely a green subprocess run.

Two original suites remain: adversarial_contract (51 tests) and transport_sse
(19 tests). Full MCP `go test ./...` is still RED on obsolete adversarial symbols;
neither suite is excluded from acceptance. All-module gates and both final
whole-candidate audits remain pending.

## HTTP and stdio lifecycle continuation

All five original `transport_http_test.go` tests have current-contract counterparts;
independent preservation and cause-assertion review passed 5/5. Scoped race/count3 passed after
CP-MCP-007 was corrected: both server-request fixtures assert the specific
forbidden-request cause, and the legacy endpoint fixture asserts JSON syntax
failure rather than accepting any EOF failure. Session advertisements retain
their original values but remain inert. Discovery carries the current version
from its first POST; a second explicit POST sends no session/resume headers.
Close sends no DELETE/GET/legacy notification. Original event IDs, roots-list
payloads, invalid version, `/message` and HTTP404 fixtures remain executable;
request counts exclude reply, reconnect, endpoint-switch and blind-retry paths.

All 15 original `transport_stdio_contract_test.go` tests and its subprocess helper
are migrated; independent preservation review passed 15/15. Scoped race/count3
passed. Preparation and delivery are explicit, current discovery replaces
initialization, and vendor test requests include required metadata without
changing their original payloads or 4 MiB blocked-write scenario.

Process exit/reaping, nonzero exit cause, live stdout closure, inherited descendant
stdout/stderr, full descendant-tree close, four malformed correlated responses,
stderr-volume safety, queued cancellation, writer state and callback ordering
retain their original assertions. The subprocess helper exits explicitly rather
than allowing the Go harness to append `PASS` to protocol stdout.

Two obsolete reply scenarios become fail-closed checks: original invalid UTF-8
and malformed server-request bytes produce their specific `InvalidPayloadError`
causes, no result, and process cleanup; no protocol-error reply handler is
restored. Peer tests separately assert zero outgoing replies to forbidden input.

Active write cancellation follows the existing writer-verdict contract: the
caller gets cancellation but cannot terminate unrelated requests. The old global
shutdown expectation is replaced by a not-closed assertion; explicit `Close`
owns process-tree/writer cleanup. Queued cancellation still cannot kill another
active write; a new healthy request can prepare/deliver before explicit close,
then settles with `ErrTransportClosed`. Runtime was not changed to satisfy these
legacy expectations. Close and cleanup checks were not dropped.

Three original suites remain: adversarial_contract, client_test and transport_sse.
Full MCP compilation/all-module gates and both final whole-candidate audits
remain mandatory. File-selected race diagnostics are not acceptance.

## Client features migration checkpoint

All 13 original `client_features_test.go` tests have current-contract counterparts;
independent preservation review passed 13/13. The original 14 URI fixtures and their
expected outcomes remain unchanged. Scoped race/count3 passes after the migration.

- Resource subscription uses `Listen` with effective-filter ACK and request ID,
  rather than the removed subscribe RPC. Original null/array/number/string terminal
  fixtures retain `InvalidPayloadError` checks and require settled cleanup. Extended
  terminal metadata retains trace and extension values with the current result tag
  and subscription ID.
- Concurrent close retains 32 callers, one transport close, invalidation closure,
  and adds active-subscription Done/Event closure. The removed global log channel
  is not reintroduced.
- Stale proxies use an ACKed tools-list filter with exact subscription ID.
  Contradicting capability now also rejects `Listen` before any extra wire traffic.
  Invalid metadata is tested with an active ACKed route plus a routable positive
  control, zero generation/events and the specific missing-metadata failure.
- Old required-task descriptor metadata is lossless inert `Extra`, not a claimed
  remote-task capability. Missing/null schemas and invalid names remain failures.
- Null and structured log data, severity and logger survive request-correlated
  delivery to the host logger. No active request/retired request means no logging;
  legacy global logging advertisements do not create a channel or authority.
- Tagged prompt resource content and its metadata remain intact.

Independent URI correctness review found MCP-COR-004 (P2): parsing erased an empty
fragment delimiter, widening exact-only routing to a subtree. The fix preserves
raw fragment presence for both equality and descendant checks. The auditor's 28
adversarial URI cases pass race/count5, including empty/absent fragment identity,
encoded separators/dots, userinfo/origin/ports, query and opaque URI boundaries.

Full MCP `go test ./...` remains RED at compilation of obsolete adversarial APIs.
Five original files still need migration: adversarial_contract, client_test,
transport_http, transport_sse and transport_stdio_contract. No full gate excludes
them; diagnostic file-selected checks are not acceptance.

## Peer and stdio migration checkpoint

All 15 original `peer_test.go` tests and all 11 original
`transport_stdio_test.go` tests now have current-contract counterparts. The
independent inventory remains an immutable original-assertion ledger; this
checkpoint records implementations, not final completeness approval.

Peer tests retain exact/string/numeric/large/exponent ID checks, integral error
code lexemes, fractional/error-object rejection, all malformed-envelope/params
fixtures and ten-pending close cleanup. Incoming server requests never dispatch
notification callbacks or send a response. Explicit obsolete replacements:

| Original peer test suffix | Current counterpart suffix / disposition |
|---|---|
| RequestAndNotificationWithSameMethodAreDistinct | Same name: one notification; server request rejected, no reply. |
| UnknownIncomingMethodReturnsMethodNotFound | UnknownIncomingMethodFailsClosed |
| EmptyMethodUsesNormalUnknownMethodSemantics | EmptyMethodFailsClosed; request and notification rejected. |
| HybridRequestResponseEnvelopeIsRejected | Same name; invalid hybrid rejected without a reply. |
| FractionalIncomingRequestIDIsRejectedWithoutDispatch | Same name; rejection, no callback and no reply. |
| MalformedIncomingRequestsReceiveInvalidRequest | MalformedIncomingRequestsFailClosed; every original fixture retained. |
| NonObjectAndInvalidJSONReceiveProtocolErrors | NonObjectAndInvalidJSONFailClosed; array/null/syntax fixtures retained. |
| CloseCancelsAndWaitsForIncomingRequestHandlers | CloseAfterForbiddenIncomingRequest; forbidden worker lifecycle is absent, close rejects later outgoing work. |

Other peer names are unchanged. Params validation additionally checks that all
four original non-object values cannot reach a registered notification handler.

| Original stdio test suffix | Current counterpart suffix / disposition |
|---|---|
| Start_CancelContext | Same name: pre-cancelled Start creates no process; no first-line wait. |
| StderrLongLineDoesNotBreakStart | Same name: 70KB stderr precedes a request-correlated successful reply. |
| StdoutExceedsMaxStreamBytes | Same name: ten 300-character padding frames exceed cumulative 2048-byte cap. |
| Call_CancelUnblocksPending | PreparedCancellationUnblocksPending; cancel after settled WasSent=true. |
| CallAfterStartContextCanceled | PrepareAfterStartContextCanceled; successful new request after Start context cancellation plus rejected pre-cancelled caller. |
| finishStdioCallResponse_CancelOverReadLimit | CallerCancellationOverReadLimit at current client error boundary. |
| call_CancelOverStreamLimit | PreCancelledPrepareOverTerminalReadLimit at current transport boundary. |
| finishStdioCallResponse_CancelOverStaleStream | CallerCancellationOverStaleStream |
| finishStdioCallResponse_LimitWithoutCancel | ReadLimitWithoutCancellation; client maps raw limit to typed validation with exact subject/cap. Raw transport sentinel remains asserted by cumulative test. |
| finishStdioCallResponse_InterruptInChainOverReadLimit | InterruptInChainOverReadLimit; wrapped/joined interrupt remains the original error. |
| streamLimitErr_InterruptOverReadLimit | TerminalInterruptOverReadLimit; closing transport unblocks prepared Await with both original error-chain causes. |

No `Request`/`Call`, first-line option, incoming request callback, old transport
response helper or runtime compatibility alias was restored.

Two more regressions were reproduced during this migration:

- Peer accepted an empty-method notification even though the existing
  `Notification.UnmarshalJSON` contract rejects it. The dispatcher now requires
  a nonempty method; `EmptyMethodFailsClosed` failed before the fix.
- Stdio dispatched Scanner's partial final token despite its byte-limit error,
  masking cumulative overflow as JSON `unexpected EOF`. The reader checks the
  Scanner error before dispatch; `StdoutExceedsMaxStreamBytes` failed before the
  fix. A partial frame cannot complete a request.

The production files plus the two migrated suites and the two new HTTP/SSE
regression suites pass scoped `go test -race`. This remains a diagnostic only:
the remaining original suites still require migration, and full-package gates
and both final audits are pending.

The independent correctness auditor repeated these scoped suites with `-race
-count=3` and independently reproduced the original partial-token/read-limit
precedence using an external reader proof. It confirmed both fixes against the
current DTO/runtime contracts and found no local regression. This targeted
review is recorded in `task35-mcp-correctness-interim.md`; it does not replace
the required full-candidate correctness/completeness audits.

## Current fixture and client-contract checkpoint

`test_support_test.go` now uses the real prepared/pending lifecycle. Preparation
is captured separately from delivery; hook execution and sent-state publication
occur at Deliver. Abort, cancellation, completion hooks, exactly-once cancel
ownership and Close use the existing peer lifecycle. Captured params/ID/result
are detached, and incoming peer writes are independently recorded. Five new
`test_support_migration_test.go` checks cover inert preparation/private copies,
abort, cancellation ownership, 64-way completion race and close unblocking both
prepared and delivered requests. Initialize/result helpers, protocol-version
setters and incoming request handlers were removed, not aliased.

All 23 original `client_contract_test.go` tests now have counterparts:

- Ten existing valid test names remain: stalled cancellation wait, terminal
  non-cancellation, prompt name, missing capability, typed structured result and
  envelope, schema failure, exact large numeric minimum, remote error envelope,
  active-ID single cancellation and fractional/monotonic progress.
- Connect uses strict self-describing discovery without roots authority and no
  initialized notification; incompatible supported versions fail without fallback.
- Seven removed filesystem-root normalization tests retain every original path
  and URI input as an explicit current no-authority negative. They do not claim
  that the client still canonicalizes roots. Both inbound roots/list rejection
  and outbound rejection before preparation are checked with zero peer writes.
- Base ping is rejected before/after discovery; ping/roots malformed `_meta`
  fixtures remain rejected with no preparation/delivery.
- Roots snapshot metadata ownership is tested through explicit host-owned raw
  inputResponses, without installing a roots service or domain identity types.

Tagged/cache-bearing DTO fixtures are explicit current values; no automatic
defaulting/normalizing fixture adapter hides missing required fields. Cancellation
checks now wait for the bounded detached single-owner send and copy notification
captures under lock: an immediate unsynchronized read raced with the other Await
path in the original fixture. Exactly one notification with the original ID is
still mandatory.

Migration reproduced a progress lifetime regression: a late-after-terminal
notification reached the consumer. The contract was fixed in README before code:
prepare, register Completion, then Deliver; terminal retirement serializes with
nonblocking enqueue, and accepted buffered progress drains before the result.
Custom pending without Completion aborts before delivery. The unused internal
requestWithProgress JSON-rewriting helper was removed; current typed Meta handles
the token. `progress_terminal_migration_test.go` deliberately holds Await until
after a late frame to prove wire-terminal retirement, accepted progress ordering
and no-completion rejection before bytes.

That regression also exposed empty MCP success with MIME but no bytes, which the
core chunk contract correctly rejects. Empty tool/resource chunks now keep their
typed value and envelope with empty MIME; three resource fixtures (zero contents,
empty text, empty blob) pass through a real core proxy. Consumer abort keeps both
the stream marker and original cause, tested on progress and terminal result.

Independent correctness recheck of this checkpoint passed the selected suites
with race/count5. The platform-selected production files plus all 27 currently
compilable test files passed scoped race/count1. Seven original files remain
explicitly excluded from this diagnostic only: adversarial_contract, client_features,
client_test, protocol_contract, transport_http, transport_sse and transport_stdio_contract.
The actual `go test ./... -run '^$'` still fails on those obsolete fixtures.
No full-package/all-module gate, final audit or overall completeness is claimed.

## SSE scenario reconciliation checkpoint

All 19 original SSE tests are independently reconciled against the POST-only
transport contract. Original private-IP fixtures, status 500/400/204 responses,
bounded cancellation, 50×200-byte stream volume with a 2048-byte cap, cancellation
and deadline precedence over read limits remain checked. Stream-volume data is
represented by valid SSE comments; original endpoint and malformed-data frames
are checked separately for exact JSON syntax failure without follow-up traffic.
No GET/endpoint runtime, session replay or compatibility shim is restored.

The pre-cancelled result-boundary case tests the private mapper; the paired actual
Prepare test proves cancellation before request ID allocation. Terminal interrupt
precedence is exercised through a real prepared pending request. Scoped race/count3
passes independently and locally. The preservation ledger now reconciles 146/197
original tests plus the helper; the remaining 51 are in adversarial_contract_test.go.
Full MCP and all-module gates, and final independent audits, remain pending.

## Adversarial scenario checkpoint

37/51 original adversarial scenarios are independently reconciled across the
peer, HTTP lifecycle, SSE framing, client and DTO migration suites. All original
512/256 concurrency bounds, cancellation-range invariants, decorator body/host
ownership fixtures, stream caps, process crash and strict DTO malformed semantics
are retained under explicit current request preparation and delivery.

Removed server-request execution is replaced by concurrent forbidden-request
assertions with zero replies. Removed polling/cursor side effects do not survive
framing migration: empty or unfinished SSE events do not complete a pending request,
while all original line separators, leading BOM and above-stdio-limit SSE line
remain valid. Actual Listen/ACK now authorizes in-flight tools invalidation.
Consumer failure after terminal response preserves stream/consumer causes but does
not emit a cancellation for an already completed request.

Independent and local scoped race/count3 checks pass. Aggregate inventory183/197
plus helper is reconciled;14 originals remain. The diagnostic race of39 platform
test files passes, while actual full MCP go test ./... remains RED on remaining
legacy references. No full gate, final audit or overall requirement acceptance
is claimed.

## Original migration inventory completed

All197 original tests plus the subprocess helper are independently reconciled.
The last14 retain original retry values, server-request identity, ordered request
method, priming/stranded/cancellable stream frames, session advertisements and
forged values, private decorator target,256-byte payload/32-byte cap, concurrent
fatal400 cleanup and notification status/body fixtures. Removed GET resume,
session ownership, initialize and retry scheduling have explicit current-contract
negative dispositions; no compatibility runtime is restored.

MCP-COR-007 P2 was reproduced during the final original stream-cap scenario:
scanner token size hid the bounded reader's read-limit cause. Contract-first
scanner headroom now leaves the actual budget unchanged and preserves typed
mapping. Independent public transport and Connect proofs and selected race/count5
close the finding; below/exact/above-cap and interrupt controls are retained.
CP-MCP-010 exact-cause/media fixture repairs are independently closed.

Actual full MCP go test -race ./... passes (19.375s), with no excluded test files.
The clear-break task34 preflight also passes. These prove current MCP gates, not
whole Task35 acceptance. All-module make test is running; normal make lint failed
before analysis because the installed Go export format is unsupported by the
pinned runner. The unchanged lint gate is being retried using repository-declared
Go1.26.3. Both final whole-candidate audits remain required.

## Final corrected candidate gates

MCP-COR-008 custom-pending consumer-abort cleanup is independently closed.
Invocation child context now releases Await on return; completion retirement,
single cancellation-notification ownership and transport reuse remain intact.
The regression hides all optional cancellation/delivery/ownership interfaces,
checks completion/route cleanup and a subsequent healthy call. Both independent
auditors rechecked the final runtime and test-only lint changes.

Final full all-module `make test` (race, session93922), `make lint`
(every module 0 issues, session87586) and task34-preflight (session90040) pass.
Lint uses the repository-declared Go toolchain and pinned runner with unchanged
rules; temporary writable caches and parallel-runner mode address host cache
permissions/toolchain/lock failures, not code exclusions. Original inventory
remains197/197 plus the subprocess helper. No release or issue write performed.
