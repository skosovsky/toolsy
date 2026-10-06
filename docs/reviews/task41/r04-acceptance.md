# R04 / D12 independent acceptance

Scope: five equally weighted criteria in the execution ledger, final diff
against `322ec4a`. Typed result/effect/postcondition callback failures are outer
nonretryable INTERNAL with no fixable arguments and an inspectable
ResultContractError.Kind phase plus exact Cause. Pre-handler correction remains.

Known result/outcome failures take precedence over diagnostic causes in handler,
registry, control, formatter, batch and iterator routing. Explicit error chunks
carry reconciliation guidance; WithErrorFormatter leaves these as hard errors.
Shared web/RAG/SQL host result validators retain result_validator phase/cause.
OperationOutcomeError says after claim, with DispatchInvoked as local evidence;
unfinished claims stay unknown without unfenced rollback. Persisted completed
results survive delivery failure.

## Reviewer A — r04_acceptance_a

**100%, accepted**, five criteria 20/20, zero unresolved detected defects.
Independently reviewed the entire final diff, new tests and migration, repeated
uncached race for root/web/RAG/SQL, lint for all four (zero issues), whitespace
checks and the runnable example. Own overlays cover arbitrary callback codes,
exact phase/cause, approval expiry after claim, public wire formatting, host
validators and RegistryView/Session iterators. Preliminary formatter timeout and
downstream validator findings were corrected and independently rechecked.
Did not mutate repository files or read the other verdict.

## Reviewer B — r04_acceptance_b

**100%, accepted**, five criteria 20/20, zero unresolved detected defects.
Independently reviewed the final diff and repeated uncached race and lint for all
four modules, whitespace checks and the example. Own overlays cover joined
callback errors, exact Cause identity, approval expiry before invoke, wire
classification, nested handler and diagnostic control causes. Preliminary
wire/nested timeout downgrade and control-routing findings were corrected and
independently rechecked. Did not mutate repository files or read the other verdict.

## Baseline and parent checks

The original archive `58085005` fails all six initial plain/ToolError postvalidator
regressions behaviorally: a host argument-repair flow produces effects=2 and
results=0. The retained initial fixture compiles against the original API; later
phase, journal, formatter, timeout and routing assertions were added to the
current API and are not part of that baseline proof. An initial fixture setup
used a nonexistent ToolError field; it was corrected before the retained baseline
run. Both reviewers independently confirmed the behavioral failure and original
production source identity.

Parent final `go test -race -count=1 ./... ./toolkits/web/... ./toolkits/rag/...
./toolkits/sqltool/...` passes. golangci-lint 2.14.0 on those paths reports zero
issues; whitespace checks pass. The example prints
`reconcile: phase=result_validator effects=1`. Logs retained here expand tabs and
remove trailing whitespace without changing messages.

One local BenchmarkExecute run observes 37,467 ns/op, 14,630 B/op and 236 allocs/op;
allocation count is the same as the R03 measurement. These single observations
are not a statistically controlled performance comparison.

## Limits

These are local host callbacks, journal fixtures, routing probes and source
review; real external effects/reconciliation are host-owned. DispatchInvoked=false
is not a fenced proof of externally not-started. Formatter callbacks, wire byte
caps, other task41 rows and the final all-module gate remain separate scope.
Percentages describe completion of stated criteria, not universal bug freedom.
