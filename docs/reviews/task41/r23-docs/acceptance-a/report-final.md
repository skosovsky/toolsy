# R23 / row39 independent acceptance A — final

**Accepted: 100%, five criteria20% each.** No unresolved detected defect in the reviewed row39 scope. Parent must combine this independent verdict with the separate reviewer; it is not whole-task/row40 acceptance. I did not read reviewer B's row39 results.

Baseline0600e12; reviewed the complete current tracked diff and new deadline fixture, public Starlark recipe/tests, reference docs and archive-padding regression. No source edits, Git commit/push/signing by this reviewer.

| Criterion | Score | Independent evidence |
| --- | --- | --- |
| 1. Generic deadline/resource contract | 20/20 | exectool package/README and root README distinguish caller context from backend-owned collection/cleanup clocks and computation/resource bounds. No generic RunRequest or model schema timeout. Backend runtime source unchanged; exectool source delta is comments. |
| 2. Capability links/policy recipe/Docker phases | 20/20 | Starlark recipe uses public DefaultConfig/New/exectool.New with explicit positive1000step budget and2s context; exact output, pre-cancel and no-caller-deadline step exhaustion execute successfully with race. Docker default5s LogTimeout begins after start during log acquisition/follow, can stop an active guest and classifies infrastructure collection failure absent parent interruption. Fresh cleanup context/IO limitations match implementation. Synthetic phase-triggered deadline is explicitly distinguished from elapsed-time proof; separate real LogTimeout clock fixture remains intact. |
| 3. Source checklist1–2 | 20/20 | Actual public execution families/schemas/authoritative wire vs typed results/authority/audience/control/effect/error/retry/lifecycle link to current contracts and executable fixtures. Session state/config/Rebind/checkpoint and codec freeze/reentry/borrowed BYOT ownership consistently documented. Inspected corresponding APIs and tests; root race executes post-handler, envelope/cache/approval/terminal, Rebind and codec fixtures successfully. |
| 4. Source checklist3–7 | 20/20 | Seven original source bullets all retained and mapped to actual HTTP origin/host/pinning/encoding/settings/pools, MCP retirement/discovery/budgets, sandbox bytes/cleanup/descendants, generator DTO/rollback/runnable stream-async consumers and release checkout/artifacts/explicit refs. Linked fixture paths and names exist; direct module races execute relevant corpus. Mocks/live/hostile ports/OS limits are honest. Historical scores and row40 full-scope gates remain separate. |
| 5. Current verification/migration/no unresolved defect | 20/20 | Corrected independent root race1 passes; relevant nested races, root/module lint and policy example pass. Migration/checklist/criterion5 include discovered bootstrap SIGPIPE and its regression. Root full suite executes new padding fixture and untracked preservation with corrected script. Baseline overlay padding test fails exit141, proving behavioral regression. No policy bound relaxed. |

## Discovered defect and closure

Initial root race reproduced TestReleasePreservesUntrackedSource exit141. Independent unsandboxed internal/release race1 reproduced it too, so it could not be waved away as a sandbox failure. Initial80% rejection is preserved in report-initial.md and initial logs.

Root fixed scripts/release.sh: write the complete committed Git archive to a private temporary file, extract that file, remove it. This eliminates git producer SIGPIPE when tar stops at its end marker before draining trailing padding. Reviewed that attribute preflight, committed input, checkout ownership, failure propagation, cleanup, build cancellation and publication scope remain intact. AAA TestReleaseBootstrapDrainsArchivePadding injects16MiB valid padding through a private Git shim, runs prepare-only actual artifact verification, asserts source unchanged and no new remote tag. Own Go overlay substitutes baseline0600e12 script only inside disposable bootstrap fixture; that new test fails with exit141. Corrected root race proves current success.

## Own checks/logs

All Go checks use GOWORK=off GOCACHE=/tmp/toolsy-review-gocache. Lint binary /opt/homebrew/bin/golangci-lint v2.14.0, run --allow-serial-runners ./... in actual module workdir; no lint -C.

- Current root go test -race -count=1 ./... (unsandboxed for process-group release fixture): PASS, root-race-final.log; includes internal/release, generator emitted consumers and examples.
- exectool/Docker/Starlark module go test -race -count=1 ./...: PASS; final Docker repeat after AAA comments PASS.
- httptool/MCP/host/Wazero/E2B module race1: PASS; actual WASI tests included, live Docker remains opt-in SKIP and E2B remains mocked.
- Root/exectool/Docker/Starlark lint: PASS0issues. Initial lint cache persistence warnings did not stop analysis; final root uses separate temporary GOLANGCI_LINT_CACHE and clean0issues output.
- go -C adapters/sandbox/starlark run ./examples/policy: PASS JSON exact stdout bounded newline, empty stderr, exit0.
- Baseline script overlay TestReleaseBootstrapDrainsArchivePadding race1: expected FAIL141, bootstrap-baseline-failure.log; no current source edits.
-194 local relative documentation links across7current files: no missing paths; new references' anchors checked; bash -n scripts/release.sh and git diff --check0600e12 PASS.

## Limits

This is row39 acceptance, not24module final task41 certification. No benchmark campaign, production publish, live Docker/E2B, Windows or arbitrary remote/custom-port conformance. Release tests use disposable repositories/local bare remotes; policy recipe executes an effect-free local print. No universal hard cleanup/syscall/foreign-callback preemption, memory isolation or exactly-once external-effect claim. Runtime caches are outside this evidence directory and should not be archived.
