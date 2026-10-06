# R21 / D26 verification

Unified finalization failure recovery handles reservation, backup, installation
and cancellation failures. Reverse rollback preserves moved old-content backups
when restore or installed-target removal fails. Primary and recovery/cleanup
causes are joined and expose target/recovery paths. Earlier staging artifacts are
cleaned on failure. Backup disposal follows completed installation; cleanup errors
report CommitComplete=true and Generate returns its complete Result.Files.

D26: generated streams remain synchronous. Hosts explicitly configure async
background timeout, collection cap and completion callback. Generated consumer
fixtures run under race and cover progress, terminal/error, input validation,
caller cancellation and explicit async completion. Runnable example is
examples/generated_stream.

Both independent reviewers accepted 100% (five 20% criteria), with no unresolved
detected defects. Parent root race and pinned lint pass; both reviewers repeated
root race/lint independently and exercised additional recovery fault probes.
Baseline 0700cc5 fails all three retained original-API recovery probes; current
race count5 passes. Additional probes cover simultaneous failed restores and
truthful Generate.Files after committed disposal failure.

Limits: normal local rename semantics, exclusive host writer and cooperative
filesystem/handler operations. No crash-atomic multi-file transaction, nonlocal
filesystem verification, hostile concurrent mutation or hard preemption guarantee.
Raw logs and reviewer-owned probe sources are retained alongside both reports.
