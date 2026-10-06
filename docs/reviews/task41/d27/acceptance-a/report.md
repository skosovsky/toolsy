# D27 independent acceptance A

Baseline HEAD: fb7a2bf. Reviewed working-tree D27 including the final raw-plus-canonical duplicate-path correction. No implementation/repository/git/go.work.sum edits were made by this reviewer. No peer review report was read.

Verdict: accepted, 100%. Unresolved concrete errors: none.

## Five criteria

1. Literal executable/Args and one serialization boundary: 20/20. Session exposes command and args separately (e2b.go:34–40); builtins contain literal executable plus argv. The parser/tokenizer/encoder and their shell-policy dead paths are removed. normalizeRuntimeArgs validates exactly one raw script arg and exactly one canonical arg after rewriting (e2b.go:139–174). No adapter encoding, splitting, joining or expansion remains.
2. Validation/materialization/ownership: 20/20. New rejects malformed executable, script and argv before provisioning; script upload uses the same canonical ScriptName as argv. WithRuntime copies Args at option creation (options.go:31); normalization copies at construction (e2b.go:145); Run copies at each client dispatch (e2b.go:216). Independent public probes preserve leading/trailing executable whitespace, argument newlines/tabs/empty string/metacharacters/Unicode/nonbreaking-space path and embedded script text. They mutate both original config and actual client-received argv, reuse one option across two sandboxes, and exercise 64 concurrent runs plus a subsequent run without races or config drift.
3. Authoritative output/lifecycle: 20/20. CommandResult has only ExitCode; both capped writers remain authoritative and no fallback exists (e2b.go:44–47,211–230). Existing primary interruption/transport/cleanup paths are unchanged apart from argv dispatch. Independent malicious-client probes ignore writer errors and return a nonzero exit: exactly 256KiB in each stream succeeds with complete bytes/exit, 256KiB+1 fails closed through ErrReadLimitExceeded and no uncapped output leaks. Fresh bounded cleanup and primary-error preservation are covered by the retained race-run suite.
4. Regressions/validation: 20/20. Module AAA argv/ownership/malformed config regressions and retained output/cancel/cleanup tests pass; public ExampleNew executes. Independently repeated final module race count3 and pinned lint after the duplicate-path correction. Additional public adversarial probes pass race count3. git diff --check clean.
5. Docs/API/migration and limits: 20/20. README, Runtime/Session comments, migration and external-package ExampleNew agree on the deliberate API break, no prequoting, host/client transport serialization ownership, canonical uniqueness and capped-only output. Documentation explicitly limits cooperative cancellation and cloud isolation/destruction claims; trusted program flags/script semantics remain host policy. This verdict reports reviewer A only; the two-reviewer gate is the parent's responsibility.

## Validation evidence

- `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go -C adapters/sandbox/e2b test -race -count=3 ./...`: PASS 16.694s, race-final.log.
- golangci-lint version 2.14.0 built with Go1.27.1, commit114493f; `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache GOLANGCI_LINT_CACHE=/tmp/toolsy-task41/d27-acceptance-a/lint-cache /opt/homebrew/bin/golangci-lint run --allow-parallel-runners ./...` from affected module: 0 issues, lint-final.log.
- `/tmp/toolsy-task41/d27-acceptance-a` public consumer module `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go test -mod=mod -race -count=3 ./...`: PASS 1.693s, probe-final.log. Source: probe_test.go, go.mod, go.sum.
- `git diff --check`: exit0.

Initial disposable consumer setup had unquoted paths with spaces and then absent go.sum triggered a sandbox sumdb-cache permission error. Corrected only disposable go.mod and copied the existing module go.sum into the disposable directory; final test succeeded without escalation or repository changes. The intermediate probe.log preserves that tooling failure, not an implementation defect.

## Limits

D27 is a specified clear break, not an R/P1 behavioral defect. No baseline compile-only failure is claimed as behavioral evidence. Local seams establish adapter argv/output/error behavior; they do not certify a real E2B cloud SDK's serialization, isolation, cancellation or destruction. Concurrent executions use independently-owned argv, while the client itself and programs remain host-selected cooperative capabilities. No benchmark/module-graph/release verification is required by this scoped change.
