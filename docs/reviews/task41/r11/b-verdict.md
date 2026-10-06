# R11 / D16 independent acceptance B — final retained-Value contract

This supersedes the earlier review of the discarded typed-value-free design. Reviewed current diff versus 4c36832, task R11/D16, revised row 11 five criteria, implementation, exact schema fixtures, MCP empty-resource projection and current README/migration/execution documentation. Did not implement or inspect another reviewer's verdict.

| Criterion | Completeness | Finding |
|---|---|---|
| 1. Empty envelope and direct/registry/RunCall/cache semantics | 20/20 (100%) | Typed Value and envelope.Result survive while wire bytes/MIME are absent. Audience/class/metadata/effects/controls/status survive all defined paths. Public independent RunCall probes verify empty_success/noop_success, retained typed decoding, and validator behavior. Panic-on-MarshalJSON BYOT confirms Empty/Noop never serialize Value. |
| 2. Result algebra, Raw override, generic/replay validation and error classification | 20/20 (100%) | Empty/Noop exclusive, no wire bytes, Noop no declared effects, Raw/status and stray MIME rejected. Raw retains typed Value while explicit JSON schema validates wire. Host typed-only Empty/Noop replay accepted. Hostile persisted wire/effect/status cases reject INTERNAL result_algebra without dispatch. Proxy ignoring yield errors cannot suppress structured error/success-status contradiction rejection. MCP's typed empty wire projection passes current core contract. |
| 3. Exact schemas and explicit mapping priority | 20/20 (100%) | Exact fixture audit/all JSON shapes covered; defaults stay in per-build maps. Independent concurrent shared registry builders with RawMessage slices/pointers pass and subsequent explicit mapping wins. Public custom marshaler explicit boolean succeeds, object mismatch fails. Top-level custom shapes deliberately uninferred. |
| 4. BYOT cloning limits | 20/20 (100%) | Implementation matches exported-data/cycle clone and documented opaque/channel/function/map-key ownership plus cross-component/subslice alias limitations. Borrowed channel retention independently verified. No universal serializer/JSON roundtrip added. JSONResultCodec explicitly requires JSON-shaped values; arbitrary unencodable Value needs a trusted host codec for persistence even though no-wire direct execution does not serialize it. |
| 5. Regression, baseline, race/lint, migration | 20/20 (100%) | Baseline behavioral failure/current correction evidence inspected. Final independent root full race and MCP full race pass, root lint clean, revised external probes race count 5 pass. Migration documents no-wire Noop break, typed retention and deliberate persisted-wire record reconciliation. |

Total: 100%. Accepted. No unresolved detected defects.

Final independent checks:
- Revised external 8 public API adversarial tests under race count 5: PASS 2.910s, including 600 concurrent shared-registry builds.
- Full root module `go test -race ./...`: PASS, root 3.365s, toolsygen 21.071s.
- Full MCP module `go test -race ./...`: PASS 27.381s, including TestMigratedEmptyResourceChunksRespectCoreContract with typed ResourcesReadResult retained.
- Root golangci-lint 2.14.0: 0 issues.
- `git diff --check`: PASS.

Evidence in this directory: probe_test.go, probe.log, full-race.log, mcp-race.log, lint.log. race.log records the superseded earlier targeted review, not the final design's only verification. External standalone probe needed GOSUMDB=off because sandbox prevents global sumdb/latest writes; ordinary root/MCP checks retained standard module verification and existing go.sum. No production publish or commit performed. This covers these five criteria; does not claim universal bug freedom or all 24 nested module verification. Host codec roundtrip, custom nested marshaler shape, opaque ownership and external effects remain host contracts.
