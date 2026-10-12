# Reviewer B — final adversarial acceptance

Date: 2026-10-12. **APPROVE**. Found defects: **2**, fixed: **2**, unfixed: **0**.
Implementation was not edited by reviewer B. Earlier approvals were withdrawn when
B1/B2 were reproduced and are not transferred to this candidate.

Final inventory:
`c6747b79cc3f0d3565b791c528d4d34a656a89a91e6bd7eeb9284fe1b77e2e88`,
**543 files**. HEAD `2b6f6ac928da1d1f5af26f43503e3f7f541a6a87` is the starting
commit; reviewed implementation is uncommitted. All files were independently
verified before tests in `/tmp/toolsy-reviewer-b-final-c674`. After tests, complete
inventories of that isolated copy and the original workspace were independently
rebuilt: file sets, every file hash and aggregate hash match exactly.
`reviewer-b-identity.json` and `reviewer-b-result.json` record the identity/results.
Temporary probes were removed before the unmodified final snapshot's full root suite.

Measurements use inventory
`9b0cd39f97838a8636f88086768c8fe9d259b5e98b5c45fcd4792dfbd44713bf`.
Reviewer B independently confirms the sole final difference: an equivalent
append assignment in `performance_permissions_test.go` to satisfy gocritic.
Every production, benchmark and fixture file is byte-identical. The exact
one-file bridge is recorded in `reviewer-b-measured-final-diff.json`.
Final independent probes/root race below execute c674, not the measured snapshot.

## B1 — P1, FIXED: defined pointer erasure caused source aliasing

For `type Named *Node`, the previous pointer branch allocated an unnamed `*Node`.
Nonnil roots/nested interfaces lost dynamic type; `cloneTypedArgValue[Named]`
failed its type assertion and returned the source. Mutating the supposed snapshot
mutated the source graph. This defect existed in official v0.20.1 and survived the
first memo optimization. It violated the explicit type/ownership contract and
was not a new memo regression.

Fresh pointer conversion to `v.Type()` now precedes memo insertion/recursion.
Nil handling and type-sensitive keys remain unchanged. Independent probes verify
roots, nested interfaces, typed nil, typed struct fields, generic isolation,
arrays, cycles and aliases across inline migration/overflow. Three pointer types
viewing one source address keep separate memo identities. Root race also runs
actual prepared-context/type-assertion regression coverage.

## B2 — P1, FIXED: empty permissions shared and retained source backing

The previous `cloneRequirements` copied only nonempty Permissions. A snapshot of
`backing[:0]` shared capacity and backing; append changed the source array. A
retained zero-length snapshot held an arbitrary-size backing array and its payloads.
Even `backing[:0:0]` retained the source pointer. This defect also existed in
v0.20.1 and violated policy snapshot isolation/retention, rather than being a memo
regression.

Make+copy now handles every nonnil slice. Nil stays nil, nonnil empty stays nonnil
with zero capacity and no source backing, and nonempty copies have capacity equal
to visible length. Independent positive probes cover 65,536-element backing,
zero-capacity source, append mutation, nil/populated behavior and an actual public
NewRequirementsPolicy callback retaining both request snapshots after return.
Separate callback snapshot domains no longer share source backing.

Identical B1/B2 probes were independently rerun against official v0.20.1, origin
`2b4c9419792bc843ff92c0cf64b35f83330a389d`: **FAIL (expected negative oracle)**,
including source mutation, lost named types, zero-capacity backing retention and
retained public callback aliasing. Raw: `reviewer-b-final-baseline-negative.log`.
The corrected final candidate passes every case. **B1 and B2 are FIXED.**

## Static memo and compatibility review

Reference keys `(type, pointer, slice length)`, pre-recursion insertion, JSON/BYOT
dispatch, output allocation, generic nil/empty distinction, exported structs/arrays,
map keys and opaque private values preserve their contracts. The two narrow fixes
restore documented type/ownership guarantees. Established manifest empty-schema
and Tags normalization is distinct from generic/Permissions nil-empty preservation.
Validator, registry, middleware, replay, codec and result preparation production
implementations are unchanged.

Each operation owns eight inline entries. Its ninth distinct insertion acquires
exclusive fixed scratch, migrates existing keys/output references, then clears
inline entries. A channel receive removes an idle lease, preventing concurrent/
nested operations from sharing active scratch. Deferred release covers normal
return and reflection panic unwinding without replacing panic values.

The table has 512 slots and at most 384 occupied slots, guaranteeing an empty slot
for probing; overflow remains searchable. Full-key equality includes type; uint16
indices cover all slots. Release zeros occupied keys/values/indices, clears inline
entries, drops overflow backing, unlinks the lease and offers fixed scratch to a
capacity-four channel. Output objects never enter that channel. Oversized graph
capacity cannot become idle scratch. No additional static defect found.

Adjacent framework-owned policy request helpers were also inspected for the B2
pattern: Manifest Tags/schemas, RegistryView slices and ToolInput/attachments copy
or normalize empty into a fresh output. CallContext BYOT references retain their
explicit immutable/synchronized host contract; universal deep copying is not claimed.

## Independent final execution

Commands ran from isolated copies with:

```sh
GOPATH=/tmp/toolsy-performance-gopath GOCACHE=/tmp/toolsy-performance-gocache GOWORK=off
```

Nine independent AAA probes plus the two shipped permissions regression tests
**PASS with race** (1.890 s):
`go test -race -count=1 -v -run '^TestReviewerB|^TestMemoryPermissions' .`.
Raw: `reviewer-b-final-probes.log`. Sources:
`reviewer-b-probe.go.txt`, `reviewer-b-named-pointer-probe.go.txt`,
`reviewer-b-permissions-probe.go.txt`.

- A 900-pointer mixed JSON/named-BYOT graph checks arrays, private fields, root
  cycles, aliases, bidirectional mutation and retained snapshots across later clones.
- 784 deliberately colliding memo keys check full probing chains/overflow;
  panic identity and fixed/overflow cleanup are verified.
- Twenty-four workers × twenty oversized cyclic clones run with GC disabled;
  source/output isolation, idle count and all idle reference slots are checked.
- Same-backing maps through different named types retain type-sensitive identity;
  early aliases survive inline migration; inline references are cleared.
- Two B1 probes cover named roots/nested/nil/generic ownership and three pointer
  identity domains across 1,100 additional maps plus typed arrays/cycles/aliases.
- Three B2 probes cover empty/nil/populated permissions, zero-capacity source
  backing release and a real retained public requirements-policy callback.

After removing temporary probes, the unmodified final c674 snapshot independently
passes `go test -race -count=1 ./...`. Raw: `reviewer-b-final-root-race.log`.
This covers root and root-owned subpackages/examples, original compatibility tests
and new retained policy requests, weak owner/registry release, reflection panic,
nested/concurrent leases, oversized/churn and between-GC clearing. Existing private
fields/map keys, malformed envelopes, custom Tool/middleware/profile/replay/codec,
sticky rejection, cancellation and accounting tests remain intact and pass.

Optional-module штатные checks are parent execution evidence and A's completeness
review; reviewer B does not claim independently running them. Initial/superseded
results are retained separately and do not stand in for final execution.

## Bound and limits

Four idle memo payloads total 4 × 29,456 bytes. On the tested allocator, four memo
blocks plus channel allocate 131,216 bytes, below the conservative 129 KiB bound.
GC-disabled adversarial probes and structural clearing establish the reachable
idle-scratch bound without forced GC. Dropped overflow/extra tables may remain
unreachable garbage until collection; the bound is not a total HeapAlloc/RSS limit.
Active operations and output graphs have separate peak costs. No global mutable
schema-address cache or output pool is introduced.

Exact profiles separate bounded scratch from unchanged startup validator ownership;
nested cumulative values are not summed. Finite natural CPU/STW windows are noisy;
no uniform GC improvement, long-term leak proof or production heap budget is claimed.
A separately assesses the full mandatory performance/measurement checklist.

This approval establishes absence of found unfixed defects for this candidate,
not mathematical absence of errors. Private fields/map keys/opaque values retain
host-owned limits. Consumer allocation/latency gates and published adoption remain
separate from successful diagnostic byte targets. Further source changes invalidate
this exact identity and require refreshed acceptance.
