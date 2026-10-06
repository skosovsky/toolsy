# D04 / D05 acceptance A

Reviewer: /root/d04_d05_acceptance_a. Accepted: 100%. No unresolved detected defects.

Each of the five row25 criteria: 20/20.
1. Primary Policy/Decision; stable binding ID; removed legacy APIs and separate
   authorizer path; narrow adapter preserves denial cause.
2. Sticky nil/typednil/nilfunc configuration; View/RestoreView checks; intentional
   omission; requirements guards and typed argument policy retained.
3. Mandatory budget fails closed; optional bypass only for absent dependency;
   invalid supplied values block dispatch/replay.
4. Baseline/current probes, snapshots/trusted identity, cause/no repair,
   cancellation/reentrant callback, replay and async completion covered.
5. Migration/current contracts/executable example agree; captured authorizer,
   RunEnv budget dependency and referenced BYOT host ownership documented.

Independent checks: root `go test -race -count=1 .` PASS 5.368s; original public
fixture on current tree `-race -count=3 -overlay ...` PASS 2.003s; pinned lint
with separate cache 0 issues; git diff --check PASS. Inspected baseline FAIL and
current full-root/MCP/human/OTel race/lint logs. Did not read peer verdict or
modify files.

Limits: no live host authorization/pricing conformance; cancellation cooperative;
referenced BYOT values/lifetime/synchronization remain host responsibilities.
Percentage measures explicit criteria, not universal absence of bugs.
