# D36 time/state — independent acceptance B

Verdict: ACCEPTED, 100% (five criteria at 20/20 each). No unresolved detected product defect; no uncovered required criterion. Parent-only implementation reviewed without reading peer verdict, editing product/tests, or committing. Signed baseline: 9ac37e39e208611acded6b13949e3e6cdff51655, independently verified good Git signature (baseline-signature.log).

| Row36 criterion | Score | Independent evidence |
| --- | --- | --- |
| Public location/calendar/elapsed contract | 20/20 | API option comments, package doc, default calculate description, README and migration agree. Both tools resolve dynamic override on every call; input numeric offset defines the instant. Static location and disabled nil callback independently exercised, including +05:45. |
| Exact DST/signed/order fixtures; unchanged arithmetic | 20/20 | Public calendar_contract_test.go replaces three weak/misleading tests. Independent public probe checks 11 exact spring/fall ±day/±24h, positive and negative mixed-order, +05:45 input and nanosecond-fraction cases. Exact wall/offset/elapsed/weekday assertions. Entire timetool.go byte-identical to independently fetched baseline: SHA256 7ec1f3a84514a1f0b3bdf014bf12ce329c0a3c02fdf772b301d0b079af5c058e. |
| StateStore boundary and active names | 20/20 | StateStore Load/Save and RunEnv.StateStore inspected; session map API/ExportSnapshot inspected. Active runtime run.State/env.State references absent; migration historical note excluded. Independent external store/session same-key probe confirms no implicit time-tool Load/Save, snapshot contains only in-memory value, explicit external Save/Load leaves session unchanged. Memory runtime untouched and full race suite passes. |
| Runnable bounded explicit host resolver | 20/20 | Untracked examples/host source and tests included. Reads env.StateStore with host.timezone key, inclusive64 byte bound, only UTC/NY selected. Missing env/port, empty/64 unsupported/65 overflow, causal store error and cooperative cancellation fixtures pass. Dynamic provider error/nil failures independently verify no output/static fallback. |
| Tests/race/lint/example/docs/baseline evidence | 20/20 | Both modules full racecount3 and pinned lint2.14.0 independently run; host included by ./..., actual example output checked. Exact three baseline production .go files overlaid; independent public probes and new calendar/provider suite intentionally pass baseline. Parent schema-omission and magic-number initial fixture failures retained, corrected code passes. |

## Commands and results

Module full tests used GOWORK=off GOCACHE=/tmp/toolsy-review-gocache:
- timetool `go test -race -count=3 ./...`: PASS1.388s, examples/host PASS1.233s (time-race.log).
- memory `go test -race -count=3 ./...`: PASS1.900s (memory-race.log).
- Both actual `/opt/homebrew/bin/golangci-lint run --allow-parallel-runners`, version2.14.0 built go1.27.1, with separate GOLANGCI_LINT_CACHE paths: 0 issues each (time-lint.log, memory-lint.log).
- `go run ./examples/host`: noon NY after calendar day,13:00 after24 elapsed hours (example.log).
- Independent external module `go test -mod=mod -race -count=3 -v ./...`: PASS1.564s (public-probe.log).
- Same command with `-overlay=.../baseline-overlay.json`: PASS1.505s (baseline-public-probe.log).
- New public calendar/provider tests against exact baseline production overlay, racecount3: PASS1.312s (baseline-calendar-suite.log). This verifies existing behavior, not a baselineFAIL claim.

Baseline overlay covers every production Go file of timetool (timetool.go/options.go/doc.go); root stays current signed baseline. Go module manifests unchanged. Baseline-source files and overlay retained with snapshot hashes; affected candidate source/docs/tests including untracked host/public tests retained under snapshots.

## Limits and initial harness failures

No arithmetic defect claimed or fixed. Ambiguous/nonexistent wall times retain Go AddDate normalization with no promised offset choice; tzdata rules are the host/embedded version. Returned64-byte bound does not cap host callback allocation/upstream reads. Fixture is immutable local data, not a durable store; StateStore is borrowed and host owns authorization/lifetime/synchronization. No hard callback preemption or automatic timezone state integration is claimed. No live storage/backend certification.

Independent probe's first go.mod omitted quotes around absolute paths with spaces (public-probe-initial-harness.log); corrected before compile. Shared sumdb permissions denied on initial module resolve; raw logs retained (public-probe-cache-access*.log), then GOPATH and GOMODCACHE redirected to own /tmp caches and full current/baseline tests passed. An extra negative-order expected elapsed value was corrected during fixture construction to -25h before a successful execution; no product failure inferred. Parent initial missing required add_days/add_hours and lint magic64 failures remain in original parent evidence; current required fields and named maxTimezoneBytes verified.

No peer verdict was read. Substantive archive excludes Go/lint caches and unrelated checkout content.
