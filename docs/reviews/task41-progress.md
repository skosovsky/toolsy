# Task 41 execution ledger

Source: `.cursor/tasks/task41-toolsy-review-remediation.md`; baseline:
`58085005dc7e4a0747b2a57af2ef2f62ba2c3750`. Production publish/push is excluded.

## Acceptance protocol

Execute one row at a time. Define the executable contract before implementation;
keep AAA regressions and current documentation together. P1 regressions must fail
on the baseline. Each row needs two independent reviewers who did not implement
it; both inspect requirements, diff and verification without seeing each other's
verdict. Each reports criterion-based completeness, uncovered criteria, defects,
checks, limitations and accepted/rejected. The gate is 100% from both and no
unresolved detected defects. After changes, repeat both reviews against the new
diff. Record acceptance, commit with a short English message, then start the next
row. Commit IDs belong in the following ledger update (a commit cannot include
its own hash). Percentages measure these criteria, not universal bug freedom.

## Sequential checklist

Row 01 is accepted and committed as `4dec511`; row 02 is accepted and committed as `d1619e8`; row 03 is accepted and committed as `322ec4a`; row 04 is accepted and committed as `4f372b2`; row 05 is accepted and committed as `5f80c9e`; row 06 is accepted and committed as `4ddcb4f`; row 07 is accepted and committed as `d1102f6`; row 08 is accepted and committed as `df733dd`; row 09 is accepted and committed as `9a9062a`; row 10 is accepted and committed as `4c36832`; row 11 is accepted and committed as `a492d5f`; row 12 is accepted and committed as `22782ff`; row 13 is accepted and committed as `1180b3a`; row 14 is accepted and committed as `6f6365b`; row 15 is accepted and committed as `e3fc1f9`; row 16 is accepted and committed as `64a40e5`; row 17 is accepted and committed as `a8bf9fb`; row 18 is accepted and committed as `71b49fa`; row 19 is accepted and committed as `f63179a`; row 20 is accepted and committed as `0700cc5`; row 21 is accepted and committed as `e9d80b4`; row 22 is accepted and committed as `8ffd009`; row 23 is accepted and committed as `c682e30`; row 24 is accepted and committed as `f5824b1`; row 25 is accepted and committed as `4b76f5f`; row 26 is accepted and committed as `38314b4`; row 27 is accepted and committed as `77d3a10`; row 28 is accepted and committed as `c7eb470`; row 29 is accepted and committed as `d9170c1`; row 30 is accepted (commit pending); rows 31–40 are pending. Associated D decisions are recorded in their row's
evidence; a retained design needs specific justification and accurate contracts.
Unresolved D work cannot be silently deferred.

| Order | Scope | Acceptance criteria |
|---|---|---|
| 01 | R01 | Origin-bound credentials; no implicit effectful redirect/body replay; public agents/OpenAPI/GraphQL regressions; same-origin read positive control, port/subdomain/downgrade negatives; SSRF unchanged; migration. |
| 02 | R02, D17 | Runtime deny precedence before DNS/dial; explicit exact/suffix/apex syntax and migration; overlap, narrow deny, sibling and normalization fixtures. |
| 03 | R03 | Lossless dynamic/typed input validation; exact bounds/enums at 2^53±1 and int64; handler/prepared/cache identities distinct; explicit duplicate/depth policy. |
| 04 | R04, D12 | Post-handler result/effect/postcondition phase/cause preserved; no argument-repair or blind redispatch; pre-handler correction retained; effect counters and journal outcome fixtures. |
| 05 | R05, D37 (checkout/artifacts) | Isolated tracked-input release preparation; expected manifests only; original branch/index/files survive success/failure/cancel; final artifact graph verified with GOWORK=off in disposable repositories. |
| 06 | R06, D37 (refs) | Only explicit release refs, collision preflight and atomic train; unrelated tags preserved/unpublished; rejected train leaves no partial publication in local bare remote. |
| 07 | R07, D18, D19 | Configuration-lifetime reusable bounded pools and ownership for all three consumers; real customization contract; bounded dial attempts over validated IPs; precise reader cap contract. |
| 08 | R08 | Atomic coherent registry/binding snapshot; callbacks outside locks; concurrent Rebind/Execute/Binding/ExportCheckpoint race-clean; incompatible rebind leaves state intact. |
| 09 | R09, D07, D08 | Stable codec snapshot/digest; explicit late registration behavior; successful export is self-consistent; checkpoint scope and BYOT ownership/reentrancy documented and verified. |
| 10 | R10, D06 | RunPolicy slices copied at capture/materialization; independent reused options; caller mutations/races do not affect sessions; catalog requirements vs call admission and MaxCalls accounting explicit. |
| 11 | R11, D16 | Empty envelope preserves audience/class/metadata/effects/control in direct/registry/cache paths; result algebra/override precedence and exact output schemas audited; cloning limits explicit. |
| 12 | R12, D13, D15 | Business-error terminal delivery/classification unchanged, no cache Put; infrastructure failure not input-correctable; explicit reuse eligibility/freshness and replay provenance; abort/cancel/control preserved. |
| 13 | R13, D20, D21, D22 | Bounded retired subscription correlation; late A leaves B live; strict malformed/unknown policy; frame/queue/discovery aggregate budgets; validated authority publication; minimal transport and configuration contract. |
| 14 | R14, D28 | Owned descendants stop before workspace deletion on success/failure/cancel/pipe paths; bounded cleanup and PID/platform limitations; known outcome plus cleanup diagnostic/reconciliation reference remains truthful. |
| 15 | R15, D29 | Exact Starlark output bytes on all exits; no trim flag; Docker constructor/client port cleanup; guest filesystem vs infrastructure limit distinction documented. |
| 16 | R16 | Explicit UTF-8 rejection or binary representation; public GET/POST fidelity and byte limits; post-effect output failure never promises rollback. |
| 17 | R17, D33 | Declared body representation, plaintext default preserved; only declared HTML converted; raw/final bounds and conversion errors explicit; host owns MIME parsing. |
| 18 | R18, D31 | Cancellable lock wait before store I/O without leaked goroutines/unbounded locks; concurrent one-instance updates preserved; structured escaped scratchpad representation and honest ownership. |
| 19 | R19, D35 (scraper) | Public default/custom scraper overflow preserves Markdown sentinel; HTML overflow distinct; cancellation cause preserved; bounded cooperative conversion limitations explicit. |
| 20 | R20, D35 (DOCX) | Public DOCX tabs/br/cr preserved as bounded whitespace; styled runs concatenate; separators counted in ParsedBytes; supported namespace/page/column semantics explicit. |
| 21 | R21, D26 | Rollback on every finalize failure/cancel with causal recovery diagnostics/backups; fault injection after first commit; ordinary streaming generation with explicit host async composition and terminal/error/cancel examples. |
| 22 | D01 | Remove generic chat-history compaction duplication after checking contexty capabilities; tool-result transcript/codecs stay; migration, no mandatory core dependency. |
| 23 | D02 | UI actions moved to shell/host; neutral bounded control contract with precise Pause/Yield/Halt effects; migration and examples. |
| 24 | D03 | Human review intent naming and clear absence of bound-grant authority; thin adapter retained; authenticated issuance host-owned. |
| 25 | D04, D05 | One primary Policy/Decision contract, narrow authorizer adapter; obsolete aliases removed; required nil gates fail explicitly; optional mode intentional; authorization/budget tests and migration. |
| 26 | D09 | Required state mutation returns errors; optional helpers explicit; nil handlers rejected consistently at construction; public regressions. |
| 27 | D10, D11 | Validated named operation profile config; original-grant recovery invariant retained; audited reauthorization/reconciliation guidance; migration. |
| 28 | D14 | Bounded local filejournal complexity/maintenance documented; reverse consumed/grant consistency verified; no automatic idempotency eviction or integrity claims. |
| 29 | D23 | Filter service names before descriptor fetch; selected dependencies retained; service/file/byte discovery bounds; excluded-service error fixture. |
| 30 | D24 | Exact custom agent stream/cancel profile; cancellation diagnostics and parent/timeout/callback-stop distinctions; no scheduler expansion. |
| 31 | D25 | Portable OTel mapping with explicit vendor opt-in; default-off content capture; chunk redaction limitations documented and tested. |
| 32 | D27 | E2B argv with one serialization boundary; unused stdout/stderr sources removed; capped writers authoritative; backend guarantee limits explicit. |
| 33 | D30, D36 (RAG) | Retrieval strategy moved to host/ragy/routery; thin retriever remains; stale cap naming/duplicate encode removed where proven; migration and cancellation semantics. |
| 34 | D32 | Consistent nil-option/negative-limit validation across toolkits; security slices snapshotted; host-port mutability explicit; regression fixtures. |
| 35 | D34 | Honest lexical SQL subset naming/dialect limits; DB role remains authorization boundary; examples/migration. |
| 36 | D36 (time/state) | Current StateStore terminology; location applies to calculate; DST calendar-day vs duration-hour semantics accurately documented/tested. |
| 37 | D40 | Exact filesystem/mount/hardlink/atomicity and Starlark/host isolation limits retained; host snapshot/atomic replacement requirements; mocks distinguished from live proof. |
| 38 | R22, D38, D39 | Normative generator presence/type table with compiling fixtures; runnable manifest/CLI/handler/stream example; nested subset honest; API/migration/install/version index; historical audits separated. |
| 39 | R23, docs checklist | Caller deadlines vs backend-owned bounds/capabilities explained with examples; every source docs-checklist item verified against current APIs and executable fixtures. |
| 40 | Final verification | All-module tests/race/lint, targeted adversarial regressions, changed hot-path/lock/stream benchmarks, disposable release artifact consumer verification; two independent full-scope acceptances; limitations recorded. |

## Evidence and decisions

### 01 — R01 (accepted)

Criteria for both reviewers (each worth 20%):

1. Remote credentials and request data never cross scheme/host/effective-port
   origin on redirects; port, subdomain and downgrade cases covered.
2. Effectful initial requests never redirect, including method rewriting and
   same-origin 307/308 body replay; no hidden second dispatch.
3. Same-origin GET/HEAD positive controls work with credentials; HTTP toolkit
   whitelist read redirects remove sensitive headers on foreign origins.
4. Public agents/OpenAPI/GraphQL fixtures and shared-policy adversarial tests
   pass with race detection, including dependent web/document/MCP modules; new defect regressions fail on baseline; SSRF and
   redirect-count validation remain enforced.
5. Public contract/migration/docs match behavior; legacy insecure paths removed;
   targeted lint is clean and both reviews find no unresolved defects.

Decision: remote adapters allow only same-origin redirects for requests that
started as GET/HEAD. POST (including GraphQL queries/introspection), PUT, PATCH,
DELETE and other methods never redirect, even when 301/302/303 rewrites to GET.
Hosts select the final RPC endpoint explicitly. HTTP toolkit GET/HEAD may follow
whitelisted origins with credentials removed; effectful requests never redirect.

Evidence: [R01 acceptance](task41/r01-acceptance.md). Final reviewers
`r01_final_a` and `r01_final_b` independently accepted all five criteria at 100%
over the cumulative baseline diff, including all seven direct modules. Initial
four-module reviews rejected classification/lint findings, then accepted; the
scope was expanded before starting R02. Final reviewers rejected remaining
consumer classification/MCP guard issues at 90%; public assertions and fixes
closed these, and both complete repeat reviews accepted at 100%. No unresolved
detected defect. Commit: `4dec511` (`fix: redirect credentials`).

### 02 — R02 / D17 (accepted)

Criteria for both reviewers (each worth 20%):

1. Runtime matching denies every blocked match before DNS/dial, including
   identical exact/suffix entries and broad allow with narrow deny.
2. Explicit shared syntax: bare hostname exact; leading dot descendants only;
   apex plus descendants requires both entries. Whitelist and blacklist use
   identical matching; case/outer whitespace/DNS root-dot normalization align.
3. Positive sibling/apex controls and negative overlap fixtures; public transport
   rejects before resolution, and configured nonempty allowlist never becomes
   permissive because normalized entries are blank. P1 overlap regression fails
   on original source.
4. Affected consumer tests and R01 regression suite remain race-clean; targeted
   lint and cumulative whitespace check pass. No SSRF/origin protection removed.
5. Migration explicitly describes the security-relevant blacklist change;
   current docs/options match syntax; conflict-map and duplicate matching paths
   removed; both independent reviewers find no unresolved defect.

Decision: exact/suffix syntax is the same for allow and deny lists. Use
`["example.com", ".example.com"]` when both apex and descendants are intended,
especially when migrating a previously implicit descendant blacklist.

Evidence: [R02 acceptance](task41/r02-acceptance.md). Independent reviewers
`r02_acceptance_a` and `r02_acceptance_b` each accepted all five criteria at
100%, with no unresolved detected defects. Both repeated seven-module race
suites and targeted lint; reviewer B independently measured zero DNS calls for
denied admissions. Commit: `d1619e8` (`fix: host policy`).

### 03 — R03 (accepted)

Criteria for both reviewers (each worth 20%):

1. Shared lossless JSON input parsing preserves 2^53±1 and max int64 for dynamic
   handlers, typed validation and interface-valued typed arguments.
2. Exact-number schema compilation enforces integer minimum/maximum/enum and
   fractional rejection, including constraints on properties named id.
3. Handler values, prepared snapshots and cache keys remain distinct for distinct
   large integers; public regression asserts each boundary.
4. Explicit structure policy rejects recursive duplicate keys, trailing documents,
   depth above 128 and node count above 100,000; migration records the break.
5. Original P1 numeric regressions fail behaviorally; current affected module
   race/lint suites pass; documentation and legacy removal match implementation;
   both independent reviewers find no unresolved detected defect.

Decision: use existing bounded jsonschemax Decode/Compile for all validated input
pipelines. Dynamic and interface numbers become json.Number; explicit Go float
fields retain the host's chosen floating-point semantics. Schema transformation
visits schema objects only, preserving property names and literal values.

Evidence: [R03 acceptance](task41/r03-acceptance.md). Independent reviewers
`r03_acceptance_a` and `r03_acceptance_b` each accepted all five criteria at 100%
with no unresolved detected defect. Both ran their own race/lint checks and
numeric/schema adversarial overlays. Commit: `322ec4a` (`fix: input precision`).

### 04 — R04 / D12 (accepted)

Criteria for both reviewers (each worth 20%):

1. Result/effect/postcondition failures after the handler are nonretryable,
   noncorrectable INTERNAL errors with inspectable phase and original cause;
   a validator-supplied ToolError cannot replace this classification.
2. Public handler-effect counter and host argument-repair fixtures prove one
   dispatch and zero results on every postvalidator failure; baseline fails.
3. Pre-handler argument validation remains correctable with zero effects;
   success still emits one valid result.
4. Journal records unknown on posthandler contract rejection and never blindly
   redispatches; completed results survive delivery errors. After-claim checks
   use honest may-have-dispatched diagnostics without unfenced claim rollback.
5. Docs/examples match phase/recovery semantics; targeted race/lint pass;
   both independent reviewers find no unresolved detected defects.

Decision: ResultContractError.Kind names result_validator, effect_validator or
postcondition for these failures; outer INTERNAL always wins over callback codes.
OperationOutcomeError records local DispatchInvoked without claiming external
success or proof of not-started. All unfinished claimed attempts remain unknown.

Evidence: [R04 acceptance](task41/r04-acceptance.md). Independent reviewers
`r04_acceptance_a` and `r04_acceptance_b` each accepted all five criteria at 100%
with no unresolved detected defects. Preliminary formatter/nested/downstream/control
findings were fixed, and both independently repeated final race/lint and probes.
Commit: `4f372b2` (`fix: result validation`).

### 05 — R05 / D37 checkout and artifacts (accepted)

Criteria for both reviewers (each worth 20%):

1. Release preparation uses a private committed-HEAD checkout. Source branch,
   index, tracked files, untracked/ignored files and local refs are unchanged on
   success, failure and cancellation; unexpected files never enter the tree.
2. Tracked module inventory is complete and explicit; only expected go.mod and
   go.sum manifests are staged. Portable edits align internal requirements and
   remove internal development replaces without regex/BSD-sed assumptions.
3. Rewritten artifacts are verified through a private file module proxy with
   GOWORK=off, without development replaces; bounded smoke compilation resolves
   the exact internal module graph. External dependency verification remains.
4. Disposable repository/local-bare tests reproduce original file inclusion/loss
   and verify success/rejection/cancel preservation; production publish is never
   used. Preparation/verification failure cannot publish a candidate.
5. Release lifecycle, source snapshot, verification and host-platform limits are
   documented; meaningful checks pass and two independent reviewers accept at
   100% with no unresolved detected defect.

Decision: source must have no tracked changes; untracked/ignored files stay local.
All source Git reads disable optional locks. Isolate clone/fetch/edit/verify/commit
and publication in a private checkout; the source index and refs are never written.
Discover modules from tracked go.mod files and reject incomplete supplied lists.
Use native Go manifest tooling and verify artifacts before the final confirmation.
R06 retains responsibility for explicit ref scope, collision preflight and atomic
publication; this row does not certify those remaining publication properties.


R05 final implementation also rejects transforming Git attributes before checkout
and bootstrap archive, forces candidate LF/symlink behavior while preserving source
EOL policy, and preserves counted native-CLI auth while filtering repository/hook
selectors. The complete private 24-module lint/race suite exposed three stale R04
post-validator test assertions in document/timetool; only those expectations were
synchronized to the existing INTERNAL/phase/cause/no-repair contract.

Evidence: [R05 acceptance](task41/r05-acceptance.md). Independent reviewers
`r05_acceptance_a` and `r05_acceptance_b`: **100%, accepted**, no unresolved detected
errors. Final full CLI prepare-only execution exited 0 after exact ZIP compilation,
private make lint/test with race, clean candidate check and cleanup. Earlier lint
lock and DNS availability failures are recorded; final success uses serial lint,
pinned installed 2.14.0, and a task-owned external archive cache. Production publish
was never used. Commit: `5f80c9e` (`fix: isolated release`).

### 06 — R06 / D37 publication refs (accepted)

Criteria for both reviewers (each worth 20%):

1. Publish only explicit root/submodule tag refspecs created by this invocation;
   unrelated local tags/commits and configured followed tags never publish.
2. Check local and exact push-destination collisions before manifest rewrites
   and again before tagging; reject ambiguous multiple push destinations.
3. One atomic, non-forcing push publishes the entire train; tag rejection or
   unsupported atomic capability leaves no partial train, with no fallback.
4. Local bare fixtures prove scope, collision, failure and source preservation;
   pre-existing remote/local refs are never deleted or rewritten by rollback.
5. Docs describe concurrency/platform limits; race/lint pass; two independent
   reviewers accept 100% with no unresolved detected defect.

Decision: explicit root/submodule refs only; check the single exact push
destination before edits and after confirmation. Require atomic capability,
without force, followed tags, mirror or sequential fallback. No remote rollback.

Evidence: [R06 acceptance](task41/r06-acceptance.md). Independent reviewers
`r06_acceptance_a` and `r06_acceptance_b` each accepted 100% (five × 20/20),
with no unresolved detected defects, independent race/lint and hostile-config /
concurrent collision probes. The previous-commit regression reproduces unrelated
tag publication. Final root checks recorded in the acceptance report.
Commit: `4ddcb4f` (`fix: release tags`).

### 07 — R07 / D18 / D19 (accepted)

Criteria for both reviewers (each worth 20%):

1. Agent/OpenAPI/GraphQL public clients/tools reuse one owned safe pool per
   configuration, with bounded idle retention; HTTP/web/document consumers share
   the same policy and do not create pools per call. MCP retains one owned safe
   pool, applies explicit settings and closes owned idle connections on Close.
2. Explicit timeout/TLS settings replace accept-and-ignore HTTPClient ports;
   TLS settings are applied to safe pinned transports, redirect/host/IP policies
   remain enforced, and unsupported custom Do/proxy/transport is not accepted.
3. Agents expose CloseIdleConnections; tool/contract factories expose optional
   cleanup-returning variants. Cleanup closes only owned idle resources, while
   compatibility factories retain automatic bounded idle expiry.
4. All resolved IPs are checked before dialing; bounded sequential attempts use
   those pinned addresses with one total timeout and caller cancellation. Stream
   reader remains stop-after-budget (exact cap cannot establish EOF), documented
   and covered including empty reads; no unbounded EOF probe.
5. AAA keep-alive/TLS/SSRF/cancellation fixtures and affected-module race/lint
   pass; migration is accurate; both independent reviewers accept 100% with no
   unresolved detected defect.

Decision: remove legacy custom HTTPClient / MergeHTTPClient surface; use
ClientSettings for timeout and TLS over the owned safe transport. Agent NewClient
returns (*Client, error) to reject invalid settings during construction. Existing
AsTools/ParseURL/Introspect factories remain, with explicit cleanup-returning
variants for host lifecycle ownership. No custom dial/Do/proxy port is introduced.

Evidence: [R07 acceptance](task41/r07-acceptance.md). Independent reviewers
`r07_acceptance_a` and `r07_acceptance_b`: 100% each (five × 20/20), no unresolved
detected defects. Seven-module race/lint, real mTLS/SSRF/timeout and active-call
cleanup probes passed after current README and lint gates were fixed. Previous
commit reproduces 20 pools for 20 calls. Commit: `d1102f6` (`fix: http pools`).

### 08 — R08 (accepted)

Criteria for both reviewers (each worth 20%):

1. Registry and binding live in one immutable configuration snapshot; Rebind
   compatibility validation and publication are atomic under concurrent writers.
2. Execute/RunCall capture one registry snapshot per call; completion policy and
   dispatch cannot mix registries. Public Binding returns an independent clone.
3. ExportCheckpoint outer/inner binding come from the same snapshot; concurrent
   ExportSnapshot/ImportSnapshot/Rebind remain race-clean and reject incompatible
   bindings without changing configuration/state.
4. Host callbacks execute without configuration/state locks. Snapshot export
   copies the state map under lock, then invokes codecs/marshal outside it; host
   referenced state values retain their explicit immutability responsibility.
5. AAA concurrency, coherent-snapshot, callback reentry and incompatible-rebind
   fixtures pass with race/lint; original race is reproduced; docs are current;
   both independent reviewers accept 100% with no unresolved detected defect.

Decision: atomic pointer to immutable session configuration, CAS validation and
replacement for Rebind. Execute/RunCall use a captured configuration. Checkpoint
uses exported snapshot's binding directly. Do not hold locks across host codecs,
manifest callbacks, handlers or yield. Session options stay immutable after setup.

Evidence: [R08 acceptance](task41/r08-acceptance.md). Independent reviewers
`r08_acceptance_a` and `r08_acceptance_b`: 100% each (five ×20/20), no unresolved
detected defects. Original races/wrong-registry/callback-lock failures reproduce
on d1102f6. Root race/lint and independent concurrency/reentry/nil/failing-decode
probes pass. Public benchmarks record unchanged Execute allocation count and
extra map-clone allocation cost for snapshot export. Commit: `df733dd` (`fix: session binding`).

### 09 — R09 / D07 / D08 (accepted)

Criteria for both reviewers (each worth 20%):

1. Explicit codec registry freeze/build lifecycle: NewSession freezes codecs
   after ordinary constructor validation; registration after freeze fails with
   inspectable ErrStateCodecRegistryFrozen. All registrar paths enforce it.
2. Codec entries/digest stay one stable schema lifetime; successful state+binding
   checkpoint restores with the same frozen registry, including required slots.
   Missing required state is rejected during export, before a useless checkpoint
   can be returned; host codec round-trip behavior remains a host responsibility.
3. Late optional/required registration and concurrent registration/construction
   have deterministic admission outcomes; constructor validation failure does not
   prematurely freeze the caller builder; schema changes need a new registry.
4. D07 names state+binding checkpoint accurately: RunPolicy/maxSteps/consumed
   count/dependencies/continuation are not persisted, host restores current
   authority/durable budget. D08 documents BYOT alias synchronization and codec
   callback reentry outside locks; no universal deep copy or callback rollback.
5. AAA lifecycle/concurrency/restoration/callback probes pass with race/lint;
   baseline reproduces accepted late registration/unrestorable checkpoint; docs
   and migration match; both independent reviewers accept 100%, no unresolved bugs.

Decision: freeze the supplied codec builder for its shared schema lifetime at
NewSession construction, after registry/run-policy validation. Freeze is also an
explicit public method; registration remains mutable beforehand. Capture one
stable digest after freezing; codec implementation state remains a host contract.

Evidence: [R09 acceptance](task41/r09-acceptance.md). Independent reviewers
`r09_acceptance_a` and `r09_acceptance_b` accepted all five criteria at 100%,
with no unresolved detected defects. Parent tests/race passed all 24 modules;
root lint has zero issues. The previous-commit behavioral probe fails and the
same probe passes on the fix. Registered 32-slot snapshot export adds approximately
2.8KiB and four allocations for required-slot checks; no-codec snapshot and
Execute allocation counts are unchanged. Short timing samples are not a
statistical guarantee. Checkpoint authority/budgets and callback/value ownership
remain explicit host responsibilities. Commit hash is recorded in the next row.

### 10 — R10 / D06 (accepted)

Criteria for both reviewers (each worth 20%):

1. RunPolicy slices are cloned when WithRunPolicy captures and materializes its
   option, and again when NewSession publishes configuration; reused options and
   separate sessions own independent snapshots. Caller mutation after capture or
   construction cannot alter admission or race with library reads.
2. CatalogRequiredTools declares visible catalog requirements, validated during
   construction before codec freeze. It never serves as an alternative call
   whitelist. AllowedTools and ForcedTool define call selection; catalog-required
   tools may differ from allowed/forced calls. Registry execution remains separate.
3. WithMaxCalls / CallAttempts / MaxCalls replace agent-step terminology and legacy
   exports. Negative limits fail construction; zero is unlimited. Atomic attempt
   accounting has explicit admission ordering and bounded successful admissions
   under contention; budget-rejected attempts count, policy-denied/nil-registry
   attempts do not. Internal retries consume one outer admission.
4. AAA regression probes cover capture/materialization/reuse mutations, concurrent
   caller writes, catalog validation, accounting/error wire and checkpoint restore
   semantics. The baseline public policy-mutation probe fails before the fix.
5. Affected tests/race/lint and documentation/migration pass; obsolete primary
   names are removed; two independent reviewers accept all five criteria at 100%
   without unresolved detected bugs.

Decision: clear break to CatalogRequiredTools and call-attempt naming, retaining
existing admission accounting (including rejected over-budget attempts) explicitly.
RunPolicy is a constructor snapshot, not an agent iteration state machine.

Evidence: [R10 acceptance](task41/r10-acceptance.md). Independent reviewers
`r10_acceptance_a` and `r10_acceptance_b` accepted all five criteria at 100%
on the final diff, with no unresolved detected defects. All 24-module race tests
passed; root lint has zero issues. Baseline admits caller-mutated write and
rejects captured read; identical probe now rejects write and accepts read.
A 32-name constructor adds 2048B/four allocations for independent snapshots;
short timing samples are not a latency guarantee. Clear-break migration includes
catalog/selection separation and the MAX_CALLS_EXCEEDED wire code. Commit hash
is recorded in the next row.

### 11 — R11 / D16 (accepted)

Criteria for both reviewers (each worth 20%):

1. Empty typed results always build a success envelope with audience, delivery
   class and metadata; wire bytes stay absent, while typed Value, effects/controls and
   result status survive direct, registry, RunCall and cached replay paths.
2. Explicit result algebra: Empty/Noop are exclusive statuses without wire bytes; Noop
   cannot declare effects. Nonempty Raw overrides only wire encoding, retaining
   the typed Value; Raw conflicts with Empty/Noop. Stray RawMimeType is rejected.
   Generic result chunks/replays enforce the same flag/wire/effect invariants;
   typed values are preserved, including empty MCP wire projections.
   Post-handler contract errors preserve classification and never authorize retry.
3. Audit exact output schemas before changing mappings: nested RawMessage output
   supports any valid JSON, while args retain the documented object default;
   explicit host type mappings override defaults without shared-registry mutation.
   Top-level custom encoders require explicit output schemas for shape constraints.
4. BYOT clone limits are explicit: exported data and cycles within one cloned
   value, host-owned opaque fields/functions/channels/map keys, no general alias
   graph or serializer guarantee across independent components/overlapping slices.
5. AAA representation/envelope/schema/cache regressions and affected race/lint
   pass; baseline probes fail for the defects, current migration describes breaks;
   two independent reviewers accept 100% with no unresolved detected errors.

Decision: Empty omits wire bytes and may report effects/control; Noop omits wire
bytes and declares no effects. Both retain typed Value. Raw is only an explicit nonempty wire representation
of Value, with output-schema validation applying to JSON wire bytes.


Evidence: [R11 acceptance](task41/r11-acceptance.md). Independent reviewers
`r11_acceptance_a` and `r11_acceptance_b` each accepted the final retained-Value
contract at 100%, with no unresolved detected defects. Both repeated root/MCP
race checks and adversarial probes after the contract revision. Parent all 24
modules pass race tests, root lint reports zero issues, and baseline behavioral
failures now pass. Commit hash is recorded in the next row.

### 12 — R12 / D13 / D15 (accepted)

Criteria for both reviewers (each worth 20%):

1. Eligible cache misses deliver business-error terminals unchanged, without Put;
   direct/registry/RunCall classification agrees with no cache. Unsuccessful and
   missing terminals are distinct; producer/consumer/control/cancellation failures
   remain inspectable and sticky without successful persistence.
2. Cache infrastructure/configuration/limit/codec failures are INTERNAL,
   nonretryable and not input-correctable, preserving causes; failures after
   handler dispatch do not authorize correction or imply effect rollback.
3. Mandatory host CacheEligibility predicate runs after current authorization
   on every attempt, before partition/storage; false bypasses caching completely.
   Idempotent/ReadOnly alone do not enable reuse. Eligibility errors fail closed;
   partition and host freshness/expiry obligations are explicit.
4. Neutral ReplaySourceMetadata distinguishes result_cache and completed_operation;
   codec/decode/rebinding is shared without a fake cache instance. Nested policy
   overlays cannot erase provenance or broaden replay audience; reducers handle
   both sources without reapplying declared effects.
5. AAA baseline/current business-error probe, targeted/all affected race/lint,
   migration and examples pass; two independent reviewers accept all criteria at
   100% without unresolved detected defects.

Decision: require an explicit per-attempt eligibility predicate, independently of
manifest idempotence. The host must prove reuse safe and bind freshness to its
partition/store policy. Replace the cache-specific boolean marker with a neutral
string provenance key and distinct source values; no legacy alias remains.

Evidence: [R12 acceptance](task41/r12-acceptance.md). Independent reviewers
`r12_acceptance_a` and `r12_acceptance_b` each accepted all five criteria at
100%, no unresolved detected defects. Both found callback cancellation gaps;
context guards after host boundaries and in shared operation/cache replay closed
those findings, with final independent repeated probes/root/MCP race checks.
Parent all 24 modules pass race tests; root/MCP/filejournal lint zero issues.
Baseline business terminal loses delivery; corrected probe delivers unchanged.
Commit hash is recorded in the next row.

### 13 — R13 / D20 / D21 / D22 (accepted)

Criteria for both reviewers (each worth 20%):

1. Local subscription cancellation retires correlation within explicit bounded
   count/time limits; late ACK/notification cannot alter generations or cancel B.
   Truly unknown/malformed messages retain strict protocol handling; ID reuse
   during retirement and saturation behavior are explicit and fail closed.
2. Separate per-frame, retained queue/in-flight and optional lifetime transport
   budgets; ordinary long-lived stdio/SSE is not capped by cumulative default
   traffic. Oversized frames/queues fail before retention/dispatch; contexts and
   typed limit causes remain inspectable. Negative/nil configuration fails early.
3. Full tool discovery bounds aggregate raw bytes/items/pages/cursors before
   descriptor accumulation/schema compilation. Failed/stale/canceled/duplicate
   discovery canceled before the commit point does not publish authority; later
   cancellation cannot undo a started successful synchronous commit. Descriptors
   remain untrusted.
4. ListToolsPage exposes page snapshots without authority publication; one typed
   DiscoverTools path validates/publishes full authority, and Discover proxies
   share its implementation. Clear API naming/migration; no unrestricted authority
   setter. Custom transport minimal contract/facets are documented accurately.
5. AAA public subscription/budget/discovery regressions, baseline cancellation
   probe and affected race/lint checks pass; current docs/examples synchronized;
   two independent acceptance reviewers return 100%, no unresolved detected bugs.

Decision: bounded retirement protects locally canceled IDs only; messages outside
that correlation window are unknown and strict. Default frame/queue/in-flight
bounds are distinct from optional lifetime traffic limits. Full discovery is the
sole client authority publication path; page inspection is explicitly nonauthoritative.

D22: retain the minimal transport and bounded synchronous atomic header-replace
facet. Only ReplaceToolHeaderBindings forbids Client reentry while the matching
client/transport authority lock is held. Descriptor mapper reentry remains allowed.
Cancellation is rechecked under that lock before starting publication; cancellation
after the commit point does not roll back success. This preserves coherent snapshots
without inventing a reentrant transaction protocol for a host-owned facet.

Row13 final gate: acceptance A 100% (forced MCP race19.805s, independent probes
count5, lint0), acceptance B 100% (MCP race21.211s, independent probes count5,
lint0); no unresolved detected defects. Parent full MCP race21.349s, focused
count5 race3.237s, lint0. Evidence: docs/reviews/task41/r13/. Initial findings and
verdicts are superseded by final accepted reviews. Commit: fix: mcp lifetimes.

### 14 — R14 / D28 (accepted)

Criteria for both independent reviewers (20% each):
1. Owned Unix process-group descendants are stopped on guest exit0/nonzero,
   cancellation and collection failure before workspace deletion; no PID/group
   reuse signal after the owned leader is reaped. No isolation/escape claims.
2. Cleanup/collection is bounded by existing cleanup timeout/WaitDelay, cancellation
   takes precedence, primary guest exit/output is preserved on successful cleanup.
   Failed cleanup is an inspectable secondary diagnostic with backend resource ID.
3. exectool error paths retain the sandbox-returned outcome in a typed error cause,
   without emitting success or enabling blind retry. Infrastructure/timeout/output
   classification is preserved and contract describes incomplete results.
4. AAA tests cover redirected and inherited child pipes, success/nonzero/cancel,
   guarantee fixture cleanup and verify no surviving group child before removal.
   A public baseline child-leak probe demonstrates before/after behavior.
5. Affected tests/race/lint and platform compile checks pass; docs/migration accurately
   scope Unix supervision and unsupported platform capabilities; both reviewers100%.

Contract: Unix host uses an owned /bin/sh anchor as unreaped process-group
leader; the guest launches directly through Go exec. Kill the owned group before
reaping the anchor; an explicit cleanup budget bounds owned output pipe collection.
Unix process-group membership escape is outside this non-isolated adapter contract.

R14 review corrections: keep the anchor separate from direct guest exec (missing
executables remain infrastructure failures; guest exit126/127/signal preserved).
Preserve unsupported-language cause through validation mapping. Join all owned
watchers/collectors before anchor reap; direct-process fallback on failed group
signals retains cleanup diagnostics. Coordinate initial stops, then sweep after
guest Wait while the anchor pins PGID, covering fork-vs-SIGKILL races. Transient
EPERM is never removal confirmation; retain bounded observation to ESRCH. Add
permanent rejecting-writer/fork fixture with cleanup of both possible children.

Row14 final gate: A100% (hostrace13.445s, exectool1.991s, adversarialcount10
4.209s, bothlint0, Linux/WindowscompilePASS); B100% (hostrace12.910s,
collection/fork+overflowcount20race5.421s, publicstart1.851s, lint0,
Linux/WindowscompilePASS). No unresolved detected defects. Parent finalhostrace
11.622s, targetedcount10race4.235s, exectoolrace1.912s, host/rootlint0.
Evidence: docs/reviews/task41/r14/. Commit: fix: host descendants.

### 15 — R15 / D29 (accepted)

Criteria for each reviewer (20% each):
1. Starlark preserves exact stdout bytes on guest exit0/1, including trailing empty
   lines and no output; no success-only presentation trim or internal trim flag.
2. Shared finalizer signature/callers across every sandbox remain coherent; normal
   guest exits, output overflow and cancellation classification stay intact.
3. Docker constructor-positive policy is the single source of limits: unreachable
   output/log-timeout fallback defaults removed; custom bounds and rejection remain.
4. Export focused Docker Client port with its lifecycle/reader/capability ownership
   contract, without giant SDK dependence in core. Starlark fs cap remains an
   explicitly documented guest error, distinct from infrastructure stdout overflow.
5. AAA byte-identity baseline/current probes, all affected tests/race/lint and docs
   pass; two independent reviewers100%, no unresolved detected defects.

Decision D29: interpreter fs.read failures are guest evaluation errors (exit1,
bounded stderr, nil Run error); stdout collection overflow is infrastructure/output
failure. Preserve this meaningful distinction rather than remapping every failure.

Row15 gate: A100%, B100%; independently verified exactbyte baselineFAIL/current
race probesPASS and all five adapter/sharedhelper race; root+alladapterlint0.
No unresolved detected defects. Parent public currentprobe count5PASS1.477s,
all affected race/rootracePASS, final pinnedlint0. Evidence docs/reviews/task41/r15/.
No live Docker/E2B; interpretation/resource limits remain backend-specific.
Commit: fix: sandbox output.

### 16 — R16 (accepted)

Criteria for each independent reviewer (20% each):
1. GET/POST tool response body is explicitly UTF-8-only: valid bytes roundtrip
   unchanged, invalid UTF-8 rejected before bytes-to-string/JSON replacement, with
   no charset sniffing/transcoding or MIME guesses. Library readers remain binary.
2. Encoding failures retain typed method/status/cause as post-dispatch result errors;
   no successful result, argument repair or automatic retry. POST effects are not
   reported as rolled back; adversarial counters prove single dispatch.
3. Source/wire budgets remain independent and inclusive, validated before decoding;
   limits/escaping/exact-boundary/context fixtures pass. POST response-read/wire
   failures are result-phase failures; pre-dispatch request bounds stay validation.
4. API/README/migration explain UTF-8 representation, binary library alternative,
   unchanged status/body shape and POST reconciliation/cancellation limits accurately.
5. AAA public baseline/current encoding/body-limit/POST-effect probes and affected
   tests/race/lint pass; both reviewers100%, no unresolved detected defects.

Contract: keep JSON status/body string shape, require actual UTF-8 response bytes
regardless of Content-Type charset; no binary encoding inference. UTF-8 encoding
failures are CodeInternal ResultContractError with typed ResponseEncodingError.
Post response read/wire failures also cannot be treated as pre-dispatch argument
errors. Caller cancellation retains its precedence and does not imply rollback.

Row16 gate: A100%, B100%, five20% criteria each; no unresolved detected defects.
Initial canceled POST wire-validation gap fixed and independently reverified.
A fullHTTP race+probe count3PASS4.825s/lint0; B fullHTTP race count3PASS4.158s,
adversarialcount5PASS1.624s/lint0. Parent fullHTTP racePASS2.477s/lint0,
rootracePASS (cached except generator18.865s). Public baseline GET/POST invalid
UTF8FAIL as expected/currentcount5PASS2.144s. Evidence docs/reviews/task41/r16/.
LocalHTTP only; cancellation checkpoints are not rollback, no live API/performance
claims. Commit: fix: http encoding.

### 17 — R17 / D33 (accepted)

Criteria for each independent reviewer (20% each):
1. MessageBody declares BodyRepresentation: zero/plaintext preserves every body
   byte (including whitespace, email angle brackets, placeholders and XML). Only
   explicit HTML converts to Markdown; no content sniffing or legacy fallback.
2. Read JSON exposes resulting representation truthfully. Unsupported declarations,
   invalid UTF-8 and conversion failures are inspectable result-contract errors
   with causes and no success/argument-repair chunk. Cancellation remains interrupt.
3. Actual raw fields/body caps precede conversion; final encoded JSON cap includes
   representation and escaping. Exact/over raw/wire and conversion expansion tested;
   synchronous converter cancellation checkpoints are documented honestly.
4. API/README/migration assign MIME parsing to host adapter, replace orchestrator
   terminology, show explicit HTML migration, keep send args/approval unchanged.
5. AAA public baseline/current plaintext proof, declared HTML/error/bounds/context
   regressions and affected race/lint pass; both independent reviewers100%.

Decision D33: finite explicit plaintext/HTML representations on reader port, output
plaintext/Markdown annotation; conversion errors never silently return originalHTML.
No provider SDK or MIME parser added; sender action contract unchanged.

Row17 gate: A100%, B100% (five20%criteria each), no unresolved detected defects.
Initial stale package-documentation terminology fixed and both reviews repeated.
A finalmailracePASS2.137s; retained freshmailracePASS2.019s/lint0; B mailracecount2PASS1.617s/privateadversarial
count3PASS2.046s/lint0; parent mailfullrace+publicprobe count3PASS2.532s/lint0.
OldAPIpublicbaselineFAIL fourplaintextidentitycases; currentcount5PASS2.335s.
Evidence docs/reviews/task41/r17/. No live provider/MIME integration or hard CPU
preemption claim; converter errors tested through explicit internal seam.
Commit: fix: mail representation.

### 18 — R18 / D31 (accepted)

Criteria for each independent reviewer (20% each):
1. All pin/read/unpin wait cancellation returns before held provider completes,
   retaining deadline/cancel cause and performing no store callbacks for canceled
   waiter; cancellation rechecked after acquisition before provider I/O.
2. One instance still serializes complete Load/modify/Save across local callers,
   preserves concurrent updates and releases admission on all error/cancel paths.
   No waiter goroutines or per-session lock registry; callbacks remain host-owned.
3. Read facts is an escaped JSON object with exact keys/values (newline/equals/XML),
   empty object for no facts; stored state/action args unchanged, full bounded JSON
   includes escaping/keys. Schema and tests reflect clear wire break.
4. README/API/migration describe session scratchpad, one-instance coordination,
   no distributed/CAS/reentrant callback guarantees, store deadline/allocation
   responsibility and cancellation without rollback. Legacy presentation removed.
5. AAA blocked-provider baseline/current cancellation probes for all three tools,
   concurrency/error/bounds/cancellation regressions and affected race/lint pass;
   both independent reviewers100%, no unresolved detected errors.

Decision D31: structured facts JSON object; bounded session scratchpad only. Host
owns long-term semantic memory and multi-instance/process atomic coordination.

Row18 gate: A100%, B100% (five20%criteria each), no unresolved detected errors.
A fullmemoryracecount3PASS3.413s/lint0, privateprobe count5PASS1.627s;
B fullracecount3PASS2.554s/lint0, privateadversarialcount10PASS3.468s,
schema count10PASS2.072s. Parent fullmemoryrace count5PASS2.626s/lint0.
Baseline publicprobe FAIL allsix heldLoad waitcases as expected; currentpermanent
publicprobe included in fullrace. Evidence docs/reviews/task41/r18/.
Host owns callback cooperation, multi-process coordination and already-started
Save outcome; no hard preemption/rollback/fairness claim.
Commit: fix: scratchpad admission.

### 19 — R19 / D35 scraper (accepted)

Criteria for each independent reviewer (20% each):
1. Public ScrapePage and tool default/custom Markdown overflows retain
   ErrMarkdownExceedsLimit/cause; safe CodeValidationFailed/reason independent of
   custom diagnostic text, errors.Is ErrValidation and no successful output.
2. Actual custom output over cap without returned error receives same semantic
   sentinel. HTML source and final JSON wire overflow remain distinguishable from
   Markdown extraction; independent inclusive source/extraction/wire caps retained.
3. Cancellation precedes budget mapping, including active-context interrupt cause
   chains and custom callback cancellation with nil result error. Cancellation guards
   surround default layout stripping/conversion/custom call; no callback goroutine.
4. API/README/migration explicitly describe cooperative synchronous bounded-input
   conversion and host CPU/allocation/deadline responsibility; no hard preemption.
   Custom cap helpers and independent budget options accurate, no legacy fakecause.
5. AAA public baseline/current default/custom/unchecked-output/cancel probes and
   affected race/lint pass; both independent reviewers100%, no unresolved defects.

Decision D35 scraper: keep bounded synchronous conversion with context checkpoints,
no goroutine-per-uncontrolled callback and no claimed hard CPU/allocation quota.

Row19 gate: A100%, B100% (five20%criteria each), no unresolved detected defects.
Initial API-option documentation gap corrected and finaldiff re-reviewed.
A fullwebracePASS1.969s/lint0, adversarialcount5PASS1.637s;
B fullracePASS1.996s/lint0, externaladversarialcount5PASS1.717s.
Parent fullwebracecount3PASS3.199s/finalpinnedlint0. Baselinepublic sixsemantic
sentinelassertionsFAIL as expected; currentpermanentmatrixPASS in fullrace.
Evidence docs/reviews/task41/r19/. Core outer tool interrupt classification remains
existing Canceled/TIMEOUT behavior, original causes preserved; no hard preemption,
intermediate-allocation or liveexternal-network guarantee.
Commit: fix: markdown causes.

### 20 — R20 / D35 DOCX (accepted)

Criteria for each independent reviewer (20% each):
1. Public document extraction preserves WordML tab as TAB and br/cr as LF;
   default/textWrapping/page/column breaks are explicitly flattened to LF. Styled
   runs/text fragments concatenate without invented spaces; paragraph breaks retained.
2. Every inserted separator counts against ParsedBytes before append; inclusive
   boundary/overlimit/combined UTF-8, raw expansion/item/source/final wire guards
   remain bounded, failure emits no partial result and cancellation retains cause.
3. Exact supported WordML namespaces (transitional, strict and legacy no namespace)
   replace URI substring matching; arbitrary prefixes work and foreign namespaces
   do not masquerade as WordML. Supported text-only subset/layout loss explicit.
4. API/README/migration describe whitespace, page/column flattening, limits and
   cooperative bounded XML parsing without hard CPU/allocation preemption claims;
   no duplicate legacy separator/namespace path remains.
5. AAA public DOCX baseline/current whitespace/styled-runs/namespace probes plus
   separator-cap/context regressions and affected race/lint pass; both independent
   reviewers100%, no unresolved detected defects.

Decision D35 DOCX: exact namespaced text-only subset, legacy unnamespaced fixtures
retained intentionally; foreign namespace nodes ignored, no full OOXML validation.
Bounded cooperative XML parsing remains synchronous, host owns harder isolation.

Row20 gate: A100%, B100% (five20%criteria each), no unresolved detected defects.
A documentfullracePASS2.620s/lint0, independentcount5PASS1.523s/finalpublic
count3PASS1.272s; B fullracePASS2.513s/lint0, independent+public/helpercount5
PASS1.611s/finalpubliccount3PASS. Parent fullracecount3PASS5.271s/finalpinnedlint0.
Baseline publicDOCXwhitespace threeNamespaces+foreignspoof FAIL as expected;
currentpermanentpublic/helpertestsPASS. Evidence docs/reviews/task41/r20/.
Text-only supported node namespace subset; page/column flattened to LF, no complete
OOXML/layout or hard parser preemption/intermediate-allocation isolation claim.
Commit: fix: docx whitespace.

### 21 — R21 / D26 (accepted)

Criteria for each independent reviewer (20% each):
1. Every finalize failure/cancellation after any installed output follows one rollback
   path, including reserve backup, old-to-backup, temp-to-target and interruption
   after last successful rename. Current partially moved backup is restored too.
2. Primary and all rollback/owned-temp cleanup failures retain causes and target/
   recovery backup paths; failed restore never deletes the only recovery copy.
   Earlier staging temps are cleaned on staging failure. Backup removal after full
   successful install is explicitly post-commit cleanup, not fake crash atomicity.
3. Fault-injected AAA baseline/current tests cover failure reserve/backup/install/
   cancel/restore/remove after first commit, existing and new targets, successful
   complete install, preserved backups and no unreported partial/recovery state.
4. stream:true generates ordinary synchronous stream tool; progress/result/error/
   invalid input/caller cancellation are observable on Execute. Host explicitly
   configures AsAsyncTool timeout/collection/onComplete; acceptance is not terminal.
   Compiling generated fixtures and host example cover terminal/error/cancel/callback.
5. API/README/migration describe filesystem exclusive-writer/cooperative syscall/
   recovery/non-crash-atomic limits and explicit async composition; affected root/
   generated-module tests/race/lint pass, both reviewers100%, no unresolved defects.

Decision D26: keep generated streaming synchronous; host opts into async execution
and lifecycle configuration. No implicit background validation/accepted-only wrapper.

Row21 gate: A100%, B100% (five20%criteria each), no unresolved detected defects.
Unified failure recovery replaces legacy commit path; moved recovery backups survive
failed restore/remove. Joined cleanup diagnostics retain all causes and owned paths.
Post-commit disposal errors report CommitComplete=true with complete Result.Files.
Parent rootracePASS (generator24.894s), targeted recoverycount3PASS1.985s/lint0.
A rootracePASS/lint0/privateprobescount5PASS1.870s; B rootracePASS/lint0/private
probescount5PASS1.870s. Both independently ran generated streaming host example;
nested generated-module race covers sync terminal/error/cancel and explicit async.
Baseline0700cc5 three originalAPI recovery assertionsFAIL; currentcount5PASS2.107s.
Evidence: docs/reviews/task41/r21/. Exclusive writer, cooperative local FS operations;
no crashatomicity/concurrentmutation/hardpreemption or nonlocalFS proof claimed.
Commit: fix: generator recovery.

### 22 — D01 (accepted)

Criteria for each independent reviewer (20% each):
1. Inspect pinned local contexty capabilities/source and run relevant boundary,
   rolling-summary/budget regressions; document coverage and semantic differences.
2. Remove generic history compaction package, its example and compaction-specific
   OTel helper/tests/docs, without recreating chat policy in core/optional adapter.
3. Keep tool-result transcript/historycodec/ResultCodec behavior and BYOT untouched;
   affected root/OTel compile/race/lint and transcript regressions remain green.
4. Migration names removed APIs and provides a compiled host-owned contexty recipe,
   explicit message projection/token-estimator/summarizer/retention ownership and
   fallback/error distinctions; root and extension gain no contexty dependency.
5. Current docs/consumer imports contain no stale deleted API recommendations;
   retained historical evidence is distinguished; both reviewers100%, no defects.

Spec-first decision: local contexty8416b7b9883a09a26e3fe740d02dd8f78bff0b7f
already owns rolling summary, tool-round-safe retention and budget execution.
Clear break removes toolsy/history and compaction-specific observability/example;
host adopts contexty semantic messages with explicit projection/policy, not a
claimed drop-in generic API. Tool-result transcript codecs remain in toolsy.

Row22 gate: A100%, B100% (five20%criteria each), no unresolved detected defects.
Removed generic compaction implementation/contracts/example and dedicated telemetry;
retained transcripts/historycodec/ResultCodec/BYOT unchanged. Source-pinned contexty
capabilities and compiled host migration expose projection/retention/fallback breaks.
Parent rootracePASS(generator52.173s), OTelracePASS1.481s/bothlint0;
contexty targetedracePASS4.270s and hostrecipecount3PASS1.620s.
A rootracePASS(generator42.179s), OTelcount3/contextycount2/hostcount3PASS,
bothlint0. B rootracePASS(generator42.400s), freshcore/historycodecPASS,
OTel/contexty/hostcount3PASS/bothlint0. Evidence docs/reviews/task41/d01/.
No contexty dependency added; local checked source only, host-owned projection and
provider budgeting/cooperative callbacks, no published/liveprovider guarantee.
Commit: refactor: history boundary.

### 23 — D02 (accepted)

Criteria for each independent reviewer (20% each):
1. Remove UIActionSignal/ErrUIAction and agent-track promises; sealed neutral
   HostEventSignal(Name, bounded JSON data)/ErrHostEvent carries no UI execution
   or authority. Host allowlists/routes events; core owns no shell dispatcher.
2. Define Pause/Yield/Halt precisely as current-call control requests, no scheduler,
   global halt, cancellation of other calls, durable pause/resume or grant issuance.
   CompletionPolicy remains host routing metadata, not an enforced state machine.
3. Enforce nonnil built-in signals, UTF-8, finite inclusive text/name/payload/count/
   aggregate bounds and strict JSON; malformed post-handler controls are INTERNAL
   result-contract failures, never argument-repair/retry. Delivery/cached controls
   use the same contract with snapshots; old cache kind ui fails explicitly.
4. AAA public control/consumer/middleware/typed-result/cache-codec regressions and
   runnable host event allowlist example verify no automatic UI action, no hidden
   scheduler and unrelated subsequent call remains executable; affected race/lint.
5. Current docs/migration list the clear API/persistence break and exact budget/
   delivery-only limits; no legacy UI API left; both reviewers100%, no defects.

Spec-first: neutral sealed HostEventSignal replaces UIActionSignal; payload is
optional strict JSON data. A delivered EventControl returns its matching sentinel
when the producer returns YieldControl; core does not preempt a producer ignoring
errors. Controls on terminal result are declarations, not automatic control errors.
Host owns UI adapters, event allowlist, persistence, authorization and scheduling.
Control field byte budget64KiB inclusive, event name128bytes, result controls64 max,
aggregate field budget64KiB; existing human16KiB default fits. Cached ui kind is
removed without silent reinterpretation. Validation is post-handler INTERNAL.

Row23 gate: A100%, B100% (five20%criteria each), no unresolved detected defects.
Neutral host events replace UI API; precise continuation/CompletionPolicy boundaries,
UTF8/strictJSON/list/name field budgets and invalidposthandler classification enforced
before normalization. Delivery snapshots built-in structs/payload; codec checks same
contract and rejects oldui kind. Host allowlist example owns routing, no coreexecutor.
Parent rootfullracePASS(generator26.795s), controlcount3PASS1.660s,
humancount3PASS1.718s, OTelPASS1.487s; allthree pinnedlints0.
A independently root/human/OTelracePASS/alllint0/probecount5PASS1.562s;
B root/human/OTelracePASS/alllint0/probecount5PASS1.834s.
Both executed host allowlist example; extra decode/snapshot/outputclassification
fixtures PASS. Evidence docs/reviews/task41/d02/. No hardproducerpreemption,
pre-allocation quota, genericaliasraceprotection or actualUI/scheduler proof.
Commit: refactor: host events.

### 24 — D03 (accepted)

Criteria for each independent reviewer (20% each):
1. Default conversational tool renamed request_human_review, payload kind
   human_review, option API WithReviewName/WithReviewDescription; no legacy aliases.
2. Thin bounded data-only pause adapter retained, no grant/authenticator/action
   execution port; clarification unchanged, both host-owned continuation requests.
3. Public AAA regressions verify new manifest/payload/custom descriptions, encoded
   inclusive bounds and no-grant action rejection after human/model assent; trusted
   host grant, exact original action replay and revoked policy remain protected.
4. Migration/current docs/examples state free-text intent has no identity, exact
   operation binding, expiry or grant authority; challenge/issuance host-owned,
   resumed operation/current policy required; executable example, module race/lint.
5. Bounds fit core MaxControlBytes, nil/invalid options fail explicitly; consumer
   error/cancel behavior preserved, no stale live old API; both reviewers100%.

Spec-first: human toolkit is a conversational review intent adapter. Rename public
review tool/option/payload contracts in a clear break. No scheduler, issuer or
operation-store dependency is introduced. Actual action authorization is governed
by OperationProfile's bound challenge and authenticated host-owned grant issuance.
Cap configuration must fit core's finite control delivery budget (1..65536bytes).

Row24 gate: A100%, B100% (five20%criteria each), no unresolved detected defects.
Review intent naming/options/payload replace misleading conversational approval API;
clarification remains unchanged and thin adapter has no authority/issuer/actionport.
Configuration fits core control cap; exact encodedUTF8 JSON/64KiB bounds and
pre-cancel/no-dispatch/hostgrant/replay/revocation contracts preserved.
Parent humanracecount3PASS2.238s/humanlint0; example+compositionPASS;
extra rootlint0 separately recorded. A humanracecount3PASS1.674s/lint0,
adversarialcount10PASS1.943s; B humanracecount3PASS1.748s/lint0,
adversarialcount10PASS2.988s. Evidence docs/reviews/task41/d03/.
Trusted-local fixture checks operation binding; no live user authentication/UI,
remote grant persistence or durable scheduling/hardpreemption proof claimed.
Commit: refactor: review intent.

### 25 — D04 / D05 (accepted)

Criteria for each independent reviewer (20% each):
1. Primary registry Policy/Decision path with stable binding ID; remove
   AuthorizationRequest alias, WithAuthorizer, WithAuthorization and separate
   registry authorizer field/path. Retain narrow Authorizer(PolicyRequest) adapter
   NewAuthorizerPolicy with explicit nil rejection and cause-preserving denial.
2. Explicit WithPolicy nil/typednil/nilfunc fails Build (sticky misconfiguration),
   view/restore typednil rejected; absent policy remains intentional no-gate mode,
   required manifest guards retained; typed argument policy stays after binding.
3. WithBudget requires dependency; absent/wrongtype/nil/typednil fails INTERNAL
   with inspectable configuration cause and no handler/replay. WithOptionalBudget
   bypasses only absent dependency, not malformed supplied dependency. Host owns
   accounting/pricing, concurrency and lifecycle; no built-in quota platform.
4. AAA baseline/current public misconfiguration probes, composed policy defensive
   snapshots/trusted view and identity, adapter cause/no argument repair, budget
   allow/deny/error/cancel/async/replay tests; root/affected race/lint/examples pass.
5. Migration/current docs/examples use primary policy and explicit optional gate,
   correct captured dependency/RunEnv contract, no legacy aliases/path; both100%,
   no unresolved detected defects. Non-JSON referenced BYOT identity remains host-owned.

Spec-first decision: consolidate authorization before validators/handler/profile
under registry Policy/Decision, including narrow authorizer adapter. Adapter
requires a host-selected stable WithPolicy ID and maps any authorization error to
nonretryable/noncorrectable policy denial while retaining original cause. Explicit
nil installation is configuration error; omission means no optional gate installed.
Budget WithBudget becomes mandatory at execution; WithOptionalBudget deliberately
permits absence only, and supplied invalid dependency fails. Dependency snapshot
is captured under RunEnv store lock and callback runs outside it, with cloned
manifest/input and cooperative cancellation checkpoints. Composed policies receive
independent framework-owned request snapshots (not arbitrary BYOT deep copies).

D04/D05 baseline probe at f5824b1 (production bytes unchanged) reproduces three
behavioral failures: explicitnilpolicy accepted, absentbudget dispatches1, typednil
budget dispatches1. Evidence docs/reviews/task41/d04-d05/ includes public fixture,
raw FAIL log, commit and production source SHA256. Implementation/review pending;
no acceptance or commit claim for row25.

D04/D05 implementation now consolidates the policy path and requires valid budget
admission by default. New AAA gate tests cover sticky nil configuration, view and
restore typed nil, authorizer cause/no-repair, required/optional dependency matrix,
callback snapshot/reentrant dependency update/cancellation, backend cause,
cache replay admission and async completion rejection. The executable
ExampleNewAuthorizerPolicy passes. The same three baseline public probes now PASS
on the current tree; targeted race PASS and root lint reports 0 issues. A full
root race run passed before the latest added test/example files; final current
root/affected-module runs and independent acceptance remain pending.

Final current root race PASS (including gate/async/cache tests and executable
example), root lint 0 issues; MCP/human/OTel race PASS. Two fresh independent
reviewers d04_d05_acceptance_a and d04_d05_acceptance_b started against current
diff; their verdicts and module lint are pending. No row25 acceptance/commit yet.

MCP/human/OTel pinned module lint completed: 0 issues in each module.
Independent acceptance is the remaining row25 gate.

Row25 independent final acceptance: A 100% (five20/20), B 100% (five20/20), no
unresolved detected defects. Reviewer reports and B independent full-root raw
race/lint retained in docs/reviews/task41/d04-d05/. Both independently assessed
current diff and did not read peer verdicts. All affected checks pass. Commit:
`fix: required gates` (hash recorded after successful signed commit).


### 26 — D09 (accepted)

Criteria for both independent reviewers, 20% each:
1. Required Put, SetState and SetSessionState return inspectable configuration
   errors for absent/unusable targets or empty keys; valid mutation returns nil.
   No ambiguous silent no-op remains; intentional optionality belongs to host.
2. Optional Lookup/GetState/GetSessionState and required Require behavior retained;
   valid DI without session and bound state share correct store/session; nil values
   are permitted storage values, typed lookup semantics unchanged. Framework map
   synchronization remains correct; referenced BYOT values remain host-owned.
3. Generic, stream, proxy, dynamic, typed and policy-constructor entry points reject
   nil handlers at construction with common inspectable ErrToolHandlerNil; base
   tool nil/typednil rejected by policy wrapper. No request-time nil dispatch.
4. Public baseline/current probes, AAA positive/negative/bound/concurrent mutation
   and constructor tests, executable examples and affected race/lint pass. Existing
   mutation call sites handle errors rather than discarding them.
5. Current docs/examples and clear-break migration explain mutation errors,
   zero/unbound environments, key/nil-value behavior, BYOT and handler construction;
   no competing legacy helper, and both reviewers find no unresolved defects.

Spec-first: retain primary mutation names but return error. ErrMutationConfiguration
is wrapped by INTERNAL, nonretryable/noncorrectable; nil RunEnv, zero RunEnv without
store, unbound session, nil Session and empty keys are invalid targets. Put still
permits DI-only NewRunEnv(nil); nil/typednil stored values remain legal (Lookup
reports missing/non-nil semantics unchanged). SetState delegates to bound Session;
SetSessionState mutates a valid synchronized session map. No implicit state creation
on DI-only RunEnv and no permissive helper. All handler constructors report shared
ErrToolHandlerNil before dispatch; policy wrapper rejects typednil base tool.

D09 baseline at 4b76f5f records six behavioral failures: Put/SetState/SetSessionState
return no error for nil/unbound targets (four cases), generic/stream constructors
accept nil handlers (two cases). Production bytes verified equal to baseline and
SHA256 retained. Current same public fixture PASS; new AAA direct contract tests
PASS under race, snapshot example executes. Implementation returns INTERNAL with
ErrMutationConfiguration and common constructor ErrToolHandlerNil. Existing mutation
call sites check errors, including goroutine-safe test error handling; nil Session
Execute state mutation now propagates failure. Root final lint 0 issues. Current
24-module race and root-final race are running; no independent row26 acceptance yet.

D09 final root race PASS; all24 modules race PASS, exit0,24 module markers. Current
original public fixture count3 PASS. Reviewer A found two explicit-type-argument
SetSessionState calls whose returned errors were not handled; both now checked.
Production unchanged; targeted codec/mutation/current tests and lint rerun, both
independent reviews continue against updated diff. No acceptance/commit claim.

D09 after-review targeted race count3 PASS2.412s, root lint0. A independently
rechecked latest diff/tests/lint and accepted100% (five20/20), earlier two ignored
calls resolved. B independent full-root race still live; no double acceptance yet.

D09 independent final acceptance: A100% (five20/20), B100% (five20/20), no
unresolved detected defects. Both reviewed latest diff and independently repeated
race/probe/lint. A's two ignored generic mutation calls fixed; both verified latest
call sites. Reports/raw reviewer logs retained docs/reviews/task41/d09. Commit:
`fix: required mutations` (hash recorded after successful signed commit). Core
BYOT reference synchronization remains host contract; no external service claims.


### 27 — D10 / D11 (accepted)

Criteria for both reviewers, 20% each:
1. NewOperationProfile accepts one named OperationProfileConfig with Store,
   Prepare, Codec, Issuer, Clock, Lease, MaxBytes; remove positional constructor
   and migrate all real callers/examples. Clear, inspectable configuration error.
2. Required ports reject nil/typednil; callbacks reject nil; issuer nonempty;
   lease positive, cap nonnegative with documented zero default. Configuration
   is captured by value; referenced host ports/callbacks retain host lifetime and
   synchronization ownership, no callback during construction.
3. Existing atomic approval/claim/finish/fence/reconciliation/replay dispatch
   boundary retained. No scheduler, automatic retries or grant replacement. Same
   operation recovery needs original consumed grant, explicit AllowRecovery,
   valid unexpired approval and trusted fenced reconciliation; fresh grant cannot
   reopen old operation, including when original approval expired.
4. AAA public baseline/current typednil config probes, valid/invalid configuration
   matrix, retained original/fresh/expired-grant recovery and no-effect counters;
   root/affected race/lint and compiling host examples pass.
5. Docs/migration/current constructor agree. Audited host reauthorization/new
   intent after trusted reconciliation explained with provenance/current policy,
   fresh authenticated approval and downstream duplicate-effect constraints;
   cannot silently refresh/rewrite immutable original grant. Both reviewers100%,
   no unresolved detected defects; no new core dependency/domain DTO.

Spec-first: replace the seven positional arguments with exported named config
(Store, Prepare, Codec, Issuer, Clock, Lease, MaxBytes). ErrOperationProfileConfiguration
identifies construction failure; typednil store/codec rejected. Retain positive
lease and cap zero default (bounded display and encoded result, same current cap).
Do not change journal authority/recovery transitions. An expired original grant
blocks retry of its old logical operation even after retry-authorized reconciliation;
a fresh unrelated grant cannot replace it. Host must resolve uncertainty first and
record audited linkage before creating any new authenticated intent, apply current
policy/approval and retain downstream idempotency/reconciliation responsibility.
No new grant refresh/replacement API and no implicit redispatch.

D10/D11 implementation replaces seven positional arguments with named config and
migrates all real callers. Current same public typednil port probe passes -race
count3; AAA config/snapshot/recovery tests pass count3. Root/human/mail race PASS;
human/mail lint0. Existing atomic journal transitions unchanged; recovery still
requires original unexpired consumed grant, AllowRecovery and fenced reconciliation.
Store valid/expired/fresh-grant cases and real registry/profile handler counter
confirm no new dispatch after fresh/expired refusal. Runnable local example:
challenge -> bound approval -> restart replay, one physical receipt. Updated current
contract/migration gives audited host new-intent steps without grant replacement.
Final root lint and independent reviewers pending; no row27 commit/acceptance yet.

D10/D11 final root lint0 and latest contract/config/recovery race count3 PASS1.712s.
AST search confirms no remaining seven-argument real constructor call. Journal,
reconciliation and operation contract source bytes equal baseline (only constructor
and real caller API changed). Root/human/mail checks and physical-one-receipt CLI
proof saved; independent acceptance starts against current diff. No row27 commit.

Independent acceptance A and B each100%, all five criteria20/20, no unresolved
detected errors. Reports saved in task41/d10-d11/acceptance-a.md and acceptance-b.md.
A independently exercised inclusive display/encoded-result caps and default1MiB
boundaries with a retained overlay fixture; race count3 PASS. Both reproduced
baseline typednil failures, current race/lint and restart replay with one receipt.
Accepted for separate signed commit `refactor: operation config`.

### 28 — D14 (accepted)

Criteria for both independent reviewers, 20% each:
1. Retain bounded local reference adapter; state full-image transaction/inspect
   time and memory complexity and serialized locking, default/inclusive quota.
2. Restore validates both directions of record/grant/consumed links, including
   missing grant/reservation, wrong binding/expiry, and inconsistent grantless records.
3. AAA corrupt snapshot and durable journal regressions reject before permission
   and preserve persisted bytes; legitimate approved/unapproved/recovery states restore.
4. Host maintenance guidance preserves uncertainty, consumed grants, replay and
   downstream idempotency; no automatic record eviction/retention API/integrity promise.
5. Contract/migration consistent; targeted/full affected race and pinned lint pass;
   both independent reviewers100% and no unresolved detected errors.

Spec-first: grant-bearing records must have matching immutable grant and consumed
reservation pointing back to their exact key; approval expiry must match. Grantless
records have zero approval expiry. Existing forward link validation remains.
Plain JSON validation establishes structural consistency only, not authentication.
Local journal retains whole-image transactions under persistent advisory lock;
maintenance belongs to authenticated host with quiesced writers and safe backups,
never automatic deletion/reinitialization after quota or uncertain outcomes.

Implementation adds reverse grant-bearing record checks and grantless expiry check,
retaining forward validation. Baseline new tests showed five behavioral failures;
current operation/recovery/normative/journal race count3 PASS2.103s/6.542s, lint0.
Durable malformed-image claim/inspect fails with preserved original bytes; underlying
OperationError is inspected through sanitized store error. Five legitimate states
with/without approvals restore. Full root race and independent acceptance underway.

Full root race PASS3.113s/filejournal2.993s; unchanged packages cached.

Independent A/B found Claim(false approval, nonempty grant) could produce a snapshot
rejected by the new restore. Fixed pre-dispatch invalid_claim validation and clear
migration; unapproved callers clear irrelevant grant IDs. Recovery cannot switch
approval mode and erase an old reservation (binding_mismatch). AAA memory/durable
checks assert rejection before mutation, valid unapproved finish/reopen and retained
consumed recovery links. Both independent acceptances must repeat final diff.

Final revised targeted race count3 PASS2.062s/filejournal6.697s; full root race
PASS4.523s/filejournal3.185s and pinned lint0. Real API-generated five-state snapshots
include expired historical approval; both independent final reviews in progress.

Independent final A and B each100%, five20/20 criteria, no unresolved detected
errors. Both independently verified revised public API probes, complete uncached
root race and targetedrace3/pinnedlint0; retained reports in task41/d14/acceptance-*.md.
Accepted for separate signed commit `fix: journal consistency`. Local Restore
benchmark10/100/1000 approvedrecords observed allocations77KiB/810KiB/8.3MiB;
retained fixture/log, concurrent timings are not a performance guarantee.

### 29 — D23 (accepted)

Criteria for both independent reviewers, 20% each:
1. Filter allowed/non-reflection service names before any descriptor request;
   excluded-service reflection errors cannot fail selected discovery; deduplicate names.
2. Explicit finite inclusive aggregate discovery service-entry, descriptor-blob and
   protobuf response-byte budgets; zero defaults, negative rejection before RPC.
   Count excluded/duplicate entries and repeated descriptor bytes to bound work.
3. Selected service dependencies retained; consistent duplicate files deduplicate,
   conflicting file identities reject; no partial publication on limit/unsupported/error.
4. AAA real reflection fixtures for excluded failures, exact/+1 limits, repeated
   blobs, selected dependencies and cancellation; affected module race/lint pass.
5. README/migration/options agree, host owns connection/deadline/authority;
   per-message receive allocation limitations explicit; both reviewers100%, no
   unresolved detected errors; no new core dependency or reflection scheduler.

Spec-first: Services is a pre-fetch allowlist, not a post-download tool filter.
Default finite budgets: 256 listed service entries (including exclusions), 512
received descriptor blobs (including duplicate files/dependencies), 8MiB aggregate
protobuf response size across list and descriptor responses. Positive caps inclusive;
zero uses default, negative fails construction before reflection stream. Aggregate
bytes count canonical protobuf encoded response size (including envelope/unknowns),
not transport headers/compression. Per-message gRPC receive cap also applies before
adapter processing; it cannot promise arbitrary server/resource conformance.

D23 implementation and docs complete: prefetch allowlist/dedup, finite service/blob/
aggregate protobuf response caps, per-message gRPC receive cap, negative validation,
conflicting same-name files reject, child stream cancellation without closing cc.
Real reflection fixture baseline failed due to excluded-service fetch; current full
module race count3 PASS5.308s and pinnedlint0. Exact/+1 quota, repeated descriptor,
selected dependency/conflict and cancellation/borrowed connection tests pass.
Two independent acceptance reviewers pending; no row29 commit yet.

Independent final A and B each100%, five20/20 criteria; no unresolved detected
errors. A module race3PASS4.796s/adversarialrace3PASS1.965s/lint0; B module race3
PASS4.742s/additionalrace3PASS1.585s/lint0. A checked malformed/nil/missing imports/
unknown bytes/defaults/max-int and owned stream cleanup; B independently reproduced
baseline, checked canonical envelope+unknown accounting and success-path cleanup
when server ignores CloseSend. Retained reports/probes in task41/d23; local fixtures
are not live remote conformance/process-heap proof. Accepted for signed separate
commit `fix: reflection discovery`.

### 30 — D24 (accepted)

Criteria for both independent reviewers, 20% each:
1. Public names/docs distinguish remote task adapter and toolsy-step-stream-v1
   SSE/cancel extension from normative Agent Protocol; no remote scheduler added.
2. Optional host-only cancellation observer reports task, parent interrupt cause,
   credentials/request stage, acknowledgement and exact failure cause, once per
   attempted interrupted-task cleanup; no credentials/diagnostics in model output.
3. Parent cancellation triggers bounded detached best-effort cancel. Stream-owned
   timeout and callback stop do not cancel remote action automatically; preserve
   their primary error/retry/outcome semantics and borrowed client/host ownership.
4. AAA local HTTP/SSE tests cover success acknowledgement, HTTP/credential failure,
   optional observer absence, caller cancel/deadline versus stream timeout/callback
   stop, original-error preservation and bounded cooperative cancellation context.
5. README/API/migration agree on synchronous cooperative observer limits, concurrency
   and cause trust; affected module race/lint pass, two independent reviewers100%,
   no unresolved detected errors, no live remote cancellation guarantee.

Spec-first: cancellation diagnostics are optional host observability, not a tool
result, remote completion proof or permission to retry creation. A cancelled parent
continues to drive one best-effort request under a fresh five-second context; the
observer receives that context and must cooperate (no forced goroutine/preemption).
Diagnostics preserve parent context cause separately from credential/cancel-request
failure and the acknowledged flag. Stream-owned timeout/callback stop remain read
interruptions, with no implicit remote cancellation or background scheduling.

D24 implementation complete: optional host CancellationObserver, exact parent
interrupt/customcause, credentials/request stage, acknowledgement and cleanupcause.
Old parent-only detached5s cancellation remains, stream-only timeout/callback stop
with active parent cause no remote cancel; primary error stays intact. Docs distinguish
custom SSE/cancel profile and synchronous cooperative callback/authority limits.
Final complete agents race count3PASS5.638s/pinnedlint0. Initial timeout fixture
stalled at server.Close without reading POST body; corrected fixture drains body and
explicitly releases cleanup, final run passes. Two independent acceptance pending.

Independent final A and B each100%, five20/20 criteria, no unresolved detected
errors. A fullagentsrace3PASS6.491s/independentconcurrent-budgetprobePASS6.983s/lint0;
B fullagentsrace3PASS5.707s/independentbudget-combinedprobePASS6.879s/lint0. Both
inspected latest callback-cause docs: existing core ErrStreamAborted wrapping keeps
original cause through errors.Is/errors.As. Concurrent12-client calls, actual5s
cooperative credential deadline, expired observer context, parent values and combined
callback+parent interruption verified. Reports/probes retained in task41/d24.
Accepted for separate signed commit `feat: cancellation diagnostics`.
