# Independent acceptance B — row39 R23 and seven-item docs checklist

Candidate: current working tree against signed baseline `0600e12`. Review only; no implementation edits, commit, push or signatures. Reviewer A report/results were not read. Read original task41 R23/documentation checklist, row39 five equal criteria, current diff/untracked recipe/reference files and relevant source/fixtures.

## Score and verdict

Current result: **80%, NOT ACCEPTED while release test exit141 is unresolved**.

| Criterion | Score | Evidence |
| --- | --- | --- |
| 1. Generic deadline/collection/cleanup/resource contracts | 20/20 | README/exectool README/doc.go explicitly pass caller execution context, separate owned bounds, deny universal hard return bound. RunRequest has only language/code/env/files; exec schema additionalProperties=false and no timeout. Public probe verifies timeout input rejects before backend call and caller positive deadline is forwarded unchanged. |
| 2. Host policy recipe and Docker phase/classification | 20/20 | Runnable Starlark public recipe uses 2s caller context plus explicit1000steps. Output is exact bounded newline. Recipe race tests check pre-cancel and step exhaustion as sandbox failure rather than time deadline. Docker default LogTimeout5s is established in source, acquisition/follow begins after start and can terminate running guest. docs distinguish collection infrastructure failure, parent interruption and secondary cleanup. Real 20ms LogTimeout clock/body-close fixture remains; setup/wait fixtures use documented synthetic phase deadline instead of premature20ms caller clock. Own Docker phase race20 passes. No live claim. |
| 3. Checklist1–2 public API/state/ownership | 20/20 | API map identifiers confirmed in current public source; current result/prepared/gate/control/session references expose schemas, authoritative typed versus wire output, hints versus authority, pre/post-handler repair and no blind redispatch, cooperative lifecycle. Atomic Rebind config, validated constructor codec Freeze, callbacks outside state lock, shallow BYOT and external StateStore distinctions verified against source and named fixtures. |
| 4. Checklist3–7 current references/evidence/scope | 20/20 | Seven preserved source items map HTTP origins/pinned DNS/deny/UTF8/owned pools; MCP full authority and bounded cancellation; sandbox phases/bytes/group/live limits; generator subset/DTO/rollback/async; isolated tracked release/artifact graph/explicit atomic refs. Exact current fixture names resolve. Historical percentages and row39 are explicitly not row40 all24module/adversarial/bench/artifact/full acceptance. Source implementation supports stated contracts; release execution anomaly is kept under criterion5 and not hidden by docs. |
| 5. Checks/link/migration/current proof | 0/20 pending | Four pinned lint final0, Docker/Starlark/exectool race3 and policy/public probes PASS;161 local Markdown links/anchors PASS. Own root race1 and separately escalated release race1 both FAIL TestReleasePreservesUntrackedSource release exit141. No100% claim until the failure is explained and repaired or independently reproduced as a resolved harness fault. |

## Commands and raw logs

All Go checks use `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache`; lint is `/opt/homebrew/bin/golangci-lint` v2.14.0, run with module working directory (no `-C`) and `run --allow-serial-runners ./...`. Final lint adds writable `GOLANGCI_LINT_CACHE=/tmp/toolsy-r23-b-lint-cache`.

- root `go test -race -count=1 ./...`: root-race.log; core3.326s, filejournal6.434s, exectool3.872s etc PASS; internal/release64.458s FAIL141.
- separate `go test -race -count=1 ./internal/release` with require_escalated: release-race-unsandboxed.log;56.718s FAIL141. Disposable test repos/local bare remotes only.
- Docker `go test -race -count=3 ./...`: docker-race.log PASS3.323s.
- Starlark `go test -race -count=3 ./...`: starlark-race.log PASS11.937s, recipe1.561s.
- exectool `go test -race -count=3 ./...`: exectool-race.log PASS2.249s.
- targeted Docker phase `go test -race -count=20 -run 'Test(ActualLogDeadlineClosesBodyAndRemainsInfrastructure|RunKillsContainerOnTimeout|RunReturnsTimeoutDuringSetup|OwnedLogBodyClosedOnCancellation|CollectionDeadlineRemainsInfrastructureFailure)$' ./...`: docker-phase-race20.log PASS2.215s.
- four lint-final logs:0issues, no issues hidden behind cache failures.
- `go -C adapters/sandbox/starlark run ./examples/policy`: policy-example.log exact output bounded\\n/exit0.
- `go -C adapters/sandbox/starlark run -race /tmp/toolsy-task41/r23-acceptance-b/public_probe.go`: public-probe.log PASS schema/deadline/language/mandatorysteps.
- docs-source-audit.log:161 target/anchor checks, all14 named fixtures found. Tracked non-test Go diff is solely exectool/doc.go; no runtime policy source change from0600e12.

## Limitations / harness issues

Initial lint attempts used unwritable global cache and emitted fact-persistence warnings; final runs with separate/tmp cache passed0issues. Initial documentation audit matcher incorrectly considered prose `Tests` an executable fixture; corrected exact Test[A-Z] matcher passes, no product defect. Root release141 is NOT treated as environment-only merely because an earlier parent run mentioned sandbox setpgid: own escalated suite also fails and remains unresolved.

Docker live remains SKIP; no E2B remote service/isolation, Windows/all-platform or hard synchronous IO preemption certification. No new all24modules benchmark/artifact campaign and no row40 final claim. Caches outside report directory are excluded from evidence.
