# D23 independent acceptance B

Decision: ACCEPTED. Total: 100%. No unresolved detected correctness findings.

Read-only acceptance against baseline HEAD c7eb470fb2d62f31e9072e487ec5beba578b3e88 and the complete current tracked/untracked D23 candidate in contracts/grpc, migration, progress row29, and docs/reviews/task41/d23. No repository files changed, no commit created, no peer acceptance report read.

| Criterion | Earned | Evidence |
|---|---:|---|
| 1. Prefetch filtering and deduplication | 20/20% | List entries are filtered for nonempty/non-reflection/allowed names before buildRegistry issues descriptor RPCs. Duplicate selected names fetch once. Real fixture excludes broken.Service and succeeds. |
| 2. Finite inclusive untrusted-work budgets | 20/20% | Zero selects 256 listed entries, 512 received blobs, 8MiB canonical protobuf response bytes. Negative discovery and execution limits reject before stream creation. Full list length includes excluded/empty/duplicate entries. Every blob charges before decoding/deduplication; every list/file response charges proto.Size including envelope/unknown fields. Subtraction-based byte accounting avoids accumulating beyond cap. Real exact/+1 tests pass; supplemental unknown/envelope repeated-byte probe passes. |
| 3. Dependency retention, conflicts, atomic publication | 20/20% | All returned selected dependency blobs enter protodesc.NewFiles. Equal duplicate descriptors merge only after charging, unequal same-name identities fail. Registry/schema/tool errors return nil, never previously constructed tools. Existing supported-schema/streaming rejection remains unchanged. |
| 4. Executable fixtures and affected-module validation | 20/20% | AAA bufconn reflection fixtures cover excluded failure, exact/+1 service/files/bytes caps, repeated blobs, selected dependency/conflict, parent cancellation, receive cap and borrowed connection reuse. Independent full module race count3 passes; pinned lint passes. Supplemental real success-path stream cancellation probe passes even when server waits for context and ignores send-side EOF. Baseline behavioral failure independently reproduced using Go overlays. |
| 5. Public contracts and boundaries | 20/20% | README, Options, migration and row29 agree on defaults, inclusive limits, canonical response sizes, exclusions/dependencies, negative rejection, ErrDiscoveryLimit and earlier gRPC receive errors. Host retains deadlines/authentication/authority/connection ownership. Per-message decode/heap limitation is explicit. No dependency/manifest/core/scheduler changes. This report supplies B's independent acceptance; the coordinator must separately obtain A's acceptance without using this report to substitute for it. |

Findings: none requiring changes.

Independent checks:
- GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go test -race -count=3 . from contracts/grpc: PASS, 4.742s.
- /opt/homebrew/bin/golangci-lint version: 2.14.0, built with go1.27.1, commit114493f.
- GOWORK=off GOCACHE=/tmp/toolsy-review-gocache GOLANGCI_LINT_CACHE=/tmp/toolsy-task41/d23-b-lint /opt/homebrew/bin/golangci-lint run --allow-parallel-runners ./... from contracts/grpc: PASS, 0 issues, exit0.
- git diff --check: PASS.
- Both retained baseline-source SHA256 values match git show c7eb470 blobs.
- Baseline reflection.go/options.go with retained prefetch fixture and discovery_budget.go removed through /tmp Go overlay: expected FAIL, tools=0, excluded-service reflection error; compilation succeeded.
- Supplemental /tmp overlay tests TestReviewBOwnedStreamCancelledOnSuccess, TestReviewBCanonicalEnvelopeUnknownByteBudget, TestReviewBZeroDefaults, race count3: PASS, 1.585s.

Check limitations: the initially requested nonexistent binary path ending in 2.14.0 was corrected to the actual version-verified binary. Initial lint used a sandbox-inaccessible default Go cache and exited7; rerun with writable GOCACHE passed. Those invocation failures are environmental, not candidate defects. No live remote conformance, arbitrary hostile allocation bound, process heap cap, or distributed cancellation claim is made. Aggregate checks run after gRPC receive/decode; adapter descriptor decode/append/registry compilation is guarded, and additional memory is explicitly documented. Supplemental probes and overlay files reside solely under /tmp/toolsy-task41/d23-b-probe.
