# Policy clone ownership and measurement contract

Frozen before optimization, 2026-10-12. HEAD 2b6f6ac928da1d1f5af26f43503e3f7f541a6a87; clean tree.
Baseline is official v0.20.1 (origin 2b4c9419792bc843ff92c0cf64b35f83330a389d).

Each clone operation owns an exclusive scratch lease, acquired lazily for reference
values and released by defer, including panic unwinding. Nested and concurrent
operations cannot share an active lease. No output objects are pooled. Callback
snapshots may outlive their call and remain mutable and isolated.

Memo keys and insertion before recursion remain unchanged: type, pointer, slice
length. Aliases/cycles include mixed JSON/BYOT; separate clone operations have
separate identity domains. Named/dynamic types, typed nil, nil/empty, pointers,
arrays, exported structs and slice cap=len remain unchanged. Map keys, private
fields, functions/channels and other opaque values remain host owned.

Proposed scratch bound: at most FOUR idle instances, each with 512 fixed entries
(key + reflect.Value), 384 uint16 occupied indices, a count and an overflow map
header. At most 384 entries use the fixed table (75% occupancy). Remaining entries
use an operation-local overflow map. Release zeros every occupied entry/index,
sets count to zero and drops the overflow map entirely (including backing storage).
Only fixed storage is returned; no graph-dependent capacity survives. A full idle
store drops the released instance. Pool is a bounded channel, not sync.Pool;
forced GC is not required to satisfy the bound. On 64-bit Go, struct size and channel
storage are measured in the tests/report. Maximum retained scratch = four struct
sizes plus channel header/four pointer slots; allocator rounding reported separately.
Peak active scratch/output/overflow is proportional to simultaneous active operations
and graph size and is NOT included in the idle bound.

No address cache, API changes, consumer pins/source, versions or validator changes.
Repeated registry prepareChunk remains a required boundary after arbitrary Tool,
middleware and profiles; investigate separately and retain unless measured evidence
and private ownership permit removal.

Fixed library fixtures: existing testdata/performance/zalo_list_groups input/output
and groups.payload, plus existing BYOT/alias/cycle/tiny clone fixtures. Fixture setup
outside hot timers. New policy manifest benchmark uses BOTH existing rich schemas.
Full library execution uses existing proxy/typed/dynamic registry/view/session policy
matrix and pre-dispatch fixture. Cold constructor measured separately. Five independent
process repeats per version, GOMAXPROCS=2 GOGC=100 GOMEMLIMIT=off GOWORK=off,
Go 1.27.1 darwin/arm64, -benchmem -benchtime=300ms. Raw measurements and all
medians/ranges saved, with longer confirmation if a timing regression is uncertain.

Retained experiment: same live registry and fixed payload, warmup, natural windows,
forced GC only outside windows; sequential and concurrent unique graph churn,
large then small graphs, repeated registry create/release, retained snapshots.
Record HeapAlloc after GC, sampled heap peak, natural NumGC/PauseTotalNs and runtime
GC/assist CPU metrics independently; RSS not measured. No production inference.

Measured stage-one adjustment: the five-process matrix exposed tiny-clone timing
+10.25% despite rich byte savings. Final candidate adds 8 inline memo entries per
operation (stack owned, never retained) and acquires fixed scratch only on the ninth
reference. Migration preserves insertion order/domain and clears inline references.
This is an implementation refinement within the original exclusive ownership/bounded
scratch contract. Stage-one raw data remains under stage1-pooled/; final data is separate.

Adversarial review found B1 in both official baseline and the first candidate:
reflect.New(Elem) erases a defined pointer type on nonnil root/interface values.
cloneTypedArgValue[T] then fails its type assertion and returns the source pointer,
violating snapshot isolation. Contract repair converts the fresh allocation to the
original pointer type before memo insertion/recursion; nil, type-key domains and
host-owned NewCallContext borrowing remain unchanged. New AAA tests distinguish
root/nested named types, aliases/cycles/type domains/nil, generic fallback and
prepared context isolation. B1 is not attributed to the memo optimization.

B2: an empty nonnil Permissions slice with positive cap previously shared and retained
its entire source backing. Any nonnil permissions now get make(len)+copy; nil remains
nil, empty remains nonnil with cap=0, and populated copies are independent. slices.Clone/
Clip cannot establish source release for a zero-length slice and are not used.

Nil/empty classification is per established helper contract: generic valueCloner
preserves nil vs nonnil empty maps/slices and clone cap=len. deepCloneMap(empty), used
for manifest schemas, intentionally returns nil even for a nonnil empty map. Manifest
Tags also retain existing empty→nil normalization through append(nil,tags...). These
normalizations detach/retain no source backing and are not changed by this task.
Requirements.Permissions retains its existing nil vs nonnil empty distinction while
repairing source ownership/capacity; no blanket claim that every manifest field keeps
nonnil empty representation is made.
