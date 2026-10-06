# R14 / D28 independent review B — initial rejected

Base: 1180b3a. Independent review of source task, row 14 criteria, implementation and contract.

Criteria (five 20%):
1. 20/20: ownership stop-before-removal, unreaped supervisor signal ordering, cancellation watcher joined before Wait; group observation uses signal0 after reaping. Escape and conservative reuse/zombie limits documented.
2. 15/20: output/exit and bounded WaitDelay/cleanup present, but a missing absolute executable returns successful guest127 instead of infrastructure error.
3. 10/20: completed cleanup outcome retained, cancellation/timeout/readlimit classifications preserved, but unsupported-language Run error loses outcome wrapper in mapExecError.
4. 20/20: AAA completion/cancel/leak tests cover inherited/redirected output and fixture child cleanup; public baseline child-leak evidence available.
5. 10/20: affected race tests pass; pinned lint fails noctx and shared CleanupError literal exhaustruct. Platform compile checks pending.

Overall 75%, NOT ACCEPTED.

Confirmed failures:
- start-probe.log: TestR14MissingAbsoluteExecutableIsInfrastructureFailure fails current with nil error, exit127. start-baseline.log same public probe passes baseline.
- outcome-probe.log: TestR14OutcomeRetainedForUnsupportedRun fails because errors.As(*RunOutcomeError) false.
- Host pinned lint noctx process_unix.go exec.Command; core-lint.log shared internal/sandboxfs CleanupError lacks ResourceID literal.

Passing independent checks: host-race.log PASS12.616s, core-race.log exectool and sandboxfs PASS. No repository edits performed; all probes use overlays.
