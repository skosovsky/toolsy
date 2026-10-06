# D14 acceptance A — revised final candidate

Baseline HEAD 77d3a10. Independent read-only inspection of complete tracked/untracked production, tests, contracts, maintenance/migration and proof diff; no repository edits or commit. No peer acceptance report read.

Verdict: **accepted, 100%** for A's independent criteria. No unresolved detected defect. Parent must separately obtain B's signoff.

| Criterion | Score | Evidence |
|---|---:|---|
| 1. Bounded reference adapter and complexity | 20/20% | README matches full load/restore/copy/serialize cost, serialized persistent advisory lock, inclusive 8MiB default/cap and memory beyond file cap. No indexing/pruning or indefinite-growth promise. |
| 2. Bidirectional structural consistency | 20/20% | Every grant-bearing record requires exact consumed key and grant binding/expiry; every consumed reservation must identify the matching record/grant; grants independently validated; grantless expiry rejected. Unconsumed/expired historical grants retained. |
| 3. AAA regressions and legitimate snapshots | 20/20% | Corrupt reverse-link restoration and durable Claim/Inspect deny permission and preserve bytes. Real Claim/Finish/Resolve matrix covers five approved/unapproved states with actually expired historical grants. Independent probes cover coherent same-mode recovery and both forbidden mode switches. |
| 4. Host maintenance and boundaries | 20/20% | Preserve unknown outcomes, consumed grants, fences, results and replay identities; quiesce workers and preserve lock inode/backup provenance. External proof needed for retention; no automatic eviction, authenticity or distributed-effect promise. |
| 5. Contract/migration and checks | 20/20% | New invalid_claim rule and recovery binding_mismatch explicitly documented; executable refusal before mutation and legitimate durable finish/reopen match contracts. Own final full/targeted race and pinned lint pass; no remaining finding. Independent B acceptance is parent's separate gate. |

## Initial finding closed

Previously unapproved Claim with incidental GrantID could dispatch and persist an unrestorable image. The revised implementation rejects it before dispatch/mutation as invalid_claim. Original standalone reproduction now returns dispatch=false, invalid_claim, and Restore succeeds on the unchanged empty snapshot. The intended compatibility break is explicit in OperationClaim, README and migration. Existing callers clear irrelevant GrantID.

Recovery cannot downgrade or upgrade approval mode for the same retry-authorized record; binding_mismatch leaves grants/reservations unchanged. Same-mode approved recovery retains the original grant and consumed link, and same-mode unapproved recovery remains grantless and restorable. Existing normative/state/reconciliation tests also pass.

## Own final checks

All use GOCACHE=/tmp/toolsy-review-gocache.

- `go test -race -count=1 ./...`: PASS, uncached full root module and filejournal affected package. Log `/tmp/toolsy-task41/d14-a-final-race.log`.
- `go test -race -count=3 -run 'TestRestoreOperationSnapshot|TestOperationClaimRejectsInconsistentApprovalMode|TestJournal' ./ ./adapters/execution/filejournal`: PASS, root1.839s / journal6.865s. Log `/tmp/toolsy-task41/d14-a-final-targetedrace.log`.
- Pinned `/opt/homebrew/bin/golangci-lint` 2.14.0 built with Go1.27.1, `run --allow-parallel-runners ./...`, own GOLANGCI_LINT_CACHE=/tmp/toolsy-task41/d14-a-lint: PASS, 0 issues. Log `/tmp/toolsy-task41/d14-a-final-lint.log`.
- `/tmp/toolsy-task41/d14-a-final-probe.go`: PASS independent public memory/durable API assertions. Invalid initial unapproved claim leaves journal image absent; valid unapproved Claim/Finish/Open/Inspect preserves completed result; both recovery mode switches fail without mutation; same-mode recoveries restore.
- Original baseline five behavioral failures and retained proof fixture/log independently inspected. Public Restore benchmark fixture agrees with full-image allocation cost; timings are explicitly local observations, not performance guarantees.

## Limits

Scope is D14 and affected root/filejournal packages on Darwin. No live distributed backend, cryptographic authenticity, downstream exactly-once, or arbitrary host filesystem integrity was inferred. Those remain explicit host boundaries. Acceptance measures these five criteria, not universal bug freedom.
