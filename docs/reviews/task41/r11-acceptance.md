# R11 / D16 acceptance

Scope: Empty delivery envelopes, result algebra, schema mapping and BYOT clone
limits. Previous commit: `4c36832`. No production push/publication.

Empty and Noop always build envelopes, preserving typed Value, audience, class,
metadata and controls without serializing Value into wire bytes. Empty permits
effects; Noop forbids declared effects. Raw overrides wire encoding while retaining
typed Value. Contradictory flags/wire/effects and stray MIME fail with nonretryable
INTERNAL ResultContractError, phase result_algebra. Generic and replay chunks
enforce the same invariants. Available typed values remain decodable.

Nested RawMessage output defaults to any valid JSON; arguments keep their object
default. Explicit host mappings/output schemas take precedence. Defaults are local
to each schema build and do not mutate the shared registry. Top-level custom
encoders remain uninferred. Reflective cloning covers exported data and cycles
within one cloned value; opaque state, map keys and cross-component/subslice alias
limits remain documented host responsibilities.

The integration gate exposed MCP empty-wire projections carrying complete typed
protocol results. The final contract preserves those values. Earlier reviews of
the discarded value-free contract are superseded; both reviewers repeated their
acceptance against the retained-Value implementation.

Verification:

- Parent all 24 modules: `make test`, go test -v -race ./..., PASS.
- Parent targeted race count 5: PASS 2.356s; full MCP race: PASS 19.268s.
- Root pinned golangci-lint 2.14.0: zero issues; diff whitespace clean.
- Identical public probes fail on the previous commit for lost delivery metadata
  and rejected nested RawMessage boolean; final candidate passes both.
- A independent revised adversarial race count 10: PASS 2.858s; full root and MCP
  race PASS, MCP 27.373s; lint zero issues. Typed replay, hostile persisted
  declarations, nested raw collections and opaque unencodable Value verified.
- B independent eight public API probes race count 5: PASS 2.910s; root full race
  PASS, MCP full race PASS 27.381s; lint zero issues. Panic-on-MarshalJSON confirms
  absent serialization; typed decoding/replay and concurrent schema mappings pass.

Both `r11_acceptance_a` and `r11_acceptance_b`: **100%, accepted**, five criteria
at 20/20 each, no unresolved detected defects. Neither implemented the production
change nor read the other verdict.

Three short benchmark samples: old Empty 47 allocations/~4082B loses metadata;
final Empty 79 allocations/~5812B preserves it. Old Noop 130 allocations/~9780B;
final Noop 79 allocations/~5812B avoids fake wire serialization. Shared-load timing
is not a statistical latency guarantee. Arbitrary non-JSON Value still requires
a suitable trusted host persistence codec; standard JSONResultCodec rejects it.
Migration covers incompatible old wire-bearing records without authorizing
eviction of durable completed operations or blind redispatch.

Logs and probe sources: [r11/](r11/). B's external probe used GOSUMDB=off for
sandbox global-cache write restrictions; ordinary root/MCP checks used standard
verification. Scoped acceptance does not guarantee universal bug absence, host
codec correctness, arbitrary callback effects or opaque object synchronization.
