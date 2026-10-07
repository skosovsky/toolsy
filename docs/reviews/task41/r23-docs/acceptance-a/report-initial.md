# R23 / row39 independent acceptance A — initial

Baseline: 0600e12. Reviewed current tracked diff and new docs/fixture/example files directly. No implementation edits, commit, push or signing. Did not consult the other row39 acceptance report.

Verdict: **not accepted, 80% (4 × 20%)**. Criterion5 remains blocked by a reproduced release bootstrap failure; no deadline/policy regression found in row39 code.

| Criterion | Score | Evidence |
| --- | --- | --- |
| 1. Generic deadline/resource contract | 20/20 | exectool/doc.go and README, root README and sandbox deadline table distinguish parent execution context, owned Docker/host collection and cleanup clocks, finite Starlark steps. No RunRequest/schema timeout field; runtime source diff is documentation only. |
| 2. Capability links/host recipe/Docker phases | 20/20 | Public Starlark policy uses DefaultConfig, positive1000steps, positive2s caller context and exectool.New; exact-output, pre-cancel and no-deadline step-exhaustion tests pass. Actual LogTimeout clock fixture preserved; phase-triggered synthetic deadline explicitly identified as lifecycle/classification proof. Docker defaults5s, starts log acquisition after start, collection infrastructure classification and independent cleanup match code. |
| 3. Source docs checklist1–2 | 20/20 | Public API family map matches actual factories/Session/state/cache/reconciliation APIs and links schemas/results/control/policy contracts. Inspected result/execution/session ownership references and executable Rebind, codec-reentry, run-policy and post-handler fixtures; root race runs those successfully. BYOT/opaque/borrowed synchronization and snapshot scope are explicit. |
| 4. Source docs checklist3–7 | 20/20 | All seven source bullets preserved; links map actual HTTP origin/pinning/encoding/pool, MCP retirement/discovery budgets, exact sandbox bytes/lifecycle, generator presence/rollback/stream/async and release local repository tests. Checked source/test names and current reference statements. Historical audits and row40 final gate remain separate; Docker/E2B live and remote/custom-port limitations stated. Release fixture failure is disclosed below rather than counted as successful current proof. |
| 5. Relevant verification/no unresolved error | 0/20 | Root race fails TestReleasePreservesUntrackedSource with exit141; independent unsandboxed internal/release race1 repeats the failure. Thus verification cannot pass and no100% acceptance. Relevant lint and sandbox/policy races otherwise pass. |

## Checks

Environment: GOWORK=off GOCACHE=/tmp/toolsy-review-gocache; /opt/homebrew/bin/golangci-lint v2.14.0, run --allow-serial-runners ./... from each module workdir (no lint -C).

- Root go test -race -count=1 ./...: FAIL only internal/release TestReleasePreservesUntrackedSource exit141; other listed packages including internal/toolsygen pass.
- Independent unsandboxed go test -race -count=1 -v ./internal/release: FAIL same fixture exit141, other release cases pass; disposable repositories/local bare remotes only.
- exectool, Docker, Starlark module race1: PASS, including new policy example tests.
- httptool, MCP, host module race1: PASS. Wazero/E2B verification running when this initial report was written.
- Root/exectool/Docker/Starlark lint: PASS0issues. Lint emits cache persistence warnings for restricted user cache; analysis completes successfully.
- Public policy go -C adapters/sandbox/starlark run ./examples/policy: PASS exact bounded newline stdout in JSON, empty stderr, exit0.
- Current linked local source/fixture paths and anchors checked in new reference documents: PASS. git diff --check0600e12: PASS.

## Scope and limits

No24module whole-candidate gate, benchmark campaign, published release, live Docker/E2B or Windows certification. Parent setpgid sandbox diagnosis alone was insufficient: independent unsandboxed failure remains unresolved. Release failure predates row39 runtime source but cannot be waived under its explicit criterion5. New synthetic context is a fixture convenience, not elapsed execution-clock measurement. True Docker collection timeout remains an actual clock test.

Logs are in this directory. Go cache is separate and must not be archived with evidence.
