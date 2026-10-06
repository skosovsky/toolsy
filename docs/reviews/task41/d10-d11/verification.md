# D10 / D11 configuration and retained recovery boundary

Baseline: commit/source SHA256 in baseline-sources.json. Production bytes matched
that commit before edits. The retained public probe supports both old positional
and current named configuration through reflection; it executes no host callback.
Both typednil Store and Codec were accepted on baseline (two behavioral assertion
FAIL, no compile failure). The same fixture under current production PASS count3
with race. Reproduce via Go overlay mapping a nonexistent root d10_probe_test.go
to the retained .go.txt fixture and run TestD10PublicTypedNilProfileProbe.

OperationProfileConfig replaces seven positional arguments. All real caller files
migrated; configuration validates required nil/typednil ports, callbacks, empty
issuer, positive lease and nonnegative cap, preserving zero 1MiB default. Config
capture is by value; host port state/lifecycle/concurrency remain host-owned.
AAA matrix and snapshot tests verify the constructor invokes no host callback.

D11 is retained deliberately: OperationStore claim/finish/reconcile source and
transitions are unchanged. Original consumed GrantID, AllowRecovery, unexpired
approval and fenced trusted retry authorization remain necessary. A fresh grant
cannot reopen the same logical operation, even after original approval expires.
New store tests exercise valid/expired originals and stale attempt fencing. An
actual registry/profile handler counter stays1 after fresh-grant and expired-
original refusals. No new refresh API, retry scheduler or bypass was introduced.
Audited host reauthorization/new intent and downstream duplicate-effect constraints
are normative in docs/execution-contract.md and task41 migration.

Checks completed: current public probe race count3 PASS; contract race count3
PASS; full root race PASS; human/mail race PASS; human/mail pinned lint0; executable
local challenge -> bound approval -> process restart replay proof PASS, one receipt.
Root final lint0 and final contract race count3 PASS. Independent A and B each accepted all five criteria at100%, no unresolved detected errors; see acceptance-a.md and acceptance-b.md. A also verified exact/+1 display and encoded-result bounds including the default1MiB cap; retained fixture acceptance-a-bounds.go.txt, race count3 PASS. Root full race predates test literal extraction/formatting and added positive clock-callback assertions only; no production change after that run. Final targeted contract race count3 and lint cover the latest test source.

Limits: local fixtures, no live external authorization/downstream conformance.
Fenced host reconciliation is trusted evidence supplied by host; test simulations
are not proof of external idempotency. No production publication.
