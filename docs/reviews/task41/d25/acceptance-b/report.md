# D25 independent acceptance B

Verdict: **ACCEPTED — 100%** for the corrected current D25 implementation. No detected unresolved errors.

Baseline HEAD: `7331736ac925527fe5efe2711eb59c292a172ffc`. Reviewed uncommitted diff on 2026-10-06 in `/Users/skosovsky/Library/Mobile Documents/com~apple~CloudDocs/Загрузки/Projects/ai/toolsy`. Source hashes at `source-sha256.json` identify the reviewed implementation/tests/README/migration/spec. Reviewer B did not read reviewer A's verdict. Repository, git and go.work.sum were not mutated by B. All B fixtures, overlays, caches and logs are outside the repository.

| Explicit row31 criterion | Score | Independent evidence |
|---|---:|---|
| 1. Vendor-neutral default / explicit independent Langfuse opt-in | 20/20 | `spanStartAttributes` emits canonical `gen_ai.tool.call.id`, operation/tool metadata; framework extensions use `toolsy.tool.*`. Default has no vendor attrs. Vendor type is opt-in, with no SDK dependency. Existing defaults/capture matrix and B external control/abort vendor matrix pass. |
| 2. Content off by default across payload paths; bounded/redacted shared fields | 20/20 | Capture gates arguments, delivered content, error/panic text and exception messages. Fixed statuses/events have no raw payload. Vendor capture=false invokes no redactor, emits no payload attr. Portable/vendor outputs reuse the same capped redacted value. Existing result/soft/hard/panic matrix, panic-redactor fail-closed fixture, and B additional delivery/error matrix pass. |
| 3. Delivered chunks only; truthful classification, success-only result; preserved semantics and UTF8 caps | 20/20 | Yield must succeed before accumulation/soft-error tracking. B rejects a soft error after accepted ordinary data for pause/yield/halt/host-event/abort/hard errors, confirms exact returned error identity, neutral control/abort versus hard error status, absence of rejected text/soft flag/result. Existing ignored-yield-error fixture verifies only accepted output. B valid Unicode caps 1/2/3/4/7/13/14/15/16/31 and invalid source/redactor caps 1/2/3/4/7/16 across all payload paths/vendor combinations pass. Panic values rethrow unchanged, no finalized result. |
| 4. AAA matrix / baseline / soft-hard and split-secret fixtures / race-lint-benchmarks | 20/20 | Repo fixtures use AAA; retained baseline behavior log fails on vendor attr, same default test currently passes. Split-secret fixture explicitly demonstrates whole-secret matcher failure and whole-chunk omission success. Independent full-module race count3, overlay adversarial race count3, pinned lint and final benchmark count3 pass. B concurrent/reentrant probe validates one shared adapter with 16 simultaneous executions, each 8 concurrent yields, plus synchronous redactor re-entry (17 independent spans; 153 redactions). |
| 5. Names/caps/opt-in docs consistent; honest sanitation boundary / scope | 20/20 | README and D25 migration agree on names, defaults, exact byte caps, normalized invalid sequences, independent opt-in, and success-only result. Split-secret limitation is prominent; full-content sanitation and raw redactor/preparation cost remain host responsibilities. No new global sanitizer, scheduler or dependency. No exporter/live-vendor conformance claimed. This report supplies B's independent 100% acceptance; parent must record both independent verdicts before final closure. |

## Validation commands and retained logs

All Go checks use `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache`.

- `go -C ext/toolsyotel test -race -count=3 ./...`: PASS 1.633s, `race-final.log`.
- `go -C ext/toolsyotel test -overlay=/tmp/toolsy-task41/d25-acceptance-b/overlay.json -race -run '^TestAcceptanceB' -count=3 -timeout=45s ./...`: PASS 2.017s, `adversarial-final-race.log`.
- From affected module: `GOLANGCI_LINT_CACHE=/tmp/toolsy-task41/d25-acceptance-b/lint-cache /opt/homebrew/bin/golangci-lint run --allow-parallel-runners ./...`: 0 issues, `lint-final.log`. Version independently checked: 2.14.0, Go1.27.1, revision114493f.
- `go -C ext/toolsyotel test -run '^$' -bench '^BenchmarkTracingMapping$' -benchmem -count=3`: PASS 15.744s, `bench-final.log`. Local no-op tracer: metadata 316.8–321.8ns /592B/9alloc, portable capture1369–1389ns /5736B/15alloc, vendor capture1845–3066ns /6504B/17alloc. Timing variance is not an improvement claim or exporter throughput measurement.

External fixture: `adversarial_test.go`; overlay: `overlay.json`. Four probe groups: control/abort/delivery classification with independent capture/vendor options; concurrent/reentrant shared adapter; valid UTF8 exact caps and vendor parity; invalid raw input/error/panic or invalid redactor result with tiny exact caps. Initial passing logs retained separately (`race.log`, `lint.log`, `adversarial-race.log`, `bench.log`).

## Initial defect and final recheck

The parent notified B of an initial invalid-UTF8 display defect in the original `truncatePayload` early return (short invalid bytes returned unchanged), before B's final verdict. B did not claim independent discovery. The corrected implementation normalizes invalid sequences to U+FFFD before cap at `ext/toolsyotel/payload.go:17`; independent raw/redactor probes exercise both below- and above-cap cases and confirm resolution. No other unresolved issue was detected.

## Primary references and limitations

Independently checked the [OTel registry](https://opentelemetry.io/docs/specs/semconv/registry/attributes/gen-ai/): canonical call.id and call.result's successful-execution meaning. Checked [Langfuse OTel mapping](https://langfuse.com/integrations/native/opentelemetry): explicit `tool` type and input/output field names. The registry currently notes movement of GenAI conventions to its dedicated repository; this acceptance covers the declared adapter subset, not complete evolving semantic-convention conformance.

No live service, external exporter or Langfuse ingestion was used. Caps bound exported display strings, not arbitrary host redactor CPU/memory or input preparation. Host redactors must be concurrent-safe; B used atomic-safe callbacks. Per-chunk matching cannot ensure whole-stream secret sanitation, as the deliberate split-secret regression fixture demonstrates. Host metadata names/call IDs must not contain secrets, per README. Tests provide evidence for the declared paths, not a universal sanitizer guarantee.
