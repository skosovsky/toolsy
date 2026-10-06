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

Row 01 is accepted and committed as `4dec511`; row 02 is accepted and committed as `d1619e8`; row 03 is accepted and committed as `322ec4a`; row 04 is accepted and committed as `4f372b2`; row 05 is accepted and committed as `5f80c9e`; row 06 is accepted and committed as `4ddcb4f`; row 07 is accepted and committed as `d1102f6`; row 08 is accepted and committed as `df733dd`; row 09 is accepted and committed as `9a9062a`; row 10 is accepted, awaiting its commit; rows 11–40 are pending. Associated D decisions are recorded in their row's
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
