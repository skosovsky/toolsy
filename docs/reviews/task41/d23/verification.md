# D23 pre-fetch filtering and aggregate reflection budgets

Baseline c7eb470, source SHA256 manifest retained. Before production edits,
TestReflectFiltersBeforeDescriptorRequests failed behaviorally because excluded
broken.Service was requested. Retained prefetch-probe.go.txt supplies the same
real bufconn reflection fixture and assertion; current test adds full quotas and
supported-path checks. Probe can overlay contracts/grpc/reflection_limits_test.go
on baseline/current, with existing schema_test.go fixture. Baseline compilation
succeeded and the failure came from the excluded service response, not build error.

Public Reflect now filters before descriptor fetch, deduplicates selected names,
and retains selected service dependency blobs. Aggregate service entry/files/bytes
limits count exclusions/duplicates/envelopes before descriptor decoding/compilation.
Negative discovery/execution caps fail before RPC. Child discovery context is
cancelled on return; borrowed connection remains usable after refusal/cancellation.
Conflicting same-name files reject. Per-frame gRPC receive and aggregate adapter
limits are separate; canonical protobuf size/decoded memory limits documented.

Parent full affected module race count3 PASS5.308s, pinned lint0. Tests cover exact
and +1 service/blob/aggregate response-size bounds, duplicate charging, dependency
projection/conflicts, parent cancellation, per-message refusal and connection reuse.
No partial tools returned. No live arbitrary server/backend conformance asserted.
Two independent final reviewers accepted all five criteria20/20 (100%), no unresolved detected errors. See acceptance-a.md and acceptance-b.md; additional overlay fixtures retained as acceptance-a-probe.go.txt / acceptance-b-probe.go.txt. A checked malformed/missing imports/nil/unknown/default/max-int and child stream cleanup; B reproduced baseline and verified success-path stream cleanup even with server ignoring CloseSend, plus unknown/envelope and zero defaults. Both independent full module race count3 and pinned lint0 passed.
