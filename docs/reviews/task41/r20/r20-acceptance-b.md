# R20 / D35 DOCX — independent acceptance B

Verdict: ACCEPT, 100%. No unresolved detected defects within the specified scope.
Review baseline: HEAD f63179a; current parser/doc diff and both previously untracked tests inspected. No implementation edits, commits, delegation or other reviewer verdict were used.

| Criterion | Score | Evidence |
|---|---:|---|
| Public whitespace, breaks and styled runs | 20/20 | Public tool fixtures pass for transitional, strict and empty namespaces with arbitrary q prefix; TAB, default/textWrapping/page/column br and cr become exact TAB/LF; styled runs produce styled without inserted spaces. Latest textWrapping fixture reread and independently rerun. |
| Inclusive bounded separators and retained guards | 20/20 | Separator helper checks context and existing byte count before each append; output on error is empty. Permanent exact/overflow Unicode, leading whitespace and paragraph tests pass. Independent overlay probes test UTF-8 plus combined separators at 6 bytes versus each cap 1–5 and 300 separator-only bytes versus 299; cancellation beats cap failure. Existing ZIP expansion/forged directory, source, item, wire and cancellation regressions pass in full module race run. |
| Exact supported namespaces and text subset | 20/20 | Single exact predicate accepts transitional, strict or empty only. Foreign URI substring and suffix cannot inject text or separators; independent strict/suffix namespace probe passes. Nested text remains rejected; complete OOXML validation is neither implemented nor claimed. |
| Contract, documentation and legacy removal | 20/20 | README, migration and row contract agree with actual whitespace, layout flattening and synchronous token/context checks. Existing paragraph behavior shares the one bounded separator helper. Substring namespace matching removed. Raw XML expansion remains bounded by ParsedBytes; no hard CPU or decoder intermediate-allocation guarantee claimed. |
| Reproducible AAA evidence, race and lint | 20/20 | Baseline public old-API log reproduces merged words in all three namespaces and foreign spoof extraction. Independent current full module race, repeated adversarial/public/helper race and final public fixture race pass. Pinned golangci-lint with private cache/allow-parallel-runners reports zero issues. |

## Independent execution

- `/tmp/toolsy-task41/r20-b-race.log`: go test -race -count=1 ./... in toolkits/document, PASS 2.513s.
- `/tmp/toolsy-task41/r20-b-lint.log`: /opt/homebrew/bin/golangci-lint run --allow-parallel-runners, 0 issues, private cache.
- `/tmp/toolsy-task41/r20-b-probe.log`: Go overlay outside repository, race count=5 for independent adversarial probes plus public/helper fixtures, PASS 1.611s.
- `/tmp/toolsy-task41/r20-b-public-final.log`: race count=3 latest public fixtures, PASS.
- `/tmp/toolsy-task41/r20-b-probe_test.go` and `/tmp/toolsy-task41/r20-b-overlay.json`: independent test source and mapping, repository unmodified.
- `git diff --check`: clean.

## Limits

Acceptance covers the explicit text-only subset, not universal OOXML validity, layout reconstruction or hostile parser isolation. Existing source/raw expansion/item/output budgets remain; XML decoder token work and intermediate allocation are cooperative and finite-input bounded rather than hard-preemptible. Percentages measure the five stated criteria and observed checks, not a claim that no possible bug exists. Other independent reviewer acceptance remains the parent's separate commit gate.
