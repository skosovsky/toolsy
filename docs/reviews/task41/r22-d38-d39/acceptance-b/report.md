# Independent acceptance B — row38 R22/D38/D39

Accepted, 100% for this row. No unresolved defects or missing acceptance evidence detected in the reviewed scope. This is my independent result; the other reviewer verdict was neither consulted nor certified here.

| Criterion | Score | Evidence |
| --- | --- | --- |
| Normative DTO/presence table | 20/20 | Compared against actual mapGoType/mapNullableGoType/mapGoTypeArray and emitted DTO; root, required/optional string/date-time, pointer integer/bool, required pointer-to-slice, optional slice, primitive value items, raw nullable unions, defaults and RawJSON behavior agree. README points to one table and removes wrong shorthand. |
| Compiling/executing generated fixtures | 20/20 | Public fixtures and my overlay probes execute real NewPresenceTool/Execute dispatch. Omitted/null/zero/empty/items and exact 900719925474099300000, 1.0, 2e2, 1e30 lexical representations pass. Invalid root/type/nonintegral/item inputs reject without handler. RawJSON exact byte identity, unknown property retention, absent optional default, empty optional slice and omitted raw-nullable DTO reserialization differences proven. |
| Runnable generator/handler/stream/nested contract | 20/20 | CLI generated both temp manifests byte-identically to checked-in files. Presence run matches documented output. Stream run schedules, emits two progress and one terminal result, waits callback/shutdown. Nested example and tests verify nested executable schema and pre-dispatch rejection of string workaround, invalid items and missing payload. Flat generator is unchanged. |
| Current docs/module map/history | 20/20 | Current index, migration and exact example commands reviewed. All 24 entries match go.work and actual go.mod module paths. Version placeholders explicitly avoid release publication claims. Fifteen indexed task28–35 historical files byte-identical against a87f5ea; paths preserved. |
| Verification/evidence consistency | 20/20 | Own full uncached root race, targeted race with adversarial overlay, pinned v2.14.0 lint 0 issues, CLI identities, all 13 indexed examples and 3 toolkit recipes exit0, 162 local Markdown file targets exist. No repository mutation/staging/commit/publish/push. |

Sum: 100/100 (100%).

Executed checks:
- GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go test -race -count=1 ./...: exit0. Core 5.288s, internal/toolsygen 41.566s, internal/release 48.735s. Includes generated consumer lifecycle/stream/async fixtures and source contract minLength/minItems/unsupported-shape regressions.
- Same environment, go test -race -count=1 -overlay=<overlay.json> ./examples/generated_presence ./examples/nested_contract ./cmd/toolsy-gen: exit0; 2.177s/1.234s/1.228s. After adding isolated invalid-value cases (other required fields retained), repeated overlay generated_presence: exit0.
- /opt/homebrew/bin/golangci-lint run --allow-parallel-runners with unique lint cache and GOWORK=off: exit0, 0 issues; binary reports v2.14.0 built go1.27.1.
- CLI generation operates on two manifests and package stubs under this evidence directory, without whole checkout copies or source regeneration writes.
- Approval index command uses its required -directory and -operation with a private existing temp directory; observed pending action is the documented expectation. Other examples output inspected in cli-examples.log.
- Four generator/CLI production Go files byte-identical to a87f5ea, independently proving no generator behavior extension; historical hashes and candidate hashes retained beside this report.

Limits: local source checkout acceptance only. Root race does not run every nested module test suite; indexed nested examples were run separately. No live cloud/service isolation, published module graph or production release is claimed. Date-time is an annotation: my probe confirms even invalid date text reaches the string DTO, matching current contract. Raw nullable values normalize internal whitespace while RawJSON preserves the accepted original byte sequence. Markdown checks verify local target existence; no external link availability claim. Aggregate two-reviewer/task41 final gate remains the parent task's responsibility.
