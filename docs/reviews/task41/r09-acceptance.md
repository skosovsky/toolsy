# R09 / D07 / D08 acceptance

Scope: codec schema lifecycle, state-plus-binding checkpoint scope, and BYOT
ownership. Previous commit: `df733dd`; original review baseline: `58085005`.
Production publication/push was not used.

## Contract and implementation

`StateCodecRegistry` is a build-then-freeze registry. NewSession first validates
RunPolicy and registry/manifests, then freezes the supplied shared registry and
computes its digest. Freeze is explicit, idempotent and nil-safe; registration
linearizes before it or fails with inspectable ErrStateCodecRegistryFrozen.
Typed, JSON and prototype registration all enforce this. A different schema
requires a new registry. Host slot options execute outside the registration lock.
Failed constructor validation leaves a mutable builder; failed checkpoint restore
still finalizes the builder because NewSession has already succeeded.

Required slots must be present on export/import. Untyped nil is omitted and cannot
satisfy a required slot. Registered non-nullable codecs cannot export JSON null;
nullable codecs can roundtrip typed nil. A null payload cannot bypass required
validation. Slot metadata is frozen; custom codec correctness, roundtrip semantics,
schema IDs and concurrent callback state remain host contracts.

D07: checkpoints persist state plus binding. Restore uses current host RunPolicy
and limits with fresh counters; it excludes dependencies, StateStore contents,
external effects and workflow continuation. Durable authority/budgets remain host
responsibilities. This is documented and tested with different restore authority
and exhausted original/fresh restored counters.

D08: Set/Get preserve aliases of pointers/maps/slices. The host synchronizes those
values. Map-slot capture and replacement are synchronized; codecs and MarshalJSON
execute outside state/configuration locks. Hydration failure does not replace the
map, but host callback writes/effects are not rolled back. No universal deep copy
or transactional callback guarantee is claimed.

## Regressions and verification

- Parent `GOCACHE=/tmp/toolsy-review-gocache make test`: PASS, all 24 modules
  running `go test -v -race ./...`; full raw log retained.
- Parent full root `go test -race ./...`: PASS; root 12.332s, toolsygen 46.458s.
- Final targeted lifecycle suite `-race -count=5`: PASS, 4.025s.
- Root pinned golangci-lint v2.14.0: 0 issues; diff whitespace check passes.
- Behavioral probe against `df733dd`: FAIL as expected. Both optional and required
  late registrations return nil; export succeeds, restoration fails with state
  schema digest mismatch. The same probe on current code: PASS; registration
  returns ErrStateCodecRegistryFrozen and restoration succeeds.
- Independent A overlay probes and targeted suite, race count5: PASS. 30 rounds
  of 10 writers x20 required registrations verify concurrent construction;
  callback Freeze/registration/SetState/Rebind, failed hydration writes, failed
  checkpoint construction and null payload validation pass.
- Independent B external public probe, race count5: PASS, 2.244s. Concurrent
  registration/freezing, JSON persistence/restore, callback reentry, changed schema
  rejection and failed restore finalization pass. Latest targeted count10: PASS,
  4.813s.

Reviewer A (`r09_acceptance_a`): **100%, accepted**, all five 20% criteria complete,
no unresolved detected defects. Reviewer B (`r09_acceptance_b`): **100%, accepted**,
all five 20% criteria complete, no unresolved detected defects. Neither implemented
production code or viewed the other review. Both caught formatting/test assertion
issues; these were fixed and independently rechecked. ToolError's public message
omits host cause, which remains inspectable through Unwrap.

## Performance and limits

Existing Execute benchmark retains 200 allocations; snapshot without codecs
retains 108 allocations. The short existing samples ran under concurrent checks,
so their wall-clock differences do not establish a speedup.

A dedicated 32-required-codec snapshot benchmark ran baseline then current, three
200ms samples each. Baseline: 15.7–17.7us, 6744B, 108 allocations. Current:
17.7–18.4us, about 9523B, 112 allocations. Checking required slots copies their
policy map, adding roughly 2.8KiB and four allocations per export. This is an
explicit correctness cost; these short runs under shared host load are not a
statistical latency guarantee. Source and raw logs are retained below.

Evidence: [logs and exact probe sources](r09/). Overlay sources use only previous
public APIs where needed for baseline reproduction. Custom codec roundtrip and
BYOT synchronization remain host responsibilities; acceptance does not certify
arbitrary host callbacks or universal bug freedom.
