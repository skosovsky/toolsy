# R10 / D06 acceptance B

Independent reviewer; no production changes; no other reviewer's verdict consulted.

Accepted: 100% (five criteria, 20/20 each). No unresolved detected bugs.

1. Slice ownership: 20/20. Capture/materialization/NewSession all clone both slices. Execution policy, stored options and reused sessions have independent snapshots. Own external public probe mutates both original slices concurrently with 12 session-construction/admission workers; read remains allowed and catalog-required write never expands whitelist.
2. Catalog semantics: 20/20. CatalogRequiredTools validates visible binding names before codec freeze, independently of AllowedTools/ForcedTool. Checked nil/missing/view tests, forced/allowed relation, registry bypass boundary and Rebind compatibility. Missing catalog fails before freezing builder.
3. Call accounting: 20/20. New APIs and MAX_CALLS_EXCEEDED sentinel/wire/classification replace primary old names. Negative limits fail before freeze. Shared atomic gate counts admitted errors and budget denials, excluding nil registry/policy rejection. Own mixed RunCall/Execute contention: 13 handlers, 137 denials, 150 attempts; nested same-session denial consumes second attempt; internal wrapper retries three handlers under one admission. Restore uses current host ForcedTool/limit and fresh counter.
4. Regression evidence: 20/20. Inspected AAA tests for slice capture/materialization/reuse, concurrent source writes, catalog lifecycle, admission ordering and wire restoration. Baseline log fails both caller mutation stages. Independent public probes exercise race, contention, nested calls, retries and checkpoint current authority.
5. Verification/docs: 20/20. Own targeted project race count=7 PASS (3.391s); root golangci-lint PASS (0 issues); external public probes race count=5 PASS (2.923s); git diff --check PASS. Reviewed final README max-calls wording and task28 migration correction, API/wire-break migration and checkpoint authority/budget boundaries. No primary obsolete APIs remain. Other reviewer's independence is preserved; parent combines separate acceptance results.

Evidence: targeted-race.log, lint.log, public-probes.log, probe_test.go, go.mod/go.sum in this directory. External isolated probe used GOSUMDB=off because sandbox denies global sumdb latest writes; normal project race and lint did not disable verification. Capturing a slice concurrently with its initial caller write is expressly unsupported; mutable registry manifests remain host-owned setup lifetime. No publication or commit performed.
