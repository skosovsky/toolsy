# D14 — Independent acceptance B

Baseline HEAD: 77d3a10b9deeef34f125b5c893415d23cdec7fe5. Read-only review of current implementation, public claim contract, all code/test/documentation changes and untracked operation_snapshot_links_test.go. Reviewer A report was not read. No repository edits or commits performed.

## Criteria

1. **20/20%** — Retains bounded local reference adapter. README correctly describes complete-image load/decode/copy/validation on Inspect and mutation, full snapshot/write/fsync, CPU/memory growth with encoded image and attempts, lock serialization, inclusive finite cap and zero default8MiB. Decoded/transient memory exceeds file cap. No incremental index or maintenance subsystem was added.
2. **20/20%** — Reverse record→grant+consumed validation requires exact record key, full binding, equal approval expiry. Grantless records require zero approval expiry. Existing grant validation and consumed→record+grant checks remain. Missing grants/reservations, detached consumption, wrong keys/binding/expiry fail closed. Unconsumed and expired historical grants remain valid.
3. **20/20%** — AAA corrupt snapshots and persisted-file regression deny permission and do not rewrite bytes. Positive matrix uses actual Claim/Finish/Resolve for all five states with approved/unapproved histories and explicitly asserts historical approval expiry. Independent /tmp recovery probe exercises actual approved and unapproved Claim→Unknown→Resolve RetryAuthorized→Restore→second Claim→Completed→Restore, preserving both attempt fences and receipt. Durable unapproved completion/reopen succeeds.
4. **20/20%** — Host maintenance keeps stable lock inode, quiesces all handles/readers/writers, preserves unknown/completed results, consumed grants, expired approvals and fences, secure backup provenance, full-image migration and downstream idempotency. No automatic eviction/retention API, no JSON authenticity claim; coherent forgery remains trusted-storage responsibility. Capacity limitation and O(total-size) transactions are explicit.
5. **20/20%** — Public claim and README/migration contracts now agree: unapproved nonempty GrantID is invalid_claim before dispatch/mutation; recovery cannot change original approval mode. Initial detected regression is fixed and independently reproduced as refusal with readable unchanged journal. Targeted/full affected race and pinned lint pass (see evidence below). This score is B's independent assessment; peer acceptance remains the leading reviewer’s coordination duty.

**Total: 100%. Verdict: ACCEPTED after fixes. No unresolved detected errors.**

## Finding resolved during review

Initial candidate accepted Claim(RequiresApproval=false, GrantID="unused") and persisted Dispatch=true, but the new Restore rejected its own snapshot with corrupt_snapshot. Filejournal then could no longer Inspect the image. This was a real API-generated snapshot, also consistent with existing reconcile-test inputs. Independent executable reproducer and original output: /tmp/toolsy-task41/d14-b-api-probe.go and d14-b-api-probe.log.

Final candidate explicitly rejects this inconsistent host input before permission/mutation, updates public/migration contracts and callers, and prevents recovery from erasing/changing consumed approval mode. Independent rerun d14-b-api-probe-fixed.log: memory Dispatch=false invalid_claim with successful empty-image restore; filejournal Dispatch=false invalid_claim with successful Inspect. Regression tests cover byte preservation and legitimate completion/reopen. Stronger positive fixture now uses real transitions and expired grants, fixing the earlier coverage gap.

## Independent validation

All commands use GOCACHE=/tmp/toolsy-review-gocache. Lint uses GOLANGCI_LINT_CACHE=/tmp/toolsy-task41/d14-b-lint and /opt/homebrew/bin/golangci-lint run --allow-parallel-runners; pinned binary reports2.14.0 built with go1.27.1.

- Uncached go test -count=1 ./... PASS: d14-b-tests.log.
- Earlier full go test -race -count=1 ./... PASS: d14-b-fullrace.log.
- Final go test -race -count=3 -run 'TestRestoreOperationSnapshot|TestOperationClaimRejects|TestOperationStore|TestOperationReconcile|TestJournal' ./ ./adapters/execution/filejournal PASS: d14-b-final-targetrace.log (root1.923s, journal7.175s).
- Final root pinned lint PASS, 0 issues: d14-b-final-lint.log. Explicit filejournal cwd lint also PASS,0 issues: d14-b-journal-lint.log (journal is part of root module).
- Final uncached full go test -race -count=1 ./... PASS (exit0): d14-b-final-fullrace.log, including internal/release and internal/toolsygen integration tests.
- git diff --check PASS.
- Independent API consistency and recovery probes PASS: d14-b-api-probe-fixed.log, d14-b-recovery-probe.log.
- Baseline behavioral log inspected: five new reverse-link subcases fail against old operation_store.go, not a build-only artifact.

## Scope and limitations

Acceptance addresses the five D14 criteria, not universal defect absence. Tests run on Darwin/local filesystem; no distributed/network filesystem or production backend claim. JSON integrity/authentication and external outcome truth are intentionally unproven. No benchmarks or large-image exhaustion experiments: complexity verified against actual whole-image load/transaction code and existing quota regression. No automatic lifecycle service or retention policy was introduced.
