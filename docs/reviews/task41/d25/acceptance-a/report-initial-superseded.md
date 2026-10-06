# D25 independent acceptance A

Reviewed uncommitted D25 at HEAD 7331736ac925527fe5efe2711eb59c292a172ffc. Verdict: **not accepted**, total **90%**. No other reviewer verdict read. No repository, git, go.work.sum, production, test or documentation files changed.

| Criterion | Score | Evidence |
|---|---:|---|
| 1. Portable default/canonical metadata/vendor opt-in | 20/20 | Default no langfuse attrs, canonical call.id, explicit flag, no dependency added. |
| 2. Default-off unified bounded/redacted policy/vendor independence | 20/20 | Capture off prevents all content/redactor invocation; input/output vendor fields mirror capped values. Fixed statuses avoid raw text. |
| 3. Delivery/classification/original semantics/UTF8 | 15/20 | 28 adversarial matrix cases pass including rejection, control and abort; invalid UTF8 redactor output violates explicit UTF8 contract. |
| 4. AAA tests/baseline/split-secret/race/lint/benchmark | 20/20 | Retained baseline behavior and matching SHA; independent race count3 and pinned lint pass; benchmarks pass. |
| 5. Honest docs/boundaries/no unresolved defects | 15/20 | Split-secret boundary and host responsibilities prominent; UTF8 guarantee currently false and no-unresolved-errors acceptance gate unmet. |

## Unresolved finding [P2]

`ext/toolsyotel/payload.go:15-16` returns short strings unchanged without validating UTF8. `ext/toolsyotel/options.go:91-94` feeds arbitrary host redactor output directly into this helper. ContentRedactor has no documented valid-UTF8 precondition. A redactor returning `string([]byte{0xff})` with max size7 produces invalid UTF8 in `gen_ai.tool.call.arguments` and its opted-in vendor mirror; analogous output/error/exception fields share the same helper. This contradicts row31 criterion3 and README finite-byte/valid-UTF8 statement. It predates the vendor change but remains an unresolved defect in the explicitly required acceptance behavior.

Reproduction retained in `/tmp/toolsy-task41/d25-acceptance-a/adversarial_test.go`, loaded through `/tmp/toolsy-task41/d25-acceptance-a/overlay.json` without adding a repository file. `TestAcceptanceAInvalidUTF8RedactorOutput` fails with `invalid UTF8 gen_ai.tool.call.arguments`. Normalize or fail closed on invalid post-redactor UTF8 before enforcing the byte cap, then repeat acceptance. A doc disclaimer alone weakens the stated criterion.

## Validation

- `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go test -race -count=3 ./...` from ext/toolsyotel: PASS2.191s, race.log.
- `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache GOLANGCI_LINT_CACHE=/tmp/toolsy-task41/d25-acceptance-a/lintcache /opt/homebrew/bin/golangci-lint run --allow-parallel-runners ./...`: 0 issues, lint.log. Version2.14.0, Go1.27.1.
- Overlay independent adversarial matrix: 7 paths ×2 vendor ×2 capture =28 passing cases, checking redaction-before-cap with multibyte strings, all event attrs, vendor mirrors, canonical metadata, hard/control/abort return identity, original panic, neutral control/abort, success-only result, rejected chunk exclusion. Full overlay command fails solely on the separate invalid-UTF8 probe. adversarial.log.
- `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go test -run '^$' -bench '^BenchmarkTracingMapping$' -benchtime=100ms -benchmem -count=1`: PASS. metadata333.5ns/592B/9alloc; vendor metadata329.1ns/592B/9alloc; capture1803ns/5736B/15alloc; capture+vendor2243ns/6504B/17alloc; bench.log. No-op provider, local M1 Max, no throughput claim.
- `git diff --check`: PASS.
- Default behavioral regression fixture SHA256 matches evidence: 65d6a01dd0b45f6252725321598769cb77ba9e92afeff64530c071703b181a82. Retained baseline shows pre-change unexpected langfuse.observation.type; did not restore or mutate baseline tree.

## Sources and limitations

Checked [OTel GenAI attribute registry](https://opentelemetry.io/docs/specs/semconv/registry/attributes/gen-ai/) for canonical call.id and success-only result semantics; checked [Langfuse native OTel mapping](https://langfuse.com/integrations/native/opentelemetry) for observation type/input/output names. Registry describes these conventions as moved to the GenAI repository, consistent with documentation's evolving-subset disclaimer.

No live exporter/backend/vendor conformance exercised. Split secrets deliberately survive per-chunk matching; whole-content sanitation is the explicit external host contract. No attempt to certify CPU/memory cost of host redactor or raw input preparation. Only the affected adapter module independently race-tested; root package loaded via local replacement. Main modules and workspace files untouched.
