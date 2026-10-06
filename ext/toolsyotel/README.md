# toolsyotel

`toolsyotel` is an extension module for OpenTelemetry instrumentation around `toolsy`.

## Tool execution tracing

Use `WithTracing` middleware on the registry builder to emit one span per tool call. By default only bounded metadata is recorded (`gen_ai.tool.name`, `gen_ai.operation.name`, `langfuse.observation.type`).

Opt-in payload capture for Langfuse / GenAI SemConv (may contain PII):

```go
toolsyotel.WithTracing(
    toolsyotel.WithTracerProvider(tp),
    toolsyotel.WithContentCapture(true),
    toolsyotel.WithMaxPayloadSize(4096), // bytes per captured field, marker included; default 4096
)
```

When enabled, spans include `langfuse.observation.input` / `output` and `gen_ai.tool.call.arguments` / `result`, truncated with `... [truncated]` when over the limit. Payload truncation is **observability display tier** — not a transport read primitive (see [docs/migration-task30.md](../../docs/migration-task30.md)).

Content capture governs **all** payload paths: arguments, delivered chunks, soft errors, hard errors, and panic diagnostics. With capture off, no error message or panic value is exported in attributes, exception events, or status descriptions. Exception type and fixed statuses remain available; control signals and stream aborts retain neutral status. Panic values are rethrown unchanged.

`WithContentRedactor(func(kind toolsyotel.ContentKind, content string) string { ... })` installs a host-owned, vendor-neutral redactor. It does not enable capture. It runs before truncation, and may run concurrently. A redactor panic fails closed to a bounded `[redaction failed]` diagnostic. Streams are redacted per delivered chunk, so host redactors must handle that boundary (e.g. replace entire sensitive chunk contents rather than assume secrets cannot span chunks). Only delivered chunks are captured.

Every captured field has a finite byte limit, including any marker, with valid UTF-8 preserved. Tiny limits may omit the truncation marker. Soft errors always set `gen_ai.tool.soft_error`; `gen_ai.tool.soft_error_text` exists only with capture enabled and passes through the same policy. Exception messages are also captured only by opt-in; statuses always use fixed descriptions. Host authentication, secret classification, and exporter configuration remain outside this extension.

If you wrap tools manually before `RegistryBuilder.Add` (instead of using `Use`), the wrapper must implement `toolsy.ChainUnwrapper` with `UnwrapNext()`. This lets `Build` detect invalid nested `AsAsyncTool` chains. `tracingTool` implements the contract; prefer `Use(WithTracing(...)).Add(...)` for async tools.

## Conversation observability

Conversation compaction and its telemetry are owned by the host/contexty.
`RecordSemanticTruncation` was removed together with `toolsy/history`.
See [the migration](../../docs/history-compaction-migration.md).
Tool execution tracing and its content policy remain available.
