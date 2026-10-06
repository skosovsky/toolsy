# D25 verification

Baseline HEAD: `7331736ac925527fe5efe2711eb59c292a172ffc`.
Original review baseline: `58085005dc7e4a0747b2a57af2ef2f62ba2c3750`.

The retained `default-probe.go.txt` fixture was installed as
`ext/toolsyotel/vendor_mapping_test.go` before production edits. Command:
`GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go -C ext/toolsyotel test -run TestD25DefaultHasNoVendorAttributes -count=1`.
It failed behaviorally on the default `langfuse.observation.type`; fatal assertion
stopped the first (capture=false) case. SHA256: `65d6a01dd0b45f6252725321598769cb77ba9e92afeff64530c071703b181a82`.
Current same regression plus vendor/capture/error/panic and split-secret cases pass.

Final affected module:
- `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go -C ext/toolsyotel test -race -count=3 ./...`: PASS 1.564s.
- `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache GOLANGCI_LINT_CACHE=/tmp/toolsy-task41/d25-parent-lint /opt/homebrew/bin/golangci-lint run --allow-parallel-runners ./...` from module: 0 issues, pinned v2.14.0.
- `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go -C ext/toolsyotel test -run '^$' -bench '^BenchmarkTracingMapping$' -benchmem -count=3`: PASS. No-op provider, one text chunk, fixed input, M1 Max darwin/arm64. Default metadata 350–409ns/592B/9alloc; vendor metadata 343–348ns/592B/9alloc; capture 1614–1651ns/5736B/15alloc; capture+vendor 1935–1939ns/6504B/17alloc. Local microbenchmark, no baseline speedup or exporter throughput claim.

Split-secret fixture deliberately demonstrates per-chunk matching failure and
whole-chunk omission success. No whole-stream sanitizer has been introduced.
Field caps bound exported display strings, not raw input preparation/redactor cost.
No external exporter or live Langfuse acceptance was exercised. Sources:
[OTel attributes](https://opentelemetry.io/docs/specs/semconv/registry/attributes/gen-ai/),
[Langfuse mapping](https://langfuse.com/integrations/native/opentelemetry).

Two independent acceptance reports pending.

## Acceptance correction

Initial A found invalid short UTF8 returned unchanged by truncatePayload; retained
initial report records 90%/notaccepted. Production now normalizes invalid byte
sequences to U+FFFD before measuring/truncating, preserving the field cap even when
replacement expands bytes. Added vendor/raw payload regression coverage; original
return/panic unchanged. Corrected fullmodule race count3PASS1.936s/lint0 and
benchmark count3PASS16.501s. Corrected benchmark allocations unchanged9/15/17;
local timings variable, no performance improvement claim. Corrected logs supersede
initial final logs. Both independent final acceptances pending.

## Final acceptance

Independent A and B each100% (five20/20), no unresolved detectederrors. Exact former
invalidUTF8 probe and extra tiny budgets pass. Both affected fullmodule race,
pinnedlint0, adversarial probes and benchmark pass. Final reports and artifacts
retained in acceptance-a/ and acceptance-b/; initial A rejection retained separately.
