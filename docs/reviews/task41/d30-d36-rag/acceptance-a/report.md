# Acceptance A — row 33 D30/D36 RAG

Verdict: **ACCEPTED, 100%**. Independent review of uncommitted implementation, executable host recipe, README/API and migration edits against baseline `fe241b0f4f1ecacacd0842107410ab56f30601ea`. No repository files, git state, go.work.sum, docs or implementation tests were edited by this reviewer. All private probes/reconstructed baseline/outputs are in this directory. No peer verdict was read. Second reviewer and ledger certification are coordinator responsibilities.

| Criterion | Score | Evidence |
|---|---:|---|
| 1. Thin single-call retriever, routing API removal | 20/20 | Aggregate/Dedup/DedupBy/Fallback deleted with no new router/SDK/dependency. AsSearchTool performs at most one Retrieve invocation; original errors remain inspectable; nil/typed-nil required ports rejected. |
| 2. Executable host policy | 20/20 | Required endpoints/predicate validated; explicit unavailable/empty fallback; terminal cancellation/deadline; error joins preserve causes; merge uses public source/chunk identity, preserves unidentified units. Discovered policy-callback cancellation defect fixed and independently retested. |
| 3. Bounds and one final wire representation | 20/20 | Redundant whole-envelope pre-encode/shallow clone removed. Per-unit escaped JSON item/source accounting retained before and after filter; postfilter count; validator precedes single final wire cap. Exact limits and custom DTOs verified publicly. No result omission. Borrowed read-only ownership documented. |
| 4. Regressions, race/lint and benchmark | 20/20 | Final full-module race×3 and private adversarial overlay race×3 PASS; pinned golangci-lint 2.14.0 zero issues. Original fallback failure independently reproduced behaviorally. Identical benchmark fixture independently rerun baseline/current. Renderer empty regression retained in markdown_test.go. |
| 5. Truthful docs/migration and no unresolved detected defects | 20/20 | README, API comments and migration29/30/41 agree on removals, finite defaults, provider/output resource distinction, cancellation and host ownership. No automatic ragy/routery or live-service claim. No unresolved implementation findings from A. |

## Discovered and resolved finding

Original `toolkits/rag/examples/host/main.go:56–57` returned primaryErr immediately when allowFallback returned false. If that callback synchronously canceled the parent with a custom cause while primaryErr was nonnil, Retrieve returned the primary error before its later cancellation check. Private TestAcceptancePolicyCancelFalsePreservesCause reproduced FAIL×3 under race: `primary denied`, while ctx.Err()==context.Canceled and context.Cause()==policy canceled; downstream providers were correctly skipped but cancel/cause were lost.

Final `main.go:56–61` stores the decision, checks interruption after the policy callback unconditionally, joins primaryErr and interruption, then branches. Private probe now PASS×3; committed-to-working-tree AAA regression tests both bool decisions and preserves primary/context/custom causes.

## Independent runs

All Go runs: `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache`.

* `go -C toolkits/rag test -race -count=3 ./...`: PASS, rag1.426s and host1.368s, race-final.log.
* `go -C toolkits/rag test -race -count=3 -overlay=/tmp/toolsy-task41/d30-rag-acceptance-a/overlay.json -run '^TestAcceptance' ./...`: PASS, rag1.664s and host1.589s, probe-final.log. Public exact default Markdown/JSON and custom DTO wire caps; actual WireLimitError size/limit; validator invocation before rejected wire; validator causal error; exact escaped provider item/source limits; provider bounds cannot be bypassed by filter/small formatter; expanded filtered count; small final DTO with larger accepted provider. Host direct canceled/deadline errors do not reach predicate/providers; source-scoped identity preserves distinct chunks and unidentified units; terminal policy cancellation retains parent cause.
* `/opt/homebrew/bin/golangci-lint run --allow-parallel-runners`, module cwd and `GOLANGCI_LINT_CACHE=/tmp/toolsy-task41/d30-rag-acceptance-a/lint-cache`: PASS, 0 issues, lint-final.log. Version verified 2.14.0 built go1.27.1. Initial concatenated binary path was corrected by coordinator.
* `go -C toolkits/rag run ./examples/host`: PASS, host-example.log, two public chunks from one source.
* Own baseline reconstruction uses `git show fe241b0:...`; all nine retained baseline *.go.txt snapshots byte-for-byte match baseline. baseline-snapshot-verification.json. New root files hidden by overlay; benchmark unchanged.
* `go -C toolkits/rag test -count=1 -overlay=.../baseline-overlay.json -run '^TestAcceptanceBaselineFallbackCause$' .`: expected behavioral FAIL: failed primary + nil fallback returns nil error. baseline-probe.log. This is the original removed API behavior; no claim that the removed-API probe compiles on current code.
* Identical public Execute benchmark fixture SHA256 `be3d52f15a85f8fdbf474054a595de6c88e615d2e0997ec7cd4be0fd4258eecd`; `test -run '^$' -bench '^BenchmarkSearchDocuments$' -benchmem -count=1 .`, baseline adds baseline-bench-overlay.json. Baseline1611alloc/133026B; current1597alloc/130399B. bench-baseline.log and bench-current.log. Supports ~14alloc/2.6KiB lower allocation for this fixture. Timing deliberately not interpreted as speedup.

## Limits

Reviewed final API/README/migration clarification: cardinality is at most once; an already-cancelled/expired request can be rejected by existing core predispatch admission with its standard interrupt, without a custom-parent-cause promise or provider invocation. Once admitted, RAG handler checks preserve custom causes at its own callbacks. This matches code and confines the change to row33.

No live backend or provider allocation/preemption/availability guarantee tested. Host callbacks retain cooperative cancellation and borrowed read-only ownership responsibilities. Constructor cap/nil-option validation D32 remains a separate future row; unchanged existing finite nonpositive defaults were not expanded into new scope. Coordinator refreshes repository evidence source checksums after the host fix and retained renderer test; reviewed-source-sha256.json identifies A’s reviewed snapshot.
