# R12 / D13 / D15 acceptance

Scope: business-error cache delivery, explicit reuse eligibility/freshness and
neutral replay provenance. Previous commit: `a492d5f`. No push/publication.

Eligible cache misses forward unsuccessful terminals unchanged without Put.
Missing/multiple terminals and infrastructure/configuration/codec/limit errors
are INTERNAL, nonretryable and not input-correctable; causes remain inspectable.
Successful results still persist before delivery. Producer/consumer/control errors
cannot be swallowed to persist success. Context checks after host callbacks stop
further dispatch/delivery once cancellation is observed. A completed Put may have
persisted despite cancellation; cancellation is not an effect rollback guarantee.

Clear break: NewResultCache requires a per-attempt CacheEligibility predicate,
before partition/storage and after current authorization. False bypasses cache
work; Idempotent/ReadOnly hints do not authorize reuse. The host binds revisions,
identity and dependency freshness and owns expiry/eviction. Concurrent misses are
not deduplicated. Prepared callback snapshots are detached; host ports/closures
and opaque values remain responsible for their own concurrency safety.

Clear break: ReplaySourceMetadata carries result_cache or completed_operation.
Both paths use shared bounded replay decoding without a fabricated cache object.
Current correlation is rebound, actual source overwrites stored provenance,
reserved policy overlays cannot erase it or broaden private replay audiences.
Reducers skip applying effects for either source. This is trusted profile
provenance, not authorization or authentication of arbitrary tool output.

Verification:

- Identical public business probe fails on baseline (zero chunks,
  VALIDATION_FAILED) and passes on current code (one error terminal, nil).
- Parent final all 24 module race suites PASS; cache/operation/policy targeted
  race count 5 PASS 2.378s. Root/MCP/filejournal pinned lint: zero issues.
- A independent public adversarial probes race count 10 PASS 1.605s; full root,
  MCP (20.671s), historycodec and filejournal race PASS; root lint zero issues.
- B independent public adversarial probes race count 5 PASS 1.577s; root and MCP
  (19.803s) race PASS; root/MCP lint zero issues.
- Both independently found callback cancellation gaps, now closed and repeatedly
  verified for partition/Get miss/hit/decode/encode/Put. Initial failure logs are
  retained separately; final logs supersede them. Whitespace checks pass.

Both reviewers accepted **100%**, five criteria at 20/20 each, no unresolved
detected defects. Neither implemented production changes nor read the other's
verdict. Tests additionally cover large business failure terminals, dynamic reuse
revocation, source overwrite, nested privacy/effect reducers and abort identity.

Three short 200ms benchmark samples with a 1KiB attachment: bypass changes from
4B/1 allocation to 1088B/4 allocations; hit from ~9964B/92 allocations to
~11064B/96 allocations. The additional eligibility snapshot copies the attachment
and adds callback isolation overhead. These fixtures are not universal throughput
or statistical latency guarantees.

Evidence: [r12/](r12/). Scoped acceptance does not prove host freshness/codec
correctness, cancellation of noncooperative callbacks, external durable-store
fault behavior or universal bug absence. Cancellation can race with an operation
already beginning. Migration retains completed-operation records and requires
fenced reconciliation rather than eviction to force redispatch.
