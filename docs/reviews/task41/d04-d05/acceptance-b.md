# D04 / D05 acceptance B

Reviewer: /root/d04_d05_acceptance_b. Accepted: 100%. No unresolved detected defects.

Each of the five row25 criteria: 20/20.
1. Primary Policy/Decision, stable ID, removed legacy authorization and narrow
   cause-preserving adapter.
2. Sticky nil/typednil/nilfunc construction; view/restore; requirements and typed
   argument policy retained.
3. Required budget; optional absence only; malformed supplied DI fails closed.
4. AAA regressions, snapshots/trusted view, cancellation/reentrant callbacks,
   replay/async completion and verification.
5. Current contracts/migration/executable example/captured port/BYOT ownership.

Independently read entire production diff/new files. No legacy Go entry points.
Composed policies isolate framework containers; BYOT references host-owned.
Budget callback outside store lock; gate before replay; missing dependency reaches
async completion callback as error.

Independent full root `go test -race -count=1 ./...` PASS including example:
raw log d04-d05-review-b-race.log. Pinned lint separate cache 0 issues:
d04-d05-review-b-lint.log. Original public overlay under race on current tree PASS.
Git diff --check PASS. Inspected baseline three behavioral failures and current
MCP/human/OTel race/lint PASS. Did not read peer verdict or modify files.

Limits: no live host authorization/pricing conformance; cancellation cooperative;
referenced BYOT values/lifetime/synchronization remain host responsibilities.
Dual acceptance separately confirmed by leading agent.
