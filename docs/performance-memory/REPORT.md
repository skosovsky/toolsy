# Policy cloning and retained memory follow-up

Status: **DONE** — all mandatory checks PASS; A APPROVE 53/53 (100%); B APPROVE, 2 found / 2 fixed / 0 unfixed.

Date: 2026-10-12. This task continues v0.20.1; it does not reopen the completed task or publish a release.

## Candidate and ownership

Initial/final HEAD: `2b6f6ac928da1d1f5af26f43503e3f7f541a6a87`, initial tree clean.
Final source inventory: `c6747b79cc3f0d3565b791c528d4d34a656a89a91e6bd7eeb9284fe1b77e2e88` (543 files;
full per-file hashes and tracked Go diff hash in `candidate-identity.json`). Uncommitted implementation;
HEAD is not a claim that the changes are committed. Measurements were built from inventory `9b0cd39f97838a8636f88086768c8fe9d259b5e98b5c45fcd4792dfbd44713bf`, saved in `measured-candidate-identity.json`. The sole final difference is an equivalent test append assignment in `performance_permissions_test.go` to satisfy gocritic; production, benchmark and fixture hashes are identical. Both reviewers independently verified this bridge; final checks run on c674.  Baseline official root/MCP v0.20.1 origin:
`2b4c9419792bc843ff92c0cf64b35f83330a389d`, verified Go module metadata and checksums.
Current workspace MCP source matches official implementation; its existing development
`require v0.0.0`/local replace resolves to the respective measured root. No repository module files changed.

Before optimization, `contract.md`, observable AAA tests and fixtures were fixed.
Each clone owns an eight-entry inline memo. On its ninth distinct reference it leases
one fixed open-address table. Type/pointer/length identity, insert-before-recursion,
cycles and aliases across JSON/BYOT remain intact. Lease migration does not create a
new identity domain. Existing nil/empty, named/dynamic types, structs/arrays/pointers,
private fields/map keys/opaque values and overlapping-slice contracts remain unchanged.

The fixed table has 512 slots, at most 384 occupied entries, and bounded occupied-slot
indices. Overflow uses a temporary map. Deferred cleanup zeroes every occupied entry,
inline entry and index; it drops the entire overflow map, including backing capacity.
Panic behavior is preserved. Output snapshots are never pooled. Scratch is leased
exclusively; concurrent/nested operations cannot reuse an active instance.

At most four idle tables survive. Struct payload is 29,456 bytes each; allocator class
is 32,768 bytes on this toolchain. Four tables plus buffered channel allocate **131,216 B**
(measured channel/header+four pointer slots: 144 B). Conservative tested bound is
**132,096 B (129 KiB)**, allowing 1 KiB for the channel. No GC or graph-size condition is
needed for this bound. Oversized maps are always dropped; extra concurrent tables are
zeroed then dropped if the idle store is full. Active operation stack (including inline
448-byte payload), active scratch/overflow and output graphs are separate peak costs,
proportional to concurrency and graph size. This is a bounded retained-memory tradeoff,
not zero additional retained heap.

## Library performance

Go 1.27.1 darwin/arm64, GOMAXPROCS=2, GOGC=100, GOMEMLIMIT=off, GOWORK=off.
Five separate processes per version for each of 57 final root/MCP benchmarks.
Fixtures are the original self-contained schema-rich input/output/groups payload;
`fixture-hashes.json`, scripts and raw logs are retained. Setup is outside hot timers;
cold constructors are a separate row. All medians and min/max: `bench-table.md` and
`bench-summary.json` (raw `baseline/candidate-*-bench-*.log`).

| Fixed scenario | baseline ns/op → candidate | B/op baseline → candidate | allocations baseline → candidate |
|---|---:|---:|---:|
| Rich dual-schema policy manifest | 51,835 → 36,090 | 70,928 → 39,016 | 286 → 275 |
| Pre-dispatch real proxy/view/policy | 52,882 → 38,342 | 73,571 → 41,654 | 322 → 311 |
| Full proxy/registry/policy call | 161,304 → 146,015 | 170,213 → 138,292 | 2,390 → 2,379 |
| MCP custom projection | 266,320 → 249,791 | 253,147 → 221,169 | 4,093 → 4,082 |
| Tiny clone | 258.9 → 235.4 | 336 → 336 | 2 → 2 |
| Cold proxy constructor | 1,205,037 → 1,204,735 | 902,895 → 870,512 | 13,443 → 13,431 |

Rich policy bytes ratio **0.55008**, below the unchanged target 0.80. Full execution
keeps the improvement (bytes −18.75%, median time −10%). Composed policy benefits are
included in the full matrix. MCP default has no policy clone in this fixture: 4,207
allocations unchanged; small B/op/timing differences with overlapping ranges
reflect SDK/allocator/host variability, not a new retained cache. No default-path saving claimed.

Existing hot allocation gates remain below their original limits: pre-dispatch <=400;
rich clone improves on official 275; fresh schema/Decode and MCP default do not lose
v0.20.1 savings. No gate or fixture-specific cold ManifestSet exception was relaxed.
Validator/decoder/fresh-delivery improvements from v0.20.1 are not new work here.

Stage-one bounded pooling and final inline refinement have separate raw series under
`stage1-pooled/` and the final matrix. Pooling gave the same rich byte reduction but
made tiny clone timing +10.25%; the inline refinement removes that regression. Rich
byte savings are attributable to memo storage, not output pooling. Additional paired
marginal evidence (tiny 269.1 → 231.0 ns, unchanged 336 B/2 allocations; rich policy 39,016 B/275 allocations in both) is recorded separately; different series are not presented as one run.

Timing/byte blips are retained and classified. Earlier superseded series and their
confirmations remain in pre-pointer-fix/ and pre-permissions-fix/. After B2 an unpaired
reuse of historical baseline timings showed systematic drift even on unchanged
paths, so it was not used for final acceptance. Official fixtures were restored,
and a new contemporaneous five-process paired baseline/candidate matrix was run.
Its only median time ratio above 1.05 is binder +5.03%, while bytes/allocations
improve. Five independent same-fixture 1s pairs give 254,528 → 264,105 ns (+3.76%)
with overlapping ranges (253436–339634 / 255370–281593); no robust timing regression
is established. No faster outlier was selected to hide variability. B/op improves
183,573 → 151,597, allocations 2,400 → 2,388. All other final matrix timing ratios
are <=1.05; tiny SDK/allocator/growing-cache byte variability has overlapping
ranges and unchanged counts, not a new retained cache.

The separate required outer-boundary confirmation uses five independent 1s process
pairs with identical 19,474 B/394 allocations. Full raw ns/B/alloc ranges in
boundary-confirm-summary.json. CPU/GC/latency conclusions remain finite and
host-sensitive; no universal speed guarantee is made.

## Retained heap and GC

`memory-summary.json` includes four independent processes/version, 100 real registry
warmup calls, seven windows per process, 300 iterations/window. Fixed windows have
300 real policy executions each; concurrent-churn has **2,400** executions and 2,400
unique small-graph clones; large-small has 300 executions, ten 3,000-child clones and
300 small clones; registry-churn has 300 constructors/executions; retained-snapshots
keeps 30 additional manifest snapshots alive across 300 executions. This is a finite
millisecond-scale library diagnostic, not a soak test or a production budget.

Forced GC occurs outside natural windows. Registry/tool and deliberately saved snapshots
are explicitly live at every retained checkpoint. HeapAlloc after GC, sampled heap peak,
natural GC count, GC STW, total GC CPU and assist CPU are separate raw fields. The sampled
peak is taken after iterations; it can miss intra-call peaks. Runtime total GC CPU includes
idle marking and is not OS CPU utilization. RSS/off-heap were not measured.

Fixed retained heap is about 669–707 KB, with candidate sequential scratch explaining
about +32 KiB. After concurrency/churn, four retained tables explain approximately +128
KiB; released-owner heaps are about 636–640 KB baseline and 770–772 KB candidate.
Retained snapshots intentionally retain detached output storage (~1.1 MB extra in this
fixture) in both versions. There is no increasing per-call source/output reachability
through scratch: between-GC structural clearing/bounds tests, panic cleanup and weak
original-node/registry release tests pass. Large graphs do not enlarge idle capacity.

Some sampled peaks rise (large-small roughly 5.7 → 6.1 MB). They include active graphs,
transient overflow/garbage and the larger live-scratch GC budget; they are not the idle
bound. Natural GC count drops in fixed/concurrent windows with lower allocation volume;
CPU/STW observations are noisy. The superseded pre-pointer-fix series had a replica-2 first-fixed-window timing/GC outlier; its raw data is retained separately. Final replicas reproduce the fixed retained plateau and lower fixed/concurrent GC counts; CPU estimates remain host-sensitive. No uniform
GC CPU/STW improvement claim, long-term leak proof or production heap cap is made.

Default sampled profiles are too sparse to precisely attribute startup validators.
Separate `*-exact-profile/` runs use `-memprofilerate=1` with the identical live registry/
warmup/forced-GC checkpoint. Their natural-window metrics are profiler-contaminated and
are excluded from timing/GC conclusions. Inuse profiles and constructor/scratch filtered
reports distinguish persistent schema/validator ownership from bounded scratch: compileResultContract
cumulative **63 KiB in both**, NewProxyTool cumulative **64.62 → 96.62 KiB**; the
32 KiB delta is the fixed scratch allocation, also independently isolated by the
remember-focused profile. These nested cumulative figures are never added. Historical
consumer NewProxyTool ~1.97 MB/compileResultContract ~1.81 MB are different cumulative
measurements: do not add nested cumulative values or assign them to the +2.4 MB consumer delta.

## Remaining areas: measured dispositions

- `wrapYieldWithCallMeta` remains unchanged: isolated boundary 19,474 B/394 allocations
  for the large library payload. Historical ~3,480 B/read is a different payload/process.
  Registry receives arbitrary custom Tools/middleware/profile/codec output; a private builtin
  result upstream does not establish immutable ownership after those callbacks. Removing
  this validation would lose malformed-envelope rejection/cancellation/accounting semantics.
  No public alreadyValidated flag or fragile payload identity trust was introduced.
- Startup validators remain registry/tool owned. No global validator/schema-address cache;
  no risky compiler rewrite was needed to meet the byte target. Registry release is tested.
- Metadata-only policy is a separate possible API contract: explicitly declare absence of
  schemas while preserving name/requirements/call context, rather than changing PolicyRequest.
  Current dual-schema copy costs 39,016 B/boundary on this fixture; avoiding it could target
  that area, but the estimate is not an implemented/API benchmark saving. Existing policy
  schemas and public signatures remain intact.

## Isolated consumer diagnostic

18 processes, 10,800 measured calls, 900 warmups, zero errors, cleanup true. Original
SDK in-memory/SQLite fixture; 50 warmup + 3×200 calls, pacing 110ms, concurrency=1 per
server; three scenarios in separate parallel processes. Same runtime settings. Mirrored
old/official/optimized/optimized/official/old sequence; each selected pair is ABBA.
`consumer/manifest.json` records binary hashes/build info, source hashes, fixture hash
and **temporary candidate override** in `/tmp/toolsy-memory-consumer/toolsy`. No consumer
source/pins or original fixture sources changed. Original external manifests and measurement
instrumentation are copied alongside the new raw JSON for audit. This is diagnostic use,
not immutable published adoption.

| Scenario | old B/op | official v0.20.1 B/op | optimized B/op | unchanged 1.20×old target |
|---|---:|---:|---:|---:|
| read | 427,736 | 525,882 | 493,971 | 513,283 PASS |
| search | 461,397 | 578,022 | 543,958 | 553,676 PASS |
| context | 452,203 | 569,919 | 536,995 | 542,644 PASS |

B/op includes client response handling; it is not server-only or production RSS. Current
optimized retained ranges are 7.208–7.243 MB, a bounded sequential-scratch increase over
official. Full consumer heap remains much higher than old. Dynamic timestamps/response
sizes are not asserted byte-identical. `consumer/summary.json` keeps all replica p95 and
allocation counts; the original T09 allocation/latency protocol was not rerun. Historical
allocation deficits 556/767/788 and historical p95 discrepancy remain open; diagnostic
counts cannot close those gates. The current old p95 is itself much slower than historical
old (host/protocol sensitivity); no cause or complete latency resolution is established.

## Checks, independent acceptance and scope

Final `make lint`, `make test`, `make test-integration`, `make test-e2e`: **PASS**, each across all 27
repository modules with their штатные flags, root/optional race included. Final results and
raw logs live in `check-results.json`/`make-*.log`. Initial formatting and linter/probe-file
coordination failures are saved separately; final PASS cannot be inferred from old logs.
Live provider checks: NOT RUN, external credentials/providers separate. `check-coverage.json` explicitly lists packages with no test files/no selected tests; command PASS does not imply coverage there.

Frozen checklist: **53 items** in `checklist.json`. A final completeness verdict is **APPROVE, 53/53 (100%)**, FAIL 0 / mandatory NOT RUN 0; B independent
adversarial verdict is **APPROVE**, found 2 / fixed 2 / unfixed 0, on final c674 inventory. Independent reports: `reviewer-a.md`/`reviewer-a.json`, `reviewer-b.md`/`reviewer-b-result.json`. Both review the same source inventory; no promise of
mathematical absence of errors. Scope contains only toolsy sources/tests/evidence and the
current task handoff. No consumer/neighbor source changes, module versions/dependencies,
public APIs, validation semantics, pins or release actions.

## Reproduction

`python3 docs/performance-memory/setup.py` verifies/downloads official baselines into /tmp.
`bench.py`, `confirm.py`, `measure.py`, `measure-confirm.py`, `boundary-confirm.py`,
`profile.py`, `marginal.py`, `checks.py`, `identity.py`, `summarize.py` preserve exact commands.
Run measurement scripts serially without concurrent heavy checks; go tools use writable
/tmp GOPATH/cache with normal checksum verification. Existing raw benchmark logs are reused
by bench.py; archive/remove them explicitly for a fresh measurement series.

Library fixtures are fully self-contained. Consumer reproduction requires the original
isolated `/tmp/zl-memory-20261012` source/binary snapshots (hashes and origins recorded);
`restore-consumer.py` verifies original archived source hashes and reconstructs/rebuilds isolated fixtures from recorded Git origins and the unchanged checkout before running consumer.py.
Missing external consumer fixtures would be NOT RUN, never a library benchmark substitute.


### Ownership repairs found during adversarial review

B1 (P1, pre-existing in v0.20.1): nonnil defined pointers lost their dynamic
type; generic snapshot fallback could return the source pointer. Fresh pointers
are converted to their original type before memo insertion/recursion. Nil and
type/pointer identity domains are unchanged. Generic and prepared-context isolation
tests distinguish the repair; NewCallContext borrowing is not changed.

B2 (P1, pre-existing): empty Permissions slices retained original backing capacity
and policy append could mutate it. Every nonnil slice now uses make(len)+copy;
nil remains nil and nonnil empty remains nonnil with cap=0. Normalizing schemas
(deepCloneMap(empty)→nil) and Tags(empty→nil) is established helper behavior and
stays unchanged; generic traversal retains its distinct nil/empty contract.

Pre-repair evidence and withdrawn approvals remain in pre-pointer-fix/ and
pre-permissions-fix/. A timing-only unpaired series after B2 is separately stored
under unpaired-permissions-series/ and is superseded by a fresh paired matrix.
Temporary fixture source loss was recovered: official module checksum/origin were
reverified; all original consumer source hashes matched the unchanged source checkout
before rebuilding isolated old/official fixtures. No source/pin adoption occurred.
