# R21 / D26 independent acceptance B

Verdict: **accepted — 100% (5 × 20%)**. No unresolved detected errors or regressions.

Reviewed independently, read-only implementation review against HEAD `0700cc5`.
Read original R21/D26 requirements, row21 criteria, all tracked changes plus
untracked `commit.go`, `commit_recovery_test.go` and generated_stream files.
No implementation participation, no other reviewer's verdict used.

1. **20/20 — unified finalize recovery.** Every reservation, old-to-backup,
   temp-to-target failure and observed cancellation uses failedFinalization.
   Per-file installed/backupMoved state covers both earlier installed outputs and
   the current partially moved old target. Final context check catches cancellation
   following the last successful install. Rollback runs in reverse order.
2. **20/20 — causes and recovery copies.** errors.Join preserves primary,
   restore/removal and owned-artifact cleanup errors. FileRecoveryError identifies
   target, artifact/backup and action. cleanupStagedTemps explicitly skips moved
   old-content backups; failed restore or removal preserves the recovery copy.
   Staging failures clean preceding temps. Post-install disposal is distinguished
   by CommitComplete=true and Generate returns complete Files alongside the error.
3. **20/20 — adversarial filesystem evidence.** Permanent tests cover reserve,
   backup, install, cancellation after first/last/backup, failed restores/removal,
   new-target rollback, staging/cleanup failures and committed disposal diagnostics.
   Reviewed shared baseline: reserve/backup leave first file new; failed current
   restore loses its backup. Current shared probe independently passed race count5.
   Additional reviewer-owned overlay independently verifies complete Generate.Files
   after disposal failure and aggregation/preservation of two simultaneous failed
   restores; repeated all selected recovery regressions count5.
4. **20/20 — explicit streaming lifecycle.** Generated factory returns synchronous
   proxy, preserving progress/terminal/error, invalid input before dispatch, caller
   interruption and consumer-error propagation. Generated-module compilation/race
   fixture covers empty/single/multiple parts, buffered progress before handler
   failure, invalid input, caller deadline, consumer stop, explicit async scheduling
   and callback success/error/timeout/validation. Example configures timeout, chunk
   cap and callback, registers async tool and shuts registry down. Independently ran
   it successfully: accepted scheduling then two progress chunks and final result.
5. **20/20 — documentation and checks.** README, generator-contract and migration
   correctly explain ordinary streams, explicit async detached cancellation,
   accepted-versus-completed distinction, cooperative execution, exclusive writers,
   non-crash-atomic filesystem rollback and committed cleanup diagnostics. Root
   race and pinned private-cache lint pass; diff whitespace check passes. This is
   this reviewer's acceptance; orchestration must independently gate on reviewer A
   too before committing.

Independent commands/evidence:

- `GOCACHE=/tmp/toolsy-review-gocache go test -race -count=1 ./...`:
  PASS, `r21-b-root-race.log`, generator52.479s/release54.623s; includes generated
  temporary-module race tests and compilation of generated_stream example.
- Pinned golangci-lint with private cache and `--allow-parallel-runners ./...`:
  exit0, **0 issues**, `r21-b-root-lint.log`.
- Reviewer-owned `r21-b-overlay.json`, selected recovery + TestR21B count5 race:
  PASS1.870s, `r21-b-adversarial.log`; source `r21-b-extra_test.go` outside checkout.
- Existing shared current overlay, TestR21FinalizePublicRecoveryProbe count5 race:
  PASS, `r21-b-shared-probe.log`.
- `go run ./examples/generated_stream`: exit0, `r21-b-example.log`.
- `git diff --check`: exit0.

Limits: cooperative synchronous filesystem operations; no process-crash atomicity,
concurrent-writer/hostile external mutation protection or hard handler preemption.
Fault injection models failing operations and normal local rename semantics. No
production publication/push or nonlocal filesystem runtime verification. Percent
measures specified criteria, not a guarantee of all possible defects being absent.
