# D02 independent acceptance B

Baseline HEAD: 8ffd009. Reviewed current tracked diff and untracked control_contract_test.go, docs/control-contract.md and examples/host_event/main.go. Read task41 D02 and row23 criteria. No implementation edits, delegation or reviewer A report access.

## Criteria (20% each)

1. **20/20**: UIActionSignal/ErrUIAction removed from executable Go API. Sealed HostEventSignal contains bounded named JSON data; no dispatcher, action port or authority added. Host example explicitly allowlists event and panel names.
2. **20/20**: Pause/Yield/Halt comments and normative contract specify host continuation requests only. No registry/session/context cancellation, durable continuation, scheduler or grants. CompletionPolicy documented as host routing hints. Public regression independently executes an unrelated subsequent tool with the same context.
3. **20/20**: Non-nil built-in signals, UTF-8, routing syntax/128-byte name, inclusive 64KiB individual and aggregate fields, count64 and strict host JSON checks are enforced. Invalid declarations precede error normalization and use INTERNAL nonretryable ResultContractError(control_contract). Built-in delivery clones structs/payload; terminal results and codec use prepareChunk. Old ui cache kind is rejected. Independent overlay verified decode count/aggregate/name/duplicate/payload misuse and terminal snapshot.
4. **20/20**: Permanent AAA coverage includes malformed controls, exact boundaries, consumer errors, nil callback, typed result controls, codec, registry continuation and middleware. Existing typed cache outcome and diagnostic-control regressions cover integration. Runnable allowlist example succeeded. Independent root/human/OTel race and lint passed.
5. **20/20**: README, package/control/completion comments, migration and control-contract document clear API/persistence break, exact limits and cooperative delivery/allocation boundaries. Legacy executable API absent. This reviewer finds no unresolved defect; second acceptance remains an independent parent gate, not a basis for this score.

**Total: 100%. Verdict: accepted.** Found errors/regressions: none. Uncovered criteria: none.

## Independent commands and evidence

- GOCACHE=/tmp/toolsy-review-gocache go test -race ./...: PASS; root5.454s, generated fixture suite51.319s. /tmp/toolsy-task41/d02-b-root-race.log
- go -C toolkits/human test -race -count=1 ./... with same cache: PASS1.438s. d02-b-human-race.log
- go -C ext/toolsyotel test -race -count=1 ./...: PASS1.396s. d02-b-otel-race.log
- golangci-lint2.14.0 run --allow-parallel-runners ./... in root, human and OTel with explicit GOCACHE and separate private lint caches: each0 issues, exit0. d02-b-{root,human,otel}-lint.log. First root lint attempt lacked explicit GOCACHE and failed cache sandbox access; successful rerun supersedes it.
- go test -race -overlay /tmp/toolsy-task41/d02-b-overlay.json -run '^TestD02B' -count=5 .: PASS1.834s. d02-b-adversarial.log. Fixture and overlay retained in /tmp/toolsy-task41.
- go run ./examples/host_event: successful allowlisted settings routing and host scheduling message. d02-b-example.log
- git diff --check: clean.

## Limits

Race testing exercises covered schedules; it is not proof of all possible races. Mutable delivered controls remain consumer-owned and concurrent mutation during capture/encoding is outside the documented contract. Finite field/count validation occurs after producer allocation and cannot preempt producers ignoring callbacks. No real UI integration, durable pause/resume or scheduler is implemented or claimed. No production push/publish performed. Acceptance is against reviewed current D02 scope, not a guarantee that unrelated remaining task41 decisions are complete.
