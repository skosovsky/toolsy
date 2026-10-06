# D25 independent acceptance A — corrected diff

Verdict: **accepted, 100%**. Reviewed corrected uncommitted D25 against HEAD7331736ac925527fe5efe2711eb59c292a172ffc. No unresolved detected errors. This verdict supersedes the original90% rejection; the original report remains `report-initial-superseded.md` and original failing probe run remains `adversarial.log`. No other reviewer verdict read. No repository/git/workspace files changed by reviewer A.

| Criterion | Score | Independent evidence |
|---|---:|---|
| 1. Portable default/canonical metadata/vendor opt-in | 20/20 | No vendor attributes by default even with capture; canonical gen_ai.tool.call.id; explicit Langfuse flag and no SDK dependency. |
| 2. Independent default-off bounded/redacted policy | 20/20 | All payload paths capture-off and vendor-off/on matrix; redactor not called without capture; mirrors identical capped redacted portable values; fixed statuses; exception payload respects policy. |
| 3. Delivery/truthful classification/semantics/UTF8 | 20/20 | 28 independent outcome cases plus exact old invalidUTF8 probe and48 tiny-budget combinations pass; original errors/panics preserved, rejected chunks excluded, result success-only, UTF8 normalized before budget. |
| 4. AAA regressions/baseline/race/lint/benchmark | 20/20 | Existing baseline/AAA vendor and split-secret evidence reviewed; independent fullmodule race count3 and pinnedlint pass; local hotpath benchmark repeated after correction. |
| 5. Honest docs/caps/host boundary/no unresolved errors | 20/20 | README and migration agree on U+FFFD before cap and independent opt-in. Split-secret limitation prominent, complete bounded sanitation external; no universal/stateful sanitizer or live-conformance claim. |

## Original issue resolved

`payload.go` now applies `strings.ToValidUTF8(s, "\uFFFD")` before measuring and truncating. This closes short-string bypass at old payload.go15-16 for raw arguments/errors and arbitrary host redactor return strings. Replacement rune budget is accounted for: caps1/2 omit the3-byte rune, cap3 exports it complete. No post-redactor path can export the originally reproduced invalid byte through this helper. Documentation describes the behavior and new repository test covers vendor×result/soft/hard/panic.

The unchanged `TestAcceptanceAInvalidUTF8RedactorOutput` now PASSes. Additional independent `TestAcceptanceAUTF8TinyBudgets` covers budgets1/2/3 ×raw/redactor ×vendor false/true ×result/soft/hard/panic =48 cases. It inspects every content-bearing attribute including toolsy.tool.error and every event attribute for byte cap and UTF8, and asserts exact replacement/omission input and hard-error values. All pass.

## Independent corrected validations

- `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go test -race -count=3 ./...` from ext/toolsyotel: PASS1.597s (`race-recheck.log`).
- `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache GOLANGCI_LINT_CACHE=/tmp/toolsy-task41/d25-acceptance-a/lintcache /opt/homebrew/bin/golangci-lint run --allow-parallel-runners ./...`:0 issues (`lint-recheck.log`), pinned2.14.0.
- `GOWORK=off GOCACHE=/tmp/toolsy-review-gocache go -C ext/toolsyotel test -overlay=/tmp/toolsy-task41/d25-acceptance-a/overlay.json -race -run '^TestAcceptanceA' -count=1 -v`: PASS1.858s (`adversarial-recheck.log`). 28 outcome matrix cases, exact former failing probe,48 tiny-budget cases.
- Repeated `BenchmarkTracingMapping` with benchtime100ms/count1/benchmem: PASS (`bench-recheck.log`). Local no-op provider only; no performance improvement or live exporter throughput assertion.
- `git diff --check`: PASS.
- Retained behavioral baseline fixture SHA256 independently matched65d6a01dd0b45f6252725321598769cb77ba9e92afeff64530c071703b181a82; pre-change failure on default langfuse.observation.type reviewed without restoring baseline tree.

## Sources and limitations

[OTel GenAI registry](https://opentelemetry.io/docs/specs/semconv/registry/attributes/gen-ai/) supports canonical call.id and success-only result; [Langfuse native OTel mapping](https://langfuse.com/integrations/native/opentelemetry) supports the observation type/input/output names. GenAI conventions have moved and remain evolving; reviewed only the adapter's documented subset.

No live backend or exporter conformance. Per-delivered-chunk redaction intentionally cannot detect split secrets; complete bounded sanitation/classification remains host responsibility. Field caps do not certify raw preparation or host redactor CPU/memory cost. Independently tested affected module, with root loaded by local go.mod replacement; did not rerun unrelated modules. Repository, git and go.work.sum untouched.
