# R15 / D29 verification

Baseline: 6f6365b. Public byte-identity probe fails on baseline success and success
with trailing blank lines: one final LF is lost. Guest failure already preserves
that LF. Current public probe passes all six cases with race count5 (1.477s).
Independent reviewers also exercise CR/LF/NUL, no output, multiple trailing blank
lines, guest exit0/1, fs.read guest failures, cancellation and output overflow.

Shared FinalizeOrInterrupt has no trim flag; every host, Docker, E2B, Wazero and
Starlark caller and shared-helper test uses the new signature. Exact typed stdout
bytes survive successful/failed guest completion. Presentation is consumer-owned.
Cancellation and infrastructure/output overflow retain their prior classification.

D29: Docker positive policy validated by New is the sole source of log timeout and
output bounds; unreachable fallback defaults and the outputLimit helper are removed.
The exported focused Client port contains only adapter-required capability/container
lifecycle/log methods. The SDK satisfies it from an external test package. Custom
ports must respect contexts, report truthful daemon capabilities and return owned
log readers whose Close unblocks reads. This does not prove a real daemon enforces
all policy fields. Starlark fs.read cap intentionally remains a guest evaluation
error (exit1, bounded stderr, nil Run error); stdout/stderr overflow is infrastructure
failure. These different failure phases are retained and documented.

Parent affected race: Starlark3.850s, Docker1.930s, E2B6.756s, Wazero69.532s,
host11.671s, shared finalizer1.718s; root race PASS. Pinned2.14 root/all5adapter
lint0 after fixing the external port fixture import group. The initial Docker lint
failure is historical and superseded by r15-final-docker-lint.log.

Both reviewers independently report five20/20 criteria and no unresolved detected
errors. Canonical baseline overlays are the evidence; initial commands that found
no tests are explicitly discarded in review records. No live Docker/E2B execution
or universal malicious-code isolation/memory guarantees inferred.

The previous row14 baseline description is clarified: inherited child pipes surface
WaitDelay failure; redirected children survive. Its actual probe logs are unchanged.
