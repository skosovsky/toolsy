# R10 / D06 acceptance

Scope: RunPolicy ownership, catalog requirements and call admission terminology.
Previous commit: `9a9062a`. No production push/publication.

## Contract

WithRunPolicy snapshots AllowedTools and CatalogRequiredTools at capture and each
materialization; NewSession also creates independent execution/options snapshots.
Caller mutation after capture/construction cannot change admissions or validation;
reused options produce independent sessions. Concurrent mutation during the initial
capture itself is not supported. There is no dynamic policy update API.

Clear break: RequiredTools becomes CatalogRequiredTools. It requires names in the
visible catalog during construction, before codec freeze. It neither requires
calling them nor restricts calls. AllowedTools/ForcedTool select calls. Catalog
requirements may be outside AllowedTools or ForcedTool. A nonempty AllowedTools
still must include ForcedTool. Registry direct execution remains separate.

Clear break: WithMaxCalls, Track().MaxCalls(), Track().CallAttempts(),
ErrMaxCallsExceeded, CodeMaxCallsExceeded and NewMaxCallsExceededError replace the
step/execution naming. The wire code becomes MAX_CALLS_EXCEEDED. Negative limits
fail construction, zero remains unlimited. Old wire code no longer maps to the
budget sentinel/classification; host wire switches need migration.

Accounting order is preserved: nil registry and RunPolicy rejection consume
nothing; other outer Session calls atomically increment attempts and fail above
the limit. Budget-rejected attempts count. Environment/registry/argument errors,
cancellation, results/errors and replay occur after admission. Internal wrappers
can repeat dispatch under one outer admission; nested Session calls each consume
one admission. This is not a count of successful handlers/effects or agent
iterations. Checkpoint restores current authority/limits and fresh counters;
durable budgets remain host-owned.

## Evidence

- Parent `GOCACHE=/tmp/toolsy-review-gocache make test`: PASS, all 24 modules,
  go test -v -race ./.... Full log retained.
- Parent targeted ownership/catalog/admission suite -race -count=5: PASS, 2.564s.
- Root pinned golangci-lint v2.14.0: 0 issues; diff whitespace check passes.
- Previous-commit public policy probe: FAIL as expected after both option capture
  and session construction. Caller changes read to write; write succeeds and read
  fails. Identical probe with the fix: PASS, write rejected/read accepted.
- A independent root race: PASS, 4.001s; targeted+overlay count8: PASS, 4.445s;
  additional adversarial count10: PASS, 2.789s. Original/materialization slice
  writes race with eight callers constructing sessions, executing and rebinding;
  forced-call/catalog independence, three inner repeats per outer admission,
  RunCall budget/wire classification and old-code sentinel exclusion pass.
- B independent targeted race count7: PASS, 3.391s; external public probes count5:
  PASS, 3.384s. Mixed 150 RunCall/Execute attempts admit exactly 13 handlers and
  reject 137 on budget, counting all 150 attempts. Current-authority checkpoint
  restore, nested budget denial and nil-registry admission pass.

Both independently reviewed the final diff, corrected documentation and migration:
`r10_acceptance_a`: **100%, accepted**; `r10_acceptance_b`: **100%, accepted**.
Each criterion received 20/20. No unresolved detected defects. Neither implemented
production code nor read the other verdict. Stale README execution/step wording was
fixed; the normative task28 migration table also uses call terminology. Historical
review evidence remains historical.

## Performance and limits

A 32-name AllowedTools constructor benchmark with a reused option ran baseline
then current, three 200ms samples. Baseline: 2408B, 8 allocations; current: 4456B,
12 allocations. The four independent constructor copies cost an additional 2048B
and four allocations per construction. Option capture copies once outside this
measurement. Short timing samples under shared load are not a statistical latency
guarantee; cloning occurs during configuration, not each admission.

Evidence and probe sources: [r10/](r10/). B's standalone probe required GOSUMDB=off
because sandbox restrictions prevented writing global sumdb latest; normal project
race/lint ran with the usual environment. Initial capture ownership, no dynamic
updates, and host durable authority/budget limits are documented. This acceptance
covers the declared contracts, not universal absence of defects.
