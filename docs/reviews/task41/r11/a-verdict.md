# Independent R11 / D16 acceptance A

Compared the current candidate with 4c36832; read original R11 and D16 plus the five ledger criteria. No implementation edits, commit, publication or second-reviewer verdict consulted.

1. Empty envelope and delivery preservation: 20/20. The early return is removed; both statuses construct an envelope without serializing Value. Direct/registry/RunCall/cache tests preserve audience, class, metadata, controls, effects where allowed and status. Non-JSON ignored Value confirms payload absence.
2. Algebra and precedence: 20/20. Typed and generic successful chunks reject both flags, flagged wire/typed/envelope payload and Noop effects. Raw retains typed Value while overriding wire; stray Raw MIME rejected. Own cache probe verifies wire 2 and typed/envelope 1 through fresh and replay delivery. Independent hostile persisted-record tests reject every flagged contradiction with inspectable nonretryable INTERNAL/result_algebra. Error/business chunks intentionally have their separate contract.
3. Exact schemas: 20/20. Exact primitive/array/nested RawMessage fixtures and shape matrix audited. Defaults are applied to local cloned mapping; explicit registry and output schema precedence verified. Own nested []RawMessage and map[string]RawMessage output works for null/array/boolean. Top-level custom marshaler/RawMessage inference remains absent and explicit schema is enforced. Input object default retained.
4. BYOT ownership limits: 20/20. Documentation accurately explains exported value cloning, cycles, opaque/private state, functions/channels, pointer map keys, differently sized overlapping slices and independent components. Inspected reflective implementation and ownership fixture; no arbitrary JSON clone introduced.
5. Verification and migration: 20/20. Own race overlay and row regressions repeated 10 times PASS (3.297s). Full root go test -race ./... PASS (core 4.632s; unchanged packages cached). Own root golangci-lint 2.14.0 PASS, git diff --check PASS. Read baseline behavioral FAIL and current PASS for lost envelope and rejected nested boolean RawMessage. Migration covers Noop payload break and deliberate migration of persistent completed-operation records without authorizing redispatch.

Accepted: 100%. No unresolved detected defect. This percentage measures the five scoped criteria, not universal bug absence. Own runs cover root module and subpackages, not a fresh all-24-module run (final gate remains). No live publication occurred. Host custom codec correctness, opaque-object immutability and arbitrary callback effects remain documented host responsibilities.

Evidence: probe_test.go, overlay.json, race.log, full-race.log, lint.log in this directory.

## Final repeat after error-status guard

Re-audited latest guard: result algebra runs before structured error validation; IsError plus EmptyResult or Noop is rejected as contradictory success/error declaration. Ordinary error results remain valid. New error-empty/error-noop regressions inspected and included in independent race ×10 repeat PASS. Final independent root lint reports 0 issues; diff whitespace clean. All five criterion scores remain 20/20; accepted 100%, no unresolved detected defects. Prior full root race evidence precedes this small guard, while the latest targeted repeat covers the changed path.

## Superseding final acceptance: typed Value retained on Empty / Noop

Earlier verdicts above applied to the superseded payload-free typed-value contract and are not the current acceptance. Re-read latest five criteria and source R11/D16; this contract omits serialization/wire bytes, preserving host BYOT data, consistent with supported MCP empty wire projections.

1. 20/20: always construct envelope; preserve typed Value, audience/class/metadata, controls, permitted effects and statuses through direct/registry/RunCall/cache. Latest fixtures pass.
2. 20/20: exclusive statuses, no wire, Noop no effects, Raw conflicts, stray MIME and error-success statuses rejected. Typed and envelope values without wire are valid. Own revised probe roundtrips integer typed/envelope payload with both statuses through JSON codec. Own invalid persisted-record probes still reject flags/wire/effects; Raw wire/typed precedence and cache replay still pass. Inspectable nonretryable INTERNAL result_algebra preserved.
3. 20/20: exact output schema shape fixtures/matrix, nested RawMessage collections, argument object default, custom mappings/local schema map and explicit output precedence unchanged and independently repeated.
4. 20/20: reflective clone limits remain honest; arbitrary opaque Value survives Empty without serialization. Own function-valued map direct succeeds and JSON codec rejects this non-JSON host value. Complete custom codec remains host responsibility.
5. 20/20: latest independent race overlay ×10 PASS 2.858s, root full race PASS (cached on this repeat), full MCP race PASS 27.373s, root lint 0 issues, diff check clean. Inspected actual MCP client EmptyResult/TypedResult/envelope combination at tool/resource projections; it remains supported. Baseline defects and current behavioral PASS evidence remain relevant. Latest migration accurately retains typed Value and rejects only incompatible old wire records without allowing blind operation redispatch.

Current final verdict: ACCEPTED 100%, no unresolved detected defects. Scope limits unchanged: no independent all-24-module repeat, no production publish, host codec/opaque ownership remain explicit. Latest evidence uses typed-retention-*.log and revised probe_test.go.
