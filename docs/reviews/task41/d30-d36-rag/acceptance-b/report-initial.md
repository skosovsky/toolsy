# Independent acceptance B — task41 row33 D30/D36 RAG

Reviewed baseline fe241b0f4f1ecacacd0842107410ab56f30601ea, task41 D30/D36 requirements, row33 five criteria, complete current RAG diff/untracked files, migration29/30/41, runnable host recipe and its tests, implementation evidence. No peer verdict read; no repository changes, live backend, or new dependency.

Initial verdict: **95%, not accepted**, pending clarification of newly stated custom cancellation-cause contract.

| Criterion | Score |
|---|---:|
| Thin single-call adapter, deleted routing API, host-owned policy | 20/20 |
| Executable host fallback/endpoint/cancellation/causal merge/identity recipe | 20/20 |
| Single final wire representation, provider/filter/count/DTO/provenance/ownership | 20/20 |
| AAA/runnable probes, behavioral baseline, identical allocation fixture, race/lint | 20/20 |
| Accurate API/README/migration and honest cancellation limits | 15/20 |

Finding: toolkits/rag/README.md:92 states cancellation/deadline including custom parent cause before/after callbacks. An already-expired WithDeadlineCause context passed to public Execute is rejected by unchanged core prepared_execution.go:64–65 before RAG handler dispatch; normalizeExecutionInterrupt receives ctx.Err only, so returned TIMEOUT retains context.DeadlineExceeded but loses context.Cause. Independent TestBTerminalDeadlineStages/before fails on custom-cause assertion all3runs; no callback is dispatched. RAG's own provider/filter/formatter/validator boundaries preserve custom cause. This is inherited core behavior, not a production regression; document exact handler/pre-dispatch boundary instead of silently expanding this row to core cancellation work. Initial failing source/log retained as probe-initial.go.txt and probes-initial-failure.log.

No other detected defect. Final report follows after contract clarification.
