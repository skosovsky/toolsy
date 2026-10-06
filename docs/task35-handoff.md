# Task35 implementation and release readiness

## Subsequent dependency refresh — 2026-10-05

After the accepted Task35 snapshot below, the user requested a Go/dependency
refresh. All 24 modules and go.work now require Go 1.27.1; Makefile/CI use
golangci-lint 2.14.0 and the replacement exhaustruct_v5 with the existing checks
and exclusions preserved. All 64 external required module paths were checked for
latest updates; no available update remains in those paths. Local toolsy module
replacements and v0.0.0 development requirements remain local.

The resiliency example was migrated to the updated route-handler API; regression
tests cover handled payload, ignored/fallthrough rejection and error cause.
Go modernization keeps existing execution contracts; repeated literal error
kinds now use identical-valued internal constants. No compatibility shim or
runtime dependency on the example's routing library was added to core.

Current refresh checks: full all-module make test/race PASS (session72472),
all-module lint 0 issues (session99183), MCP preflight PASS, go mod verify PASS,
runnable resiliency example and its race tests PASS. One previous /dev/zero
timing test exceeded its 200ms bound under concurrent compilation; its unchanged
race/count3 repeat and the subsequent complete all-module gate pass. The Task35
independent reports below certify their recorded pre-refresh candidate, not a
new independent audit of this refresh. No release or GitHub write performed.

Checks use isolated writable GOPATH/GOMODCACHE/GOCACHE under
`/private/tmp/toolsy-deps-upgrade.sYzPkj` and the existing temporary build cache.
The lint wrapper selects the same pinned 2.14.0 runner and only adds
`--allow-parallel-runners` to avoid another project's global runner lock.
No test, module or lint rule is excluded by the wrapper. Final logs are
`test-confirmed.log` and `lint-complete.log` in that temporary directory.

## Accepted Task35 snapshot before the refresh

Source issue: https://github.com/skosovsky/toolsy/issues/3. This is an implementation
handoff, not evidence that release gates passed. See `task35-requirements.md` for
independently checkable criteria and `task35-activation.md` for deferred scope.

Current candidate: the final all-module test/race, lint and MCP clear-break
preflight gates pass after custom-pending cleanup and lint corrections.
MCP-COR-008 is independently closed. Both renewed whole-candidate audits pass;
zero open confirmed findings remain. Earlier checkpoints below are historical.

## Final card status

| Card | Delivery | Remaining acceptance |
|---|---|---|
| BUG-001/002 | Prepared result cache; current authorization; complete private outcome | Implemented; final gates and both audits PASS |
| TLS-001 | Bound approval, trusted issuer/clock, atomic durable reservation | Implemented; final gates and both audits PASS |
| TLS-002 | Scoped atomic claims/fencing, complete outcome, explicit host reconciliation | Implemented; final gates and both audits PASS |
| TLS-003 | Deferred: no confirmed script/tool consumer | Scoped bridge/converters/limits/control/restart and conformance remain unimplemented |
| TLS-004 | Deferred: no confirmed persistent workspace workflow | One selected backend lifecycle/isolation/handles/artifacts/cleanup remains unimplemented |
| TLS-005 | Deferred: no external server consumer/selected transport | Scoped server/transport/protocol fixtures and conformance remain unimplemented |
| TLS-006 | Deferred: no measured deployed large catalog | Scoped search/lazy resolve/refs/cursors and conformance remain unimplemented |
| TLS-007 | Deferred independently: no server requiring a remote profile | Authorization/elicitation/tasks runtime and conformance remain unimplemented |
| TLS-008 | Explicit independent/terminal modes, bounded schema/cardinality validation | Implemented; final gates and both audits PASS |

All121 requirements are independently accounted for;54 conditional requirements
remain deferred, none rejected. Final active implementation is66/67 (98.51%).
ARCH-10 remains an explicitly user-accepted historical process exception, not
retroactive implementation. Final acceptance is67/67 (100%) including that
exception; no partial/pending active criteria remain. Deferred work is not
counted as implemented or advertised as a delivered capability.

## Recorded Contract-First deviation

The task specification defined execution contracts before implementation, but
some executable fixtures were added after the initial implementation iteration.
Current consistency and passing fixtures cannot prove the required historical
order. The user explicitly accepted this historical-order deviation on
2026-10-04; ARCH-10 is marked accepted-deviation, not historically implemented.
Recent COR-006/007 corrections followed contract,
failing regression, then implementation; they do not retroactively erase the
earlier deviation. No timestamps or acceptance exceptions are invented.

## Integration changes for the issue author

- Replace the removed idempotency middleware with an explicit `ResultCache`
  partition and complete `ResultCodec`, or choose `OperationProfile` for effects.
  Use a new storage namespace; old cached bytes are not operation records.
- Generate stable logical operation IDs; preserve them on repeated delivery and
  distinguish new intentional actions. Never recycle an earlier attempt ID.
  Unknown/cancel/lease expiry never authorize automatic retry. Supply trusted
  reconciliation or a verified downstream idempotency contract with the original key.
- Inventory confirmation flags. Replace free-text approval as permission with
  bound challenge/grant, authenticated approver, trusted issuer/clock and an atomic
  durable store. Build redacted UI from canonical args; bind secret/dependency freshness.
- Keep current typed and wrapper policy checks. Custom protected tools require
  the prepared seam; unsupported bases fail closed. Wrapper policy sees final
  handler args, and replay never broadens a stored audience.
- Supply the restricted Registry/view/Session executor for nested calls, with
  explicit host context. Do not locate a root registry or reuse raw handler env
  as a scoped executor. Session budgets and RunPolicy remain enforced.
- Declare stream semantics explicitly. Terminal streams need schema/limits and
  separate progress/control/incomplete handling; provider input waits for completion.
- Treat replayed effects/control declarations as already delivered using
  `CacheReplayMetadata`; rebind current call correlation without applying effects again.
  Remove this library-owned key from wrapper metadata configuration. Dynamic JSON
  interface numbers replay as `json.Number`; use explicit conversions or a custom
  codec for arbitrary concrete Go types inside interfaces.
- There are no integration changes yet for workspace handles/cleanup,
  search refs/rediscovery or remote capability handles: TLS-003–007 are deferred,
  not advertised as implemented. Their activation conditions and remaining work stay explicit.

## Release and issue closure

These changes are substantial. After successful requirement-by-requirement
acceptance, both final auditors and **all** module test/race/lint gates, use
`make release-break` only after a separate explicit user command. Do not bypass
open findings or mandatory gates, or delete user files to obtain a green gate.

After implementation acceptance, close issue #3 only after a separate explicit
user command. The closing report must include verified/deferred status for each
card, this integration checklist, remaining limitations and any explicitly
authorized child-issue links. A task file, partial implementation or green root
suite alone is not grounds for closure. No release or GitHub mutation has occurred.

## MCP migration checkpoint — not final acceptance

Clear break is already authorized; no additional permission is needed to remove
obsolete API expectations. The immutable original test inventory and recovery
copy remain referenced in `task35-mcp-migration.md`.

Peer (15), stdio (11) and client-contract (23) original tests now have current
counterparts. The test transport has real prepared/pending lifecycle coverage.
New runtime regressions were fixed and independently rechecked: SSE EOF framing,
empty notification method, cumulative stdio limit precedence, progress retirement
at wire terminal, empty chunk MIME and preservation of consumer error causes.
These targeted proofs are not the final two-auditor review or all-module gates.

At that checkpoint seven original MCP files still needed migration. Full package compilation remained
red on obsolete test APIs; those files are not silently skipped in acceptance.
The current task remains active. Do not reuse an earlier percentage as final
acceptance after these MCP changes.

Read-only GitHub check on 2026-10-05: issue #3 remains OPEN and unchanged since
2026-10-03T16:14:44Z. Release and issue closure remain separately authorized steps.

## MCP migration continuation — protocol and features

Independent preservation audits now reconcile 82 original tests: peer15, stdio11,
client-contract23, protocol20 and client-features13. This is a test-migration
checkpoint, not requirement completeness or goal acceptance. Assertions that
could falsely pass on missing metadata were strengthened with valid-base controls
and cause-specific/typed failures; original 14 URI expectations remain unchanged.

MCP-COR-004 P2 empty-fragment subscription overscope was found, fixed and closed
by independent review: 28 adversarial URI cases passed race/count5. Protocol and
features scoped race/count3 passed, and all 30 currently compilable platform test
files passed the expanded diagnostic race/count1. Full MCP `go test ./...` was
rerun and still fails on obsolete adversarial API references. Five original files
remain: adversarial_contract, client_test, transport_http, transport_sse and
transport_stdio_contract. No acceptance gate excludes them. All-module gates and
both final whole-candidate audits remain pending; goal stays active.

## MCP migration continuation — HTTP and stdio lifecycle

Five original HTTP tests and 15 stdio lifecycle tests plus their subprocess helper
now compile against current preparation/discovery contracts. Scoped race/count3
passes for each suite. The expanded diagnostic race of all 32 currently compilable
platform test files passes; full MCP `go test ./...` remains RED on obsolete
adversarial symbols. Three original suites remain: adversarial_contract,
client_test and transport_sse. Their 95 original tests are not silently skipped
in acceptance.

Preservation audit of HTTP exact-cause assertions and stdio original scenarios
is pending. No runtime compatibility API was introduced. Active stdio write
cancellation no longer asserts the obsolete global-shutdown behavior; the
existing writer-verdict contract keeps unrelated requests alive, while explicit
Close owns process/writer cleanup. Invalid incoming UTF-8/server requests fail
closed instead of soliciting a removed error-response handler. Subprocess helper
exits explicitly, preventing Go harness PASS text from corrupting protocol stdout.

## MCP migration continuation — client25 and public boundaries

Independent completeness now reconciles 127/197 original migration tests plus
the subprocess helper. This is a scenario-inventory checkpoint, not active
requirement completeness. HTTP5 and stdioContract15 preservation audits are closed;
parent-visible exact-input peer send spies close the two stdio no-reply gaps.
Client25 preservation audit passed, retaining original limits, subjects, annotations,
raw cancellation ID/trace, unsupported selection and workspace negatives.

MCP-COR-005 P2 partial Start-resource cleanup and MCP-COR-006 P2 three public list
read-limit mapping omissions were reproduced, fixed and independently closed.
The common typed mapping preserves cause for diagnostics without exposing secret
text in Error/Reason; interrupts retain priority. New list-boundary public table
went RED before the fixes and passes afterwards. Client25 race/count3 passes;
independent boundary regressions race/count5 pass. Expanded diagnostic race of
all 33 currently compilable platform test files passes. Full MCP gate remains
RED on obsolete adversarial references. Two suites remain: adversarial51 and
legacy SSE19. Full all-module gates and both final whole-candidate audits remain
pending; release/issue closure were not performed and goal remains active.

## MCP migration continuation — SSE19 reconciliation

Clear break was authorized in the original task; no additional approval is
required for in-scope implementation and test-double migration. The historical
Contract-First ordering exception does not relax current architectural boundaries.

Independent preservation review reconciles 146/197 original tests plus the
subprocess helper after migrating all 19 SSE scenarios. Private-address checks,
HTTP status fixtures, cancellation/limit precedence and stream-volume limits are
preserved under POST-only request-scoped SSE. Removed endpoint behavior has exact
negative assertions for relative/absolute endpoint frames and malformed data;
extra negative cases do not inflate the original inventory denominator.
Both implementation and independent scoped SSE race/count3 checks PASS.

Only the 51 original adversarial scenarios remain to migrate. Full-package and
all-module gates and both final whole-candidate audits remain pending. This is
a scenario-preservation checkpoint, not final requirements acceptance.

## MCP migration continuation — adversarial37 reconciled

Of the original 51 adversarial tests, 37 are now independently reconciled in five
current-contract suites: peer12, HTTP/lifecycle17, SSE framing4, client3 and DTO1
(the DTO test retains all 11 original subcases). The aggregate original migration
inventory is 183/197 plus the separately retained subprocess helper. Remaining14
are still visible in adversarial_contract_test.go; full MCP go test ./... was
rerun and remains RED on their obsolete retry/GET/session/initialize APIs.

Race/count3 passes locally and independently for each migrated scope. The expanded
diagnostic race/count1 of all 39 platform-selected compilable MCP test files passes
(18.805s); this selected-file diagnostic is not the full-package acceptance gate.
An initial cross-platform file selection failed on Windows-only imports and was
corrected using Go's platform-selected TestGoFiles, without changing build tags.

CP-MCP-009 masked negatives are closed: 202 and exact media-type failures assert
their actual causes; original malformed UTF8 fixtures are retained, with a valid
correlated-envelope UTF8 negative and matching ASCII positive control added.
Strict DTO negatives now contain current mandatory fields and targeted positive
controls. Terminal consumer abort preserves both causes and emits no obsolete
cancellation after a completed response; active cancellation still asserts exactly
one notification with the actual request ID. No runtime compatibility layer was
added. All-module gates and final whole-candidate audits remain pending.

## MCP migration inventory complete — current candidate

Independent original-scenario ledger reconciles197/197 tests plus the subprocess
helper. Last14 source preservation and current negative dispositions pass actual
package race/count3. MCP-COR-007 scanner/read-budget P2 was reproduced, fixed after
README/task acceptance contract updates and independently closed by public
transport/Connect proofs and race/count5 controls. CP-MCP-010 fixture gaps are closed.

Full MCP race (not selected files) passes19.375s. make task34-preflight passes.
git diff --check passes. All-module make test is currently running. Normal make
lint failed on Go1.27 export data incompatibility in the pinned runner, not lint
findings; repository toolchain Go1.26.3 retry is running with unchanged Makefile,
rules and scope. No runtime/test scopes are excluded to make a gate pass.

Do not confuse197/197 migration-scenario preservation with the121-row requirements
matrix or active N/M. Final completeness/correctness audits must inspect this
candidate and actual all-module outcomes. No release or issue mutation performed.
