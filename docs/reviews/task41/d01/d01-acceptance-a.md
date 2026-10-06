# D01 independent acceptance A

Verdict: accepted. Completeness: 100% (five criteria, 20% each). No detected unresolved errors or regressions. Review baseline: HEAD e9d80b4 and current working diff, including untracked docs/history-compaction-migration.md. I did not implement changes, delegate work, or read acceptance B.

1. **20/20 — contexty capability check.** Verified local HEAD 8416b7b9883a09a26e3fe740d02dd8f78bff0b7f; inspected pipeline.go, budget_execution.go, budget_retention.go, rolling_summary.go, strategies.go and rolling-summary tests. Required ID selection rejects unresolved/anonymous required content; complete and pending rounds expand protection; rolling recent tail is not silently trimmed; summary errors preserve causes and do not trigger mechanical fallback. Independent targeted contexty race regressions passed twice. Migration explicitly documents generic-to-semantic projection, single-summary result, estimator semantics, prefix-vs-role retention and fallback differences.
2. **20/20 — duplication removed.** Generic history package, example, compaction OTel helper and specific tests are deleted. Remaining content-policy test paths exercise the retained result/error/panic paths. No replacement generic policy or optional adapter was introduced in toolsy.
3. **20/20 — transcript/BYOT boundary preserved.** No changes to historycodec, ResultCodec or core execution production contracts. Independent root race includes transcript/historycodec, typed persistence and generator tests; all passed. OTel race passed three times. Root and OTel independent private-cache lint each returned zero issues.
4. **20/20 — migration/host recipe.** Removed public API names and helper/example paths are named. The documented host function matches the compiled extracted recipe; GOWORK=off local-contexty host race tests passed three times, exercising protected prefix/recent-tail composition and provider-cause propagation without fallback. Host owns projection/identity/provenance, effective budget, estimator, summarizer, retention, retries and telemetry. Root/extension Go modules and Go source imports contain no contexty dependency.
5. **20/20 — consumer/docs consistency.** Searched tracked current source/docs for semantic_truncation, SemanticTruncation, toolsy/history and former APIs. Remaining references describe removal/migration, supported historycodec or historical review requirements; no live deleted-API recommendation or consumer import remains. git diff --check passed. This independent verdict is 100% with no detected defects; the separate B verdict and resulting two-reviewer commit gate remain the coordinator's responsibility and were not consulted.

## Independent commands/evidence

- `GOCACHE=/tmp/toolsy-review-gocache go test -race ./...` — PASS, internal/toolsygen 42.179s; `/tmp/toolsy-task41/d01-a-root-race.log`.
- `GOCACHE=/tmp/toolsy-review-gocache go -C ext/toolsyotel test -race -count=3 ./...` — PASS 1.449s; d01-a-otel-race.log.
- `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go -C ../contexty test -race -count=2 -run 'Test(Rolling_|DropHeadStrategy_|Budget_Pending|Acceptance_BudgetRetention|Acceptance_BudgetRequiredContent|BudgetPipeline_)' .` — PASS 4.439s; d01-a-contexty-race.log.
- `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go -C /tmp/toolsy-task41/d01-host test -race -count=3 ./...` — PASS 1.255s; d01-a-host-race.log.
- `/opt/homebrew/bin/golangci-lint run --allow-parallel-runners ./...`, root and ext/toolsyotel cwd with separate d01-a caches — both 0 issues; d01-a-root-lint.log and d01-a-otel-lint.log.
- `git diff --check`, dependency/import searches, current-doc references and source diff inspection — clean within D01 scope.

## Limits

This verifies the checked local contexty revision, not published-version availability, arbitrary host projections, provider token accuracy or hard interruption of blocking callbacks. Deletion is an intentional clear API break. Root race may reuse valid Go test cache entries for unchanged packages; the generator and OTel/contexty/host checks executed successfully, with explicit repetition for the latter three. Full all-module final acceptance remains a later task. No uncovered D01 criteria.
