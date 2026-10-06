# R19 / D35 scraper — independent acceptance A

Reviewed working tree against HEAD 71b49fa, original TLS-R19/D35, row 19 contract, tracked implementation/docs/options diff and untracked markdown_cause_test.go. Read-only source review; wrote only private /tmp probes and this report. No implementation edits or commits.

Verdict: ACCEPT — 100%. No unresolved detected defects within the row contract.

| Criterion | Score | Evidence |
| --- | --- | --- |
| 1. Public default/custom semantic overflow retains sentinel and cause with safe validation reason | 20/20 | Public library/tool matrix independently run in full race; private helper joins ErrValidation with original cause. Reason only configured cap; no output emitted. Baseline six old-public-API cases fail sentinel assertions, proving regression relevance. |
| 2. Unchecked custom output, independent inclusive budgets | 20/20 | Actual returned byte length checked separately; synthesized semantic sentinel only on Markdown overflow. Source/wire negative discriminator cases pass. Independent adversarial probe proves exact three-byte UTF-8 source/extraction success and two-byte extraction rejection. No wire cap or byte slicing removed. |
| 3. Interrupt precedence and synchronous cancellation checkpoints | 20/20 | Guards before/after callback and around default layout/conversion. Independent live-context joined Canceled, DeadlineExceeded and ErrTimeout causes retain original diagnostic and use noncorrectable INTERNAL. Callback cancellation with nil error and under-budget output fails closed. Direct pre-canceled default conversion fails with compatible cause. No spawned converter goroutine. |
| 4. API, README, migration accurately document cooperative limits | 20/20 | Reviewed corrected latest options.go comment: derived defaults vs explicit independent limits, library no final JSON cap. README/migration/API correctly avoid hard CPU deadline/intermediate allocation guarantees, require cooperative custom callbacks, preserve semantic vs source/wire distinctions. |
| 5. AAA regression evidence, affected race/lint, independent acceptance | 20/20 | Public AAA tests and independent overlay probes pass; full web race and pinned lint pass. This is one independent acceptance; orchestrator must separately obtain acceptance B before commit. |

Independent commands/results:
- GOCACHE=/tmp/toolsy-review-gocache go test -race -count=1 ./... in toolkits/web: exit 0, 1.969s. Log /tmp/toolsy-task41/r19-a-race.log.
- Pinned /opt/homebrew/bin/golangci-lint run --allow-parallel-runners, dedicated /tmp/toolsy-task41/r19-a-lint-cache: exit 0, 0 issues. Log /tmp/toolsy-task41/r19-a-lint.log.
- go -C toolkits/web test -race -count=5 -overlay=/tmp/toolsy-task41/r19-a-overlay.json -run '^TestR19A': exit 0, 1.637s. Log /tmp/toolsy-task41/r19-a-adversarial.log. Probe /tmp/toolsy-task41/r19-a-adversarial_test.go.
- git diff --check: clean on latest docs/options/source tree.
- Inspected /tmp/toolsy-task41/r19-baseline-public.log: six expected public library/tool failures at sentinel assertions on baseline.

Limits: finite source/output bounds and checkpoints do not bound converter intermediate allocations or preempt an uncooperative custom callback. No live external network integration or universal bug-free claim. Cancellation may preserve a joined Markdown sentinel in its cause chain while the outer classification remains INTERNAL, as required by original cause preservation. No additional work delegated.
