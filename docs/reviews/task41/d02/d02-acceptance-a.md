# D02 independent acceptance A

Baseline: HEAD `8ffd009`; reviewed current unstaged diff and untracked `control_contract_test.go`, `docs/control-contract.md`, `examples/host_event/main.go`. Read task41 D02 and row23 criteria. Read-only review; no implementation edits, delegation, or reviewer B report access.

## Completeness: 100% (5 × 20%)

1. **20/20 — Neutral host boundary.** UIActionSignal/ErrUIAction are removed from executable Go APIs. Sealed HostEventSignal carries Name and optional JSON bytes, maps to ErrHostEvent, and has no dispatcher, authority issuer, generic execution callback, UI API, or shell dependency. Host example uses an explicit name and panel allowlist.
2. **20/20 — Precise control scope.** Pause/Yield/Halt documentation defines host continuation requests and excludes scheduler/global cancellation/durable resume/grants. CompletionPolicy is a host hint. Public registry regression covers each signal and an unrelated subsequent call despite CompletionHalt. Terminal result Controls remain declarations without automatic sentinel return.
3. **20/20 — Bounded output contract.** controlSize and validateControlDeclarations enforce typed/non-typed nil rejection, UTF-8, strict duplicate/trailing/depth JSON validation, inclusive 64KiB signal/list bytes, 64 controls, and 128-byte ASCII routing names. Invalid outputs become nonretryable INTERNAL ResultContractError(control_contract). Validation runs before error normalization. YieldControl and prepareChunk copy signal structs and host payload; codec encode/decode reuses validation and legacy ui kind is rejected.
4. **20/20 — Regression and composition evidence.** New AAA tests cover invalid forms, inclusive bounds, producer aliases, callback errors, typed result/codec, legacy cache, middleware and unrelated registry calls. Independently ran root, human and OTel race/lint and runnable allowlist example. Additional external-package overlay tests confirmed terminal producer alias isolation, malformed cached duplicate/trailing/depth payload rejection, and malformed host controls bypassing argument-repair error formatting.
5. **20/20 — Migration/documentation.** README, control-contract, API comments and migration describe exact API/cache break, field limits, delivery-only bounds, producer cooperation and host-owned authorization/routing. Current executable source has no legacy UI API. Historical task/review/migration references are intentional records of removed API. Independent A checks are complete; parent must separately enforce peer acceptance before commit.

## Errors and regressions

No unresolved defects found. No uncovered implementation criterion. This assessment covers the current D02 diff; it is not a guarantee against arbitrary future defects.

## Independent commands and results

- `GOCACHE=/tmp/toolsy-review-gocache go test -race ./...` — PASS (root 5.806s, toolsygen 51.316s; other packages passed/cached). Log `d02-a-root-race.log`.
- `GOCACHE=/tmp/toolsy-review-gocache go -C toolkits/human test -race ./...` — PASS 1.823s. Log `d02-a-human-race.log`.
- `GOCACHE=/tmp/toolsy-review-gocache go -C ext/toolsyotel test -race ./...` — PASS 1.601s. Log `d02-a-otel-race.log`.
- `/opt/homebrew/bin/golangci-lint run --allow-parallel-runners ./...` — root/human/OTel each 0 issues, distinct `d02-a-{root,human,otel}-lint-cache` directories. Logs `d02-a-{root,human,otel}-lint.log`.
- `go run ./examples/host_event` — PASS: routed settings request, then confirmed scheduling remains host-owned. Log `d02-a-host-example.log`.
- `go test -race -overlay=/tmp/toolsy-task41/d02-a-overlay.json -run '^TestD02AProbe' -count=5 .` — PASS 1.562s. Read-only overlay source `d02-a-probe_test.go`, log `d02-a-probe-race.log`.

## Limits

Reviewed D02 affected scope, not final all-24-module acceptance. No actual UI application is invoked: example demonstrates the host adapter boundary. Delivery/persistence limits do not prevent producer allocations, preempt uncooperative producers, grant authority, or impose whole-invocation control quotas. Consumers must not concurrently mutate captured Go aliases; docs explicitly state that limit. Legacy ui persistence intentionally requires host migration.

**Verdict: принято — 100%, no detected unresolved errors.**
