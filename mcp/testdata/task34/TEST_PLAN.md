# Task34 fixture and adversarial test matrix

These fixtures freeze the MCP `2026-07-28` contract used by task34. Files under `official/` are positive golden wire examples derived from the final schema. Files under `adversarial/` are negative inputs or ordered frame sequences. `duplicate-result-key.json` is intentionally syntactically ambiguous at the object-model boundary: a strict decoder must reject the duplicate key instead of accepting last-write-wins.

The byte-exact authoritative schema snapshot is [`schema/mcp-2026-07-28.schema.json`](schema/mcp-2026-07-28.schema.json). It comes from the official [`modelcontextprotocol/modelcontextprotocol`](https://github.com/modelcontextprotocol/modelcontextprotocol) repository at immutable commit [`271ecc9accafdd9b83a3c869fa67c22953b2af80`](https://github.com/modelcontextprotocol/modelcontextprotocol/commit/271ecc9accafdd9b83a3c869fa67c22953b2af80), path `schema/2026-07-28/schema.json`. Its SHA-256 is `ef70b61f99b6d2e5e3b46863822eab08dff6a45bedc7a08914e0e5b133f40203` and its size is 181474 bytes. [`schema/PROVENANCE.json`](schema/PROVENANCE.json) records the source and enumerates every one of the 19 wire fixtures that cites this artifact as its contract basis.

`TestTask34OfficialSchemaArtifactHasImmutableProvenance` recomputes the digest, checks the pinned revision/commit/path/size, verifies the 2020-12 dialect and revision-specific definitions, and resolves every cited fixture. `make test` includes this test, so changing either the schema or provenance without deliberately updating the hard-coded trust anchor fails the gate.

All 19 wire fixtures plus the two provenance artifacts are inventoried; the wire fixtures are decoded or exercised by `fixture_contract_test.go`. Transport, peer, subscription and schema behavior is paired with focused AAA tests in the corresponding task34 suites. `make test` runs this executable subset and scans production Go files before any release action.

## Cycle-1 decisions applied

1. `RequestMeta.clientInfo` is optional in the general protocol but mandatory in toolsy's stricter self-describing request policy. The emitter owns it and caller extension maps cannot forge it.
2. Malformed result `serverInfo` is dropped by `ResultMeta` and never affects authorization or discovery behavior.
3. Request-scoped logging is opted into with `RequestMeta.LogLevel`; HTTP validates stream provenance and the client emits only bounded structured diagnostics. No session-wide capability or `logging/setLevel` exists.
4. `ExtensionRegistry`/`ExtensionCodec` provide the BYOT seam while unknown declarations stay raw and inert; registration alone does not advertise a capability.
5. `CallTool`, `ReadResourceRound`, and `GetPromptRound` expose explicit MRTR rounds with fresh request IDs; convenience APIs surface `InputRequiredError` and never auto-retry.
6. `Mcp-Name` wording. It is derived for exactly `tools/call`, `prompts/get`, and `resources/read`, but unsafe values use the same Base64 sentinel representation before transport. Minimal doc correction: “decoded header value exactly matches the body value,” not “raw header bytes equal the body string.”
7. Progress/logging transport wording. “Only from the original response stream” is exact for HTTP. Stdio uses the shared byte stream and correlation metadata. Minimal correction: split the statement by transport, as already done for cancellation.
8. Negative legacy grep. Production legitimately retains the strings `Mcp-Session-Id` and `Last-Event-ID` in the request-decorator denylist. Gates must forbid emission/parsing/state, not those denylist literals. The task34 wording has been narrowed; release tests must follow that narrower rule.

## Matrix

| Area | Positive fixtures | Adversarial fixtures | Required assertions |
| --- | --- | --- | --- |
| Discovery/version | `official/discover-request.json`, `official/discover-response.json` | `adversarial/discover-legacy-only.json`, `adversarial/discover-wrong-version-field.json` | Discovery is first; exact `supportedVersions`; cache/meta preserved; no initialize fallback on mismatch, `-32601`, EOF, timeout, malformed body. |
| Request envelope | `official/discover-request.json`, `official/input-retry-request.json` | `adversarial/duplicate-result-key.json` plus generated reserved-key collisions | Version/capabilities required, client info emitter-owned, non-reserved metadata cloned losslessly, duplicate/reserved keys fail closed. |
| Peer correlation | ordered response fields in official files | `adversarial/peer-sequences.json` | String/numeric IDs do not collide; integral numeric forms correlate losslessly; fractional, unknown, duplicate terminal and inbound server requests close/fail the peer. |
| Stdio lifecycle | discovery and retry request frames | `adversarial/peer-sequences.json`, `adversarial/legacy-forbidden.json` | Discover is first line; serialized writes; pre-delivery cancel has no ID/notification; post-delivery cancel uses exact raw ID; crash/close unblocks and reaps. |
| Streamable HTTP | `official/http-tools-call.json` | `adversarial/http-header-mismatch.json` | POST only; exact protocol/method/name; no session/GET/DELETE/resume; JSON and request-scoped SSE terminal handling; correlated HTTP 400 `-32020` stays typed/in-band. |
| `x-mcp-header` | tool schema and mirrored headers in `official/tools-list-response.json` and `official/http-tools-call.json` | `adversarial/x-mcp-header-invalid-tools.json` | Static top-level string/integer/boolean only; safe integer; RFC `tchar`; case-insensitive uniqueness; malformed siblings filtered; missing/null omitted; sentinel ambiguity and UTF-8 Base64 exact. |
| Result/MRTR | `official/input-required-response.json`, `official/input-retry-request.json` | `adversarial/result-unions.json` | Required tag; `input_required` only for call/read/get; at least one interim field; no hidden retry; current-round responses and byte-exact state; unsupported capability typed. |
| Subscriptions | `official/subscription-sequence.json` | `adversarial/subscription-invalid-sequences.json` | Ack is first per subscription ID; effective filter may narrow request; every event/terminal matches request ID; out-of-filter or pre-ack event fails closed; graceful vs abrupt close distinguished; no hidden re-listen. |
| Cache | cache fields in discovery/tools official fixtures | `adversarial/cache-invalid.json` | Required fields on discover/list/templates/read; zero/public/private round-trip; missing/null/fractional/negative/unknown rejected; no hidden cache refresh. |
| Schema | `official/tools-list-response.json` | schema entries in `adversarial/schema-and-content-invalid.json` | Default 2020-12; supported explicit drafts preserved; local refs bounded; remote refs/unknown dialect/unbounded composition rejected; input root object; output accepts arbitrary JSON shapes. |
| Content | `official/content-result.json` | content entries in `adversarial/schema-and-content-invalid.json` | All tagged blocks and resource-content union round-trip; exact large integer size; Base64 strict; mixed text/blob and unknown type rejected; arbitrary structured JSON preserved. |
| Legacy release gates | none | `adversarial/legacy-forbidden.json` | No old protocol constant, lifecycle DTO/handler, roots/logging-set-level/ping/resource-subscribe path, session state, GET/DELETE/resume behavior, fallback flag or deprecated alias. Header denylist literals are allowed. |

## Additional generated cases

Some properties are more useful as generated tests than static fixtures:

- cancellation/response races with both winners and exactly-once waiter completion;
- randomized valid integral JSON-RPC IDs (`1`, `1.0`, exponent forms, values beyond `2^53`) and fractional rejection;
- bounded fragmentation of cancelled-ID correlation state;
- SSE CR/LF/CRLF, BOM, truncated UTF-8, oversized event/body and terminal event with stream left open;
- `x-mcp-header` sentinel-shaped literal values, empty string, leading/trailing tab/space, CR/LF, non-ASCII, safe-integer boundaries and one-step overflow;
- local `$ref` cycles/depth/fan-out/time limits and unsupported `$schema` identifiers;
- pagination page/cursor limits with cache metadata changing between pages;
- consumer-channel overflow while authoritative invalidation generation still advances;
- repeated `Close`, concurrent `Close`/request/cancel and child-process tree leak checks on Unix and Windows.

## Cycle-5 executable coverage

These are executable tests, not checklist placeholders:

| Generated boundary | Executable proof |
| --- | --- |
| Integral/fractional/string JSON-RPC IDs | `TestTask34RandomizedIntegralRPCIDsCanonicalizeWithoutStringCollision` generates 512 deterministic pseudo-random values and equivalent decimal/fraction/exponent spellings. |
| Cancelled-ID fragmentation | `TestTask34CancelledIDFragmentationRemainsBoundedAndFailsClosed` reaches the exact range cap and verifies fail-closed overflow. |
| SSE framing | `TestTask34SSEAcceptsBOMLineEndingsAndFragmentedFrames` runs LF, CR and CRLF frames with BOM through a 1/2/5/3/8-byte reader; `TestTask34SSERejectsTruncatedUTF8AcrossFragments` rejects an incomplete UTF-8 sequence. Existing request-SSE tests cover EOF, byte limit, cross-request provenance and terminal correlation. |
| Schema cycles/depth/fan-out | `TestCompileAcceptsBoundedLocalReferenceCycle`, `TestCompileRejectsExcessiveSchemaDepth` and `TestCompileRejectsFanOutAboveNodeBudget` execute the compiler boundary. The implementation intentionally uses deterministic depth/node budgets; it has no wall-clock deadline API, so this plan does not claim a flaky elapsed-time assertion. |
| Pagination/cache changes | `TestTask34PaginationPreservesChangingPerPageCacheSnapshots` fetches two cursor pages with different TTL/scope, hashes both typed snapshots, and repeats the two-page fetch through the iterator to prove there is no hidden page reuse. |
| Subscription pressure | `TestTask34SubscriptionOverflowAdvancesAuthorityDuringConcurrentClose` overflows both consumer channels while verifying every authoritative generation increment and racing idempotent `Subscription.Close` calls. |
| Process crash/tree/leaks | `TestTask34StdioCrashUnblocksWaiterAndReapsProcess` and `TestTask34StdioConcurrentCloseKillsDescendantProcessTree` use the current self-describing request contract, assert waiter completion, process-tree death and closure of writer/reader/stderr/process goroutines. Platform-specific `processExists` helpers contain no legacy lifecycle behavior. |
| Snapshot memory bound | `TestComputeSnapshotDigestRejectsOversizedEncodingBeforeCanonicalization` proves the production digest rejects encodings above its byte cap before canonical decoding. |

## Cycle-7 pinned-schema boundaries

`schema_contract_cycle7_task34_test.go` makes the newly reviewed schema details executable instead of relying on prose:

| Boundary | Executable proof |
| --- | --- |
| Resource sizes | `TestTask34PinnedSchemaRequiresIntegerResourceSizes` checks the pinned `Resource` and `ResourceLink` definitions and rejects fractional, string and null DTO sizes. |
| MRTR unions | `TestTask34PinnedSchemaMRTRInputUnionsAreExact` checks the exact three request and response branches, the map-value references, valid examples for every branch and unknown variants. |
| Canonical Base64 | `TestTask34CanonicalBase64RejectsNonCanonicalSpellings` rejects missing/excess padding, whitespace, URL alphabet and non-zero trailing bits for content and resource blobs. |
| Reserved HTTP failures | `TestTask34ReservedHTTPErrorStatusCoherence` binds schema codes `-32020..-32022` to HTTP 400 and verifies that the same JSON-RPC errors on other HTTP statuses remain transport errors. |
| Stdio output bounds | `TestTask34StdioRejectsOversizedOutgoingFrameBeforeWrite` proves an encoded frame at the configured byte limit is rejected synchronously before delivery or process I/O. |

## DoD execution order

1. DTO fixture decode/encode tests.
2. Client lifecycle and capability tests.
3. Peer and stdio race/process tests.
4. HTTP header/SSE/security tests.
5. MRTR and subscription state-machine tests.
6. Feature mapping/content/schema/cache tests.
7. Negative legacy compile/grep/runtime gates.
8. `go test -race ./...`, workspace tests/lint, then non-destructive `make test`. `make release-break` remains a separate intentional commit/tag/push action.
