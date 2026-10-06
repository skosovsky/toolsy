# D30/D36 RAG verification

Baseline HEAD `fe241b0f4f1ecacacd0842107410ab56f30601ea` (signed D27).
Original module source captured before edits under /tmp/toolsy-task41/d30-baseline,
retained as baseline/*.go.txt. Overlay restores originals and hides new nonbenchmark
root files; same external API benchmark fixture runs against both implementations.
Only module root benchmark run, no live provider. Overlay JSON retains scratch
paths for reconstruction; original-files identifies snapshots.

Behavioral baseline: `go -C toolkits/rag test -run '^TestBaselineFallbackPreservesPrimaryFailure$' -count=1 -overlay=/tmp/toolsy-task41/d30-baseline/probe-overlay.json .` under GOWORK=off/GOCACHE as below. Expected assertion FAIL: failedprimary+nilsecondary returned nil,nil. API now deliberately removed; no claim that this removed-API probe compiles on current tree. Host recipe separately rejects absent required endpoint and tests allowed/denied/terminal/causal fallback. This is not a compile-only defect claim.

Final `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go -C toolkits/rag test -race -count=3 ./...`: PASS rag1.730s/host1.646s.
Pinned /opt/homebrew/bin/golangci-lint2.14.0 module run --allow-parallel-runners dedicated cache:0issues.
Host executable `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go -C toolkits/rag run ./examples/host` PASS, two distinct chunk IDs/source retained. Targeted AAA tests cover singleprovidercall/noRetry/causalerror, customcancel at provider/filter/formatter/validator, downstream suppression, exact wire budgets/customDTOs/escaping, required nil/typednil ports. Existing provider/filter/source/item/count/provenance tests retained.

Identical BenchmarkSearchDocuments fixture: five Unicode/escaped documents with metadata, default JSON, full public Execute; constructor excluded. Commands GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go -C toolkits/rag test -run '^$' -bench '^BenchmarkSearchDocuments$' -benchmem -count=3 .; baseline adds -overlay=/tmp/toolsy-task41/d30-baseline/overlay.json. M1 Max darwin/arm64. Baseline1611alloc/~133076B; current1597alloc/~130429B. ~14alloc/2.6KiB lower; walltime noisy, no speedup/generalperformance claim. Benchmark predates last constructor-only nil-port guard; timed execution path unchanged.

Per-document JSON encoding remains required for actual escaped item/source bytes.
Whole-envelope pre-encode/shallowclone removed; final format pipeline encodes once.
Borrowed host values stable/read-only, metadata not deepcopied. No new RAG router,
provider SDK/dependency, live retrieval service or availability proof. Independent
acceptances pending.

## Acceptance corrections

A independently detected host predicate cancellation + false decision bypassed
terminal context check. Fixed host predicate postcheck for both booleans, preserving
primary and custom cancellation causes; repo regression covers both decisions.
Corrected parent full rag/host race count3 PASS1.333s/1.457s; pinnedlint0. Moved
retained Markdown-empty renderer regression out of removed router tests; targeted
race3PASS. Source manifest updated to corrected Go/docs snapshot.

B independently demonstrated already-expired predispatch core guard (unchanged at
baseline) omits custom context.Cause; README/migration now explicitly restrict
customcause guarantee to RAG handler callback boundaries after admission. Core
predispatch retains standard interrupt and calls no provider. This is an existing
core limit, not a new regression or narrowed original D30 terminality requirement.
No attempt to claim customcause support at every generic core boundary. Latest
reports must recheck docs; original probe failure retained by B.

## Final independent acceptance

A and B each accepted100% (five20/20), no unresolved detected errors. Both final
fullmodule/host race, adversarial probes, lint2.14.0=0, baselinebehavior and identical
benchmark verified. Final README/migration predispatch/handler distinction read by
both. Reports/rawlogs/probes retained acceptance-a/b; original failure evidence
kept and superseded. Separate signed commit authorized after both final verdicts.
