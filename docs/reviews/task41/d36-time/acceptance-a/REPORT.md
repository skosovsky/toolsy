# Independent acceptance A: task41 row36 D36 time/state

Verdict: ACCEPTED, 100% (five criteria, 20/20 each). Baseline: signed 9ac37e3.
Review conducted independently, without peer verdicts. No product/test edits or commits.
Reviewed tracked diff plus untracked calendar_contract_test.go and examples/host.
Reviewed source hashes in reviewed-sha256.json; tracked reviewed.diff and affected
baseline snapshots retained. Arithmetic timetool.go compares byte-identical to baseline.

1. 20/20 — API option/package/default tool descriptions, README, migration and ledger
agree: both current and calculate resolve host location, provider takes priority per
call; RFC3339 offset identifies an instant; conversion to host zone precedes calendar
AddDate and elapsed Add. Fractional precision remains. nil provider disables override;
provider nil location/error fails without fallback. No new arithmetic fix claimed.

2. 20/20 — Ten exact public fixture cases cover forward/reverse spring/fall days
(23/25 hours), forward/reverse elapsed24 hours, days-before-hours ordering, and zero
operation fractional base with foreign numeric offset. Exact result offset/time,
elapsed delta and weekday asserted. Old incorrect 07Z=02EST comments and vacuous hour
range assertions removed. README order example checked against fixture. Ambiguous/
nonexistent wall time is explicitly Go normalization with no offset-choice promise.

3. 20/20 — Active source/docs audit found no remaining executable legacy run.State,
env.State or RunEnv.State fields. Migration mentions historical run.State explicitly
as history. RunEnv.StateStore/WithStateStore expose borrowed Load/Save; session map
and ExportSnapshot implementation are distinct; external store excluded. Memory
retains same StateStore scratchpad contract. Independent public probe with trap store
confirms neither time tool automatically reads/writes it.

4. 20/20 — Runnable example explicitly wires StateStore/provider and accepts only
host-selected UTC/NY; 64-byte returned-value check, missing port, storage error,
canceled cooperative store,65-overflow,exact64 unsupported and empty zone tested.
Immutable fixture is honestly local/read-only, not durable proof. Current+calculate
provider resolution asserted on four calls; provider error and nil-result/no-fallback
covered. Independent probes additionally check nil provider disables previously set
callback, static location governs both tools, exact foreign-offset fractional result,
callback context/env identities when invoked, cancellation/no output and nil result
priority. Context may be framework-prechecked; no hard callback preemption is claimed.
Host remains responsible for callback allocations, upstream bounds, synchronization.

5. 20/20 — Independently executed with GOWORK=off, GOCACHE=/tmp/toolsy-review-gocache:
- full time+host go test -race -count=3 ./... PASS1.393s/1.239s
- full memory go test -race -count=3 ./... PASS1.797s
- pinned /opt/homebrew/bin/golangci-lint version2.14.0; run
  --allow-parallel-runners ./... using distinct reviewer caches: both0 issues
- host example prints calendar12:00-04 versus elapsed13:00-04, both Sunday
- /tmp public adversarial consumer probes race count3 PASS1.666s
- exact baseline git archive with current public contract test only overlaid:
  targeted public DST/provider race count3 PASS1.529s, intentionally
- independent adversarial probes against exact baseline production PASS1.484s
- git diff --check passed.

Parent initial required-delta/schema and magic-number lint failures reviewed and
retained in product evidence; corrected fixtures now reach intended callbacks.
Reviewer first consumer run lacked dependency checksums and hit read-only sumdb
cache; reused repository go.sum and reran successfully, retaining initial log.
Some read commands initially used wrong cwd/filename; corrected; no material audit gap.

No uncovered acceptance criteria or unresolved errors. No live durable persistence,
independent tz database guarantee, distributed store atomicity, or hard cancellation
proof implied. Existing arithmetic passes baseline as required. Temporary full
baseline checkout /tmp/toolsy-d36-a-baseline and caches are intentionally outside
archived evidence; affected baseline snapshots/probes/logs/report are all in this
acceptance directory. Before parent commit, both independent verdicts must be100%.
