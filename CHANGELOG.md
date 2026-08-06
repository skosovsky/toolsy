# Changelog

## Unreleased (task31/task32/task33 contracts)

### Breaking

- MCP client now supports only protocol revision `2025-11-25`; older revisions and compatibility fallback are removed.
- Legacy `SSETransport`, `NewSSETransport`, `SSETransportOption` and `WithSSE*` APIs are removed in favor of single-endpoint `StreamableHTTPTransport`.
- MCP `Transport` is now a bidirectional JSON-RPC peer using `Request`/`PendingRequest`; request IDs are available before awaiting a result.
- MCP roots, progress, content and prompt DTOs use their `2025-11-25` wire shapes. `ProgressInfo.Current` and `Total` preserve fractional wire values.
- Non-standard `ResourceContents.annotations` is removed; annotations remain only on content blocks where the `2025-11-25` schema defines them.
- MCP `resource_link.size` now uses lossless `JSONNumber`; mathematically integral forms such as `1`, `1.0` and `1e3` retain their exact wire representation, while fractional values and present `null` fail strict encoding and decoding.

- `Registry.View` is the primary capability boundary; `Subset` now delegates to a capability-backed view.
- Calls to tools outside an active view manifest return `CodeCapabilityDenied`; `Session.RunCall` classifies policy/capability denials as infrastructure/pre-tool failures.
- `RestoreView` validates durable snapshot identity and manifest digest before recreating a view.
- Non-empty `ToolRequirements` require an attached requirements policy before execution.
- `TypedToolSpec.Handler` now receives `ValidatedArgs[TArgs]`; raw validation moves to `ArgsBinder[TArgs]`.
- Registered state snapshot slots reject explicit JSON `null` by default; strict-mode unknown `null` keys fail closed.
- `RegistryViewSpec.Policy` now requires a stable `PolicyID`; restore validates the policy digest as part of view identity.
- `SessionSnapshot` is stamped with session binding and cannot be imported into an incompatible registry/view/schema.

### Added

- Strict MCP lifecycle/capability negotiation, Streamable HTTP sessions/version headers/resume, roots requests and discovery invalidation.
- Correlated terminal JSON POST responses, independent POST-SSE resume state, retry-before-reconnect, and queued stdio writes with request IDs available before I/O completes.
- Exact range-based cancelled-ID correlation, WHATWG-complete SSE framing, and independent stdio process/pipe shutdown with typed terminal-cause propagation.
- MCP structured tool outputs with output-schema validation and lossless text/image/audio/resource content blocks.
- Typed MCP protocol, capability, stale discovery, JSON-RPC, HTTP/session and remote execution errors.
- Symmetric schema validation for decoded and programmatically constructed exported MCP wire DTOs.
- JSON-RPC union fields and reserved extensions now fail closed; MCP enums, file roots, tool names, complete object schemas and binary base64 are validated at the DTO boundary.
- Operation state is published only after `notifications/initialized` has been delivered; schema-valid empty implementation and prompt strings are preserved.
- Icon metadata uses parser-flag-independent component URI grammar for HTTPS authority/scoped IPv6/IPvFuture/port and US-ASCII structural RFC 2397 media parameters, and icon sizes contain strings only; finite negative progress values remain valid and participate in monotonic ordering.
- Server capability extensions preserve arbitrary JSON values losslessly, while known experimental and task capability branches follow their schema-defined object shapes.

- Typed call context, typed tool policy, structured tool effects, and `ToolResult` helpers.
- Registry view snapshots with manifest digest, required tool validation, and restore-time mismatch checks.
- Requirements policy support for host-owned subject/scope types.
- `SessionBinding`, `SessionCheckpoint`, `Session.Rebind`, and view-scoped checkpoint restore helpers.
- State schema digests include slot policy, value/codec type, and optional `WithStateSlotSchemaID`.
- `ToolEnvelope` on chunks/outcomes for result/error delivery classification without JSON sniffing.
- `NewPolicyToolFromSpec` for one-step policy-aware generic tool construction.
- `NewPolicyTool` for binder/policy/requirements hardening around existing generic tools.
- Migration notes in [docs/migration-task31.md](docs/migration-task31.md).
- Migration notes in [docs/migration-task32.md](docs/migration-task32.md).

## v1.0 (task28 hardening)

### Breaking

- **`WithStrictStateCodecs(true)`** — export/import requires registered codecs for non-nil state keys; use `CodeStateCodecMissing` on violation.
- **Error chunks** — `validateErrorChunk` accepts only `MimeTypeToolErrorJSON`; legacy text error chunks are normalized to structured wire with `CodeInternal` at delivery time.
- **`ImportSnapshot`** — unsupported version and corrupt payload return `*ToolError` with `CodeInternal` (`Retryable: false`) instead of plain `fmt.Errorf`.

### Added

- `WithStrictStateCodecs`, `CodeStateCodecMissing`, `NewStateCodecMissingError`, `NewSnapshotHydrationError`.
- `normalizeErrorChunk`, `prepareChunk` in chunk delivery pipeline.
- `examples/resiliency` migrated to `Session.RunCall` + `NewTypedTool`.

See [docs/migration-task28.md](docs/migration-task28.md) and [docs/adr/adr-task28-hardening.md](docs/adr/adr-task28-hardening.md).

## v0.9.0

### Breaking

- **`NewRunEnv(session *Session, opts ...)`** — first argument binds in-memory state. Use `nil` for DI-only environments. See [docs/migration-task28.md](docs/migration-task28.md).
- **In-memory state moved to `Session`** — `runEnvStore` no longer holds a `state` map. Use `SetSessionState` / `GetSessionState` on `*Session`, or `SetState` / `GetState` on a `RunEnv` created with that session.

### Added

- `Session.Export()` / `Session.Import()` for checkpoint serialization (state only; not deps or attachments).
- `StateTypeRegistry` and `WithStateTypeRegistry` for typed JSON roundtrips on `Import`.
- `StateTypeRegistry.Register` returns `error` on invalid key, prototype, or nil registry.
- `Session.Export()` on nil `*Session` returns an empty map (not nil).
- `ValidateRunEnvSession` and env binding checks in `Session.Execute`.

### Notes (async pipeline)

- Global registry middleware for `AsAsyncTool` runs in the background goroutine when registered via `RegistryBuilder` (task24). Documented in README and `AsAsyncTool` godoc.

### Added (library hardening, task26)

- `RegistryBuilder.Build` rejects nested `AsAsyncTool(AsAsyncTool(...))` with a clear error (chain walk via `ChainUnwrapper`, including manual middleware before Add).
- `ChainUnwrapper` / `UnwrapNext` contract for tool wrappers; `ext/toolsyotel` tracing middleware implements it.
- `WithMaxCollectedChunks`, `DefaultMaxCollectedChunks` (1000), and `ErrAsyncCollectedLimitExceeded` for background chunk collection.
