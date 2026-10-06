# D04 / D05 gate verification

Production source at f5824b1 was unchanged when the baseline probe executed. Source hashes
and exact baseline commit are retained in baseline-sources.json. The external
public API overlay uses only existing WithPolicy/WithBudget/registry/tool methods.
Allthree behavioral regressions fail, without compile failure: explicit nil policy
accepted; missing budget and typednil budget both dispatch handler once with nil
error. The same fixture now passes on the required-gate implementation with zero dispatch.

Reproduce in a checkout of the recorded commit by mapping an otherwise nonexistent
root d04_d05_probe_test.go to the retained .go.txt fixture using go test -overlay,
then run -count=1 . -run '^TestD05PublicGateMisconfigurationProbe$'. No production
push/publication performed. Baseline/current logs establish behavior; they do not replace independent acceptance.


Current validation (all exits 0):
- Root `go test -race -count=1 ./...`: PASS, including executable example.
- Root pinned golangci-lint 2.14.0: 0 issues.
- MCP, toolkits/human and ext/toolsyotel `go test -race -count=1 ./...`: PASS.
- Targeted gate/cancellation/replay/async tests: PASS under race.
- Original external public overlay: PASS on current production source.

The root race log predates only an equivalent example error-constructor style
change (fmt.Errorf literal replaced by errors.New); final lint compiles that
example. Independent reviewers completed their reviews against the current tree. MCP/human/OTel module lint reports 0 issues. A and B each accepted 100% (five criteria20/20), no unresolved detected defects; see acceptance-a.md and acceptance-b.md. No live pricing/auth service conformance is
claimed; callbacks are host ports and arbitrary BYOT references remain host-owned.
