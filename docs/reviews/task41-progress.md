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

Row 01 is accepted and committed as `4dec511`; row 02 is accepted and committed as `d1619e8`; row 03 is accepted and committed as `322ec4a`; row 04 is accepted and committed as `4f372b2`; row 05 is in progress; rows 06–40 are pending. Associated D decisions are recorded in their row's
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
was never used. Commit: pending final accepted commit.
