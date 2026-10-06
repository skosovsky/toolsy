# R14 / D28 — independent acceptance B

Base: 1180b3a. Reviewed final direct-runtime / owned-anchor implementation including shared sticky ownedProcessGroup.stop. This verdict supersedes review-initial.md; its two demonstrated functional regressions and two lint issues are fixed.

## Completeness: 100% (20/20 for each criterion)

1. Owned Unix group lifetime: anchor remains unreaped through guest Wait, all collectors and cancellation watcher. Sticky mutex-protected stop executes one group SIGKILL, preserves any failed group diagnostic and direct-process fallback, and prevents reaped/reused-group destructive signaling. Confirmation uses signal0 only. Workspace removal follows confirmed cleanup; failed cleanup retains locator/workspace.
2. Normal exit/output preserved; direct Go exec preserves launch failures, guest exits126/127 and signal exit -1. Owned output collection budget closes pipe readers, then joins both collectors. Cancellation takes precedence; cleanup diagnostics do not reclassify its deadline. OS uninterruptible states and escaped groups are explicitly outside hard-bound claims.
3. RunOutcomeError retains precisely sandbox-returned outcome for all inspected Run error mappings including unsupported language. Primary timeout/cancel/readlimit categories remain inspectable. No success chunk on error; CleanupError exposes ResourceID and cleanup classification without leaking cleanup-cause deadline into execution classification.
4. AAA regressions cover child pipes inherited/redirected, success/nonzero/cancel, and direct-process fallback with injected group-signal failure. Tests clean owned fixture processes. Public baseline completion-child leak probe fails both redirected/inherited cases; current affected suite passes. B public missing-executable probe passes baseline, initially failed supervisor implementation, now passes alongside real exits126/127/signal. B public unsupported-outcome probe initially failed, now passes.
5. Final affected host and core race suites, pinned host/core lint and Linux/Windows amd64 compile checks pass. Docs/migration accurately scope /bin/sh anchor, direct-runtime semantics, cleanup budgets, conservative zombie/reuse failure, reconciliation locator and non-Unix best effort.

## Independent evidence

- host-final-race.log: PASS11.826s, count1, latest sticky owner source.
- core-final-race.log: exectool PASS1.722s; sandboxfs PASS1.922s, count1.
- start-final.log: missing executable + genuine exit126/127/signal public probes, count3 race PASS2.194s.
- outcome-final.log: public changing-support sandbox Run error retains outcome, count3 race PASS1.586s.
- host-final-lint.log and core-final-lint.log: 0 issues. Pinned golangci-lint, host private cache and allow-parallel-runners to avoid shared lock contention.
- linux-final-compile.log/windows-final-compile.log: exit0; crosscompile, not claims of executing tests on those platforms.
- start-baseline.log and /tmp/toolsy-task41/r14-baseline-child.log: baseline comparisons.
- git diff --check: PASS.

No detected unresolved implementation errors within supported contract. Accepted limitations: host is non-isolated; escaped or maliciously interfering processes are not contained; generic OS process/fs syscalls cannot promise a hard deadline; group reuse/zombies can conservatively retain workspace; non-Unix tree termination is best effort. No repository changes, commits or implementation delegation performed by reviewer.

## Final delta re-acceptance — EPERM observation and overflow child regression

The final implementation now treats Darwin signal0 EPERM as unconfirmed group presence and continues bounded read-only observation until ESRCH or five-second deadline. It never treats EPERM as removal and never sends another destructive signal after anchor.Wait. This is consistent with conservative zombie/PID/group-reuse limits. The added permanent AAA output-overflow child test verifies read-limit classification, absence of spurious cleanup error, child termination and workspace removal, with fixture cleanup on failure.

Independently rerun against this exact latest source:
- host-final2-race.log: complete affected host suite count1 race PASS11.672s.
- overflow-final2-race.log: permanent output overflow plus live-child regression count10 race PASS4.395s.
- host-final2-lint.log: pinned lint 0 issues.

Core source did not change after its final PASS checks. Final completeness remains **100%, all five criteria20/20, accepted with no detected unresolved errors**. This addendum supersedes timing/source coverage of the earlier host run.

## Superseding final re-acceptance — fork concurrent with first signal

This is the final acceptance against the implementation adding ownedProcessGroup.sweep and collection_unix_test.go. It supersedes prior 100% verdicts for source coverage. A single sticky group signal could miss a child fork concurrent with signaling; the latest source sweeps only after guest Wait and while the unreaped anchor still pins PGID, periodically during bounded collection and again before joining watcher/reaping anchor. EPERM after a known successful stop is unconfirmed presence rather than success; post-reap signal0 must still yield ESRCH. No destructive signal occurs after anchor.Wait. Group-level signal failure remains cached and diagnostic; direct owned-process fallback is preserved.

The permanent collection writer error adversarial fixture starts group descendants while the guest continues shell execution, uses a rejecting output writer, verifies prompt termination and group cleanup, and explicitly cleans both known child PID files on failure. Its error naming issue was independently found by B pinned lint and corrected to errCollectionWriter.

Independent latest-source verification:
- host-final3-race.log: full host race suite count1 PASS12.910s.
- collection-final3-race.log: collection writer failure/fork plus output-overflow child fixtures count20 race PASS5.421s.
- start-final3.log: public missing executable and genuine guest126/127/signal probes count3 race PASS1.851s.
- host-final3-lint.log: pinned lint0issues after naming fix.
- linux-final3-compile.log/windows-final3-compile.log: compile exit0.
- Core error/outcome source unchanged since independently passing final race/lint and public outcome probes.
- Final git diff --check PASS.

**Final verdict: accepted, 100% completeness, five criteria20/20, no detected unresolved errors within documented supported contract.** All stated platform/OS/escape limitations remain accepted. Reviewer B made no repository edits.
