# R18 / D31 independent acceptance B

Reviewed current diff against a8bf9fb, original task41 R18/D31, progress row18, README/API/migration and both untracked test files. No implementation files edited; no other reviewer verdict consulted.

| Criterion | Score | Evidence |
|---|---:|---|
| Context-aware wait, retained cancellation/deadline cause, no waiter I/O | 20/20 | All three public tools with held Load return on deadline/manual cancellation before owner release, one Load and zero Saves. Acquire checks before select and after successful admission; loadFacts checks before provider I/O. |
| Serialized complete updates, release on errors/cancel, bounded gate | 20/20 | One capacity-one channel owns full Load/modify/Save; defer release covers each admitted handler. Existing concurrent updates pass under race. Independent pre-canceled-handler, failed Save then successful Read, and repeated canceled-admission probes pass. No waiter goroutines or per-session map. |
| Structured facts, escaping, schema and bounds | 20/20 | Exact facts map replaces newline formatting; empty nonnil map encodes {}. Generated public output schema has map additionalProperties string. New escaped key/value fixture includes newline, equals, quote, XML, CR, NUL and Unicode with inclusive full JSON boundary. Stored object/state key and pin/unpin args/status unchanged. |
| Honest ownership and migration | 20/20 | API/README/migration identify bounded session scratchpad and serialized instance across sessions, sole writer, no fairness/CAS/distributed/reentry, host callback deadlines/allocation, no rollback. Legacy formatting code removed. |
| Regression evidence and independent verification | 20/20 | Inspected baseline log: all six tool/cancel-kind cases fail on waiting until provider release. Independently ran memory full race count3, lint, focused adversarial/public concurrency count10 and generated-schema count10. No unresolved detected errors. This score refers to this review's gate; sibling acceptance is separately required by coordinator. |

Total: **100%**. Verdict: **ACCEPT**. Detected actionable errors: **none**.

Independent checks:
- `go -C toolkits/memory test -race -count=3 ./...`: PASS 2.554s (`r18-b-race.log`).
- Pinned `/opt/homebrew/bin/golangci-lint run --allow-parallel-runners`, module workdir and private cache: 0 issues (`r18-b-lint.log`).
- Read-only overlay: pre-canceled handlers zero callbacks, failed Save releases admission and preserves original cause, 10,000 canceled admissions, existing public held-load cancellation, structured wire boundary and concurrent updates, race count10: PASS 3.468s (`r18-b-adversarial.log`).
- Public generated schema and independent overlay tests, race count10: PASS 2.072s (`r18-b-schema.log`).

Limits: cancellation cannot forcibly interrupt a noncooperating in-flight host callback or undo an already-started Save. Gate is instance-wide, intentionally no fairness or distributed atomicity. Context checks do not eliminate the inevitable race between last check and host callback invocation. No remote provider or multiple-process coordination proof claimed. Percentages measure these row criteria, not universal bug freedom.
