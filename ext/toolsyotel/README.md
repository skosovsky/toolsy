# toolsyotel

`toolsyotel` is an optional OpenTelemetry middleware module for `toolsy`.
It has no Langfuse SDK or exporter dependency.

## Tool execution tracing

Use `WithTracing` in the registry builder to emit one span per tool call.
The default mapping is vendor-neutral: `gen_ai.tool.name`,
`gen_ai.operation.name=execute_tool`, and `gen_ai.tool.call.id` when supplied.
Content capture is **off by default**, including error/panic messages, exception
attributes and status descriptions. Fixed statuses and exception types remain.
Tool names/call IDs are host-provided metadata; do not put secrets in them.

```go
toolsyotel.WithTracing(
    toolsyotel.WithTracerProvider(tp),
    toolsyotel.WithContentCapture(true), // sensitive content: explicit host decision
    toolsyotel.WithMaxPayloadSize(4096), // bytes per field, marker included; default 4096
    toolsyotel.WithContentRedactor(redactor), // host-owned, concurrent-safe
)
```

This adapter uses an explicit subset of the evolving
[OTel GenAI attributes](https://opentelemetry.io/docs/specs/semconv/registry/attributes/gen-ai/).
Framework-specific fields use the `toolsy.tool.*` namespace. With capture enabled:

| Attribute | Meaning |
|---|---|
| `gen_ai.tool.call.arguments` | Captured input |
| `toolsy.tool.output` | Concatenated successfully delivered chunks when Execute returns nil |
| `gen_ai.tool.call.result` | Same captured output only when Execute returns nil without a soft error chunk |
| `toolsy.tool.error` | Captured Execute error, including control/stream-abort error text |
| `toolsy.tool.soft_error_text` | Last nonempty captured soft error summary |
| `exception.message` | Captured hard error or panic diagnostic |

`toolsy.tool.soft_error`, `toolsy.tool.control_signal` and
`toolsy.tool.stream_aborted` are content-free flags. Soft/hard errors set error
status; control signals and stream aborts retain neutral status. Panic values are
re-thrown unchanged after the span ends. A panic does not finalize output/result;
input and an opted-in exception message may already have been captured.

Each captured field has a finite byte cap with valid UTF-8 preserved. Invalid
byte sequences (including host redactor output) become U+FFFD before the cap is
applied, so replacement bytes also count toward the budget. Values over
that cap use `... [truncated]` within the cap; tiny caps may omit the marker.
Nonpositive configured limits fall back to 4096. These are observability display
strings, possibly truncated or non-JSON, not executable arguments/results or a
transport read primitive (see [display-tier policy](../../docs/migration-task30.md)).

## Langfuse compatibility is explicit

Add `WithLangfuseCompatibility(true)` to opt into
[Langfuse observation mapping](https://langfuse.com/integrations/native/opentelemetry):

```go
toolsyotel.WithTracing(
    toolsyotel.WithTracerProvider(tp),
    toolsyotel.WithLangfuseCompatibility(true),
    toolsyotel.WithContentCapture(true), // independent opt-in; omit to keep payloads off
    toolsyotel.WithContentRedactor(redactor),
)
```

The flag adds `langfuse.observation.type=tool`. Only when content capture is also
on does it mirror captured arguments into `langfuse.observation.input` and captured
output/error into `langfuse.observation.output`. Both mappings share the same
redacted, capped field value; the vendor flag cannot enable capture or bypass its
policy. On panic there is no observation output. Provider/exporter setup and live
backend compatibility are owned by the host and are not certified here.

## Redaction boundary: split secrets can survive

**Redaction runs per delivered chunk, before concatenation. A secret split across
chunks can survive a whole-secret matcher and reappear in the combined output.**
For example, matching `SECRET_TOKEN` separately against `SECRET_` and `TOKEN`
removes nothing. A size cap does not provide secret sanitation.

`WithContentRedactor` does not enable capture. It runs before truncation, may run
concurrently, and must be safe for every `ContentKind` (input/output/error/panic).
A redactor panic fails closed to bounded `[redaction failed]` content. Rejected
chunks are not captured. Redactor calls and raw input/chunk preparation are not
CPU/memory-bounded by the exported field cap.

Hosts must disable capture for sensitive tools, omit whole sensitive chunks using
a trusted classifier, or sanitize complete content under a separate bounded host
contract before sending it through this middleware. This extension does not
provide a stateful whole-stream sanitizer, universal detector or scheduling layer.
Host authentication and secret classification remain outside it.

## Migration and wrappers

D25 removes default Langfuse attributes. Opt into the vendor flag if dashboards
need that mapping. Update old `gen_ai.tool.call_id` queries to
`gen_ai.tool.call.id`; replace custom `gen_ai.tool.soft_error`,
`gen_ai.tool.soft_error_text`, `gen_ai.tool.control_signal` and
`gen_ai.tool.stream_aborted` with the corresponding `toolsy.tool.*` names.
`gen_ai.tool.call.result` now represents only successful execution; use the
explicit output/error fields for diagnostics. No legacy alias is emitted.
See [task41 migration](../../docs/migration-task41.md).

If wrapping tools manually before `RegistryBuilder.Add`, implement
`toolsy.ChainUnwrapper.UnwrapNext()` so Build can detect invalid nested async
chains. `tracingTool` does so; prefer `Use(WithTracing(...)).Add(...)` for async tools.

## Conversation observability

Conversation compaction and telemetry are owned by the host/contexty.
`RecordSemanticTruncation` was removed with `toolsy/history`.
See [the migration](../../docs/history-compaction-migration.md).
