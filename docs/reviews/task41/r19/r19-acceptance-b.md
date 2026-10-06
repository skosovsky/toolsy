# R19 / D35 scraper independent acceptance B

Reviewed current tracked/untracked row19 diff against HEAD 71b49fa, original Task41 R19 and D35, five ledger criteria, complete scraper/library/tool flow, options/README/migration and markdown_cause_test.go. No repository files edited.

| Criterion | Score | Evidence |
|---|---:|---|
| Public default/custom Markdown cap preserves semantic/custom cause and safe validation reason | 20/20 | Public library/tool tests cover default expansion, joined custom diagnostic/cap and unchecked custom output. markdownLimitError retains ErrValidation and original cause with errors.Join while Reason uses configured byte cap. Source text/custom diagnostics do not become public Reason. |
| Unchecked output, independent source/extraction/wire bounds | 20/20 | Oversized nil-error output receives WrapMarkdownExceedsLimit cause. Source/wire failures lack Markdown sentinel. Independent external overlay verifies source8/extraction1 exact success, library with wire1 unaffected, exact JSON wire success and wire-minus1 failure without sentinel. |
| Cancellation precedence and synchronous checkpoints | 20/20 | Permanent tests verify current-context cancellation with nil callback error and active-context joined interrupt/cap cause. Independent tool overlay covers joined context.Canceled/context.DeadlineExceeded/toolsy.ErrTimeout while ctx stays active: original interrupt errors.Is-compatible, no chunks and no client-correctable classification. Checks surround custom call and default layout/converter work; no callback goroutine. |
| Honest API/docs and bounded cooperative conversion decision | 20/20 | Latest options.go explicitly describes derived defaults and independent overrides, library has no final wire cap. Scraper/API/README/migration explain cooperative synchronous callbacks, bounded input/returned-output, no CPU preemption/intermediate allocation promise, host sandbox for hard bounds. |
| Regression proof, independent race/lint, no unresolved detected defects | 20/20 | Baseline public log fails all six library/tool/default/custom/unchecked sentinel cases. Current complete web race passes; pinned lint zero issues; adversarial external overlay count5 under race passes. |

Total: **100% (100/100)**. Verdict: **ACCEPT**. No unresolved detected errors within R19/D35 scraper scope.

Independent verification:
- /tmp/toolsy-task41/r19-b-race.log: full web module go test -race -count=1 ./..., PASS 1.996s.
- /tmp/toolsy-task41/r19-b-lint.log: golangci-lint v2.14.0, private cache and --allow-parallel-runners, 0 issues.
- /tmp/toolsy-task41/r19-b-adversarial-final.log: public API overlay exact source/Markdown/wire caps, library wire isolation, tool interrupt/cap cause precedence; -race -count=5, PASS 1.717s.
- /tmp/toolsy-task41/r19-baseline-public.log: old public API failures reviewed, not rerun.
- git diff --check: clean.

An initial independent overlay assertion incorrectly required INTERNAL at the outer Tool.Execute boundary for all cancellation causes. Existing core wrapHandlerError deliberately unwraps context.Canceled and maps deadline/ErrTimeout to TIMEOUT, preserving errors.Is. This is correct interrupt precedence, not a scraper defect. Corrected assertion requires cause preservation/no correctable validation/no chunks; final overlay passes. Initial raw log retained for transparency.

Limits: local HTTP fixtures and cooperative callbacks only; no hostile-converter CPU/memory preemption claim, no real internet service proof, no future distributed/host retry semantics guarantee. Conversion remains synchronous; post-callback checks cannot stop an uncooperative callback while running. No judgment about other remediation rows. Overall two-reviewer gate remains parent responsibility.
