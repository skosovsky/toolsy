# R14 / D28 verification

Row baseline: 1180b3a. The public descendant completion probe fails on all four
baseline combinations (exit0/7; inherited/redirected pipes): children remain alive.
The unchanged probe passes on the current owned-group implementation.

Unix runs the configured guest directly through Go exec, preserving start failures,
ordinary exit126/127 and signal ExitCode=-1. A separate /bin/sh anchor holds the
owned process group until cleanup completes. Cancellation and output collectors
share the initial stop outcome. All collectors and the cancellation watcher join
before anchor reap. Subsequent bounded sweeps run after guest Wait, while anchor
still pins PGID, covering a fork concurrent with the initial group signal.
No destructive process-group signal follows anchor reap. Failed group signals
attempt direct owned-process termination and preserve cleanup diagnostics.

Darwin can transiently return EPERM for dying group members. This is not cleanup
confirmation: read-only post-anchor observation must reach ESRCH within five
seconds, otherwise the workspace is retained with CleanupError.ResourceID. Collection
has its own five-second budget and closes owned pipes on expiry. Kernel/filesystem
syscalls cannot promise a hard timeout for uninterruptible OS states. Deliberate
process-group escape, malicious signal/credential interference, arbitrary blocking
writers and hostile filesystem mutation are outside this trusted host adapter.
Non-Unix retains documented best-effort direct process termination.

exectool error paths retain exactly the backend-returned RunResult inside
RunOutcomeError; errors.As exposes it without a success chunk. CleanupError has
an optional opaque ResourceID. Validation mapping retains unsupported-language
causes; timeout/cancellation/output-limit classification remains intact. Returned
results on interrupted/setup failures can be zero/incomplete and do not authorize
blind retry. Backend cleanup remains backend-owned, not a core controller.

Parent checks: full host race PASS11.622s, exectool race PASS1.912s, collection/fork+
overflow/completion race count10 PASS4.235s; pinned host and root lint0. Independent
reviewers additionally execute public start/exit/outcome probes, rejecting-writer
collection/fork probes, full affected race/lint, and Linux/Windows amd64 compilation.
No live cloud/container/production tests or Linux/Windows runtime guarantees inferred.

Initial reviews found shell-launch start-error masking, dropped unsupported-language
cause, lint errors, repeated-signal EPERM and a fork-vs-groupkill surviving child.
Those were corrected with direct guest launch, cause-preserving mapping, synchronized
initial stop, bounded pinned-group sweeps and permanent AAA regressions. Historical
failed logs remain evidence and are superseded by final accepted verdicts.
