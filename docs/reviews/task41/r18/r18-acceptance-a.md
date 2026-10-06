# R18 / D31 independent acceptance A

Reviewed working-tree implementation against HEAD `a8bf9fb`, task41 R18/D31 and row18 criteria. Read tracked memory/docs diff, untracked cancellation_public_test.go and gate_contract_test.go, unchanged options/bounds tests and generated output schema. No repository mutation, implementation delegation, or other reviewer verdict consulted.

| Criterion | Score | Evidence |
| --- | --- | --- |
| 1. Cancellable pin/read/unpin admission | 20/20 | Public tests exercise all three tools with deadline and explicit cancellation while initial host Load owns admission; return before provider release, errors.Is context cause, no extra Load/Save. acquire checks before/select/after acquisition and releases raced cancellation; load checks before host callback. Baseline public probe logs show all six scenarios fail on original mutex. |
| 2. Serialization, release and ownership | 20/20 | One fixed-capacity channel per instance encloses full Load/modify/Save, with deferred release in all handlers. Existing concurrent update test preserves all ten facts; no implementation waiter goroutine or session lock registry. Post-Load cancellation prevents Save. Additional independent overlay proves pre-canceled call performs zero callbacks and Save failure releases gate for subsequent successful pin. Callback cooperation/reentrancy limits explicit. |
| 3. Structured exact bounded representation | 20/20 | readResult Facts is map[string]string; empty map remains nonnil and encodes object. Escaping/exact wire boundary tests cover newline, equals, XML-like text, NUL, CR and Unicode. Full json.Marshal bounds count escaped keys/values and object wrapper. Independent overlay verifies generated facts schema type object with string additionalProperties. State key/map storage, action arguments and status constants unchanged. |
| 4. Accurate docs and migration | 20/20 | README, package docs and migration specify session scratchpad, instance-wide serialized coordination, no fairness/distributed CAS, no copying or synchronous callback reentry, callback allocation/context responsibility and no rollback of started Save. Clear break from legacy key=value facts described. |
| 5. Regression and verification | 20/20 | Independent all-module race repeated three times passed; pinned lint with private cache and allow-parallel-runners 0 issues; additional adversarial overlay repeated five times with race passed. Source/doc diff check clean. Existing bounds, malformed/duplicate state, nonmutation, read-only policy and output preflight tests retained. This is this reviewer's independent acceptance; other reviewer gate is orchestrator responsibility. |

Total: **100% (100/100)**. Verdict: **ACCEPT**. Unresolved detected errors: **none**.

Own verification:
- `GOCACHE=/tmp/toolsy-review-gocache go -C toolkits/memory test -race -count=3 ./...` → PASS 3.413s; `/tmp/toolsy-task41/r18-a-race.log`.
- Pinned `/opt/homebrew/bin/golangci-lint run --allow-parallel-runners ./...`, memory module, private `r18-a-lint-cache` → 0 issues; `/tmp/toolsy-task41/r18-a-lint.log`.
- Temporary overlay `r18-a-probe_test.go`, `-race -count=5 -run TestR18A` → PASS 1.627s; `/tmp/toolsy-task41/r18-a-probe.log`, overlay JSON retained in /tmp.

Limits: no distributed/multi-process atomicity or hard interruption of arbitrary host callback certified. Cancellation can race immediately after a check; implementation does not claim atomic cancellation vs callback dispatch or rollback. Channel capacity bounds coordination state, not number of caller goroutines created by a host. Race/test evidence concerns supported local one-instance contract and is not proof against all schedules. No new infrastructure dependency or stored data migration required.
