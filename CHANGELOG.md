# Changelog

## Unreleased (task31/task32/task33/task34 contracts)

### Breaking

- MCP client now supports only protocol revision `2026-07-28`; `2025-11-25`, older revisions, downgrade and compatibility fallback are removed.
- MCP lifecycle is stateless: `Connect` uses strict `server/discover` and exact `supportedVersions`; initialize/initialized, protocol sessions, `Mcp-Session-Id`, HTTP GET/resume/DELETE and `ErrSessionExpired` are removed.
- Roots, session logging, base ping, server-request dispatch and resource subscribe/unsubscribe APIs are removed without deprecated aliases.
- All MCP results require `resultType`; cacheable discover/list/read results require typed `ttlMs`/`cacheScope`. `input_required` is surfaced only for tools/call, resources/read and prompts/get, without automatic MRTR retry.
- Streamable HTTP is POST-only and emits exact protocol/method/name routing headers. Valid `x-mcp-header` tool arguments are mirrored through `Mcp-Param-*`, including the `=?base64?...?=` sentinel encoding.
- Legacy `SSETransport`, `NewSSETransport`, `SSETransportOption` and `WithSSE*` APIs are removed in favor of single-endpoint `StreamableHTTPTransport`.
- MCP transport is request-oriented and no longer exposes server-to-client request dispatch; request IDs remain available before awaiting a result.
- MCP progress, content and prompt DTOs use their `2026-07-28` wire shapes. `ProgressInfo.Current` and `Total` preserve fractional wire values.
- Non-standard `ResourceContents.annotations` is removed; annotations remain only where the `2026-07-28` schema defines them.
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

- Strict MCP `2026-07-28` discovery/capability validation, self-describing request metadata and stateless stdio/Streamable HTTP transports.
- `subscriptions/listen` invalidation streams with acknowledgment-first subscription ID/effective-filter validation.
- Typed current-revision result/cache/MRTR/error contracts, including `HeaderMismatch` (`-32020`), missing client capability (`-32021`) and unsupported protocol (`-32022`).
- `ComputeSnapshotDigest` and the closed `Snapshot` set for strict, canonical, type-separated discovery/list/read snapshot identity including cache metadata.
- HTTP `x-mcp-header` validation, invalid-tool filtering, safe-integer primitive mirroring and exact Base64 sentinel encoding.
- Correlated terminal JSON/POST-SSE responses and queued stdio writes with request IDs available before I/O completes; interrupted HTTP streams are terminal and never resumed implicitly.
- Exact range-based cancelled-ID correlation, WHATWG-complete SSE framing, and independent stdio process/pipe shutdown with typed terminal-cause propagation.
- MCP structured tool outputs with output-schema validation and lossless text/image/audio/resource content blocks.
- Typed MCP protocol, capability, stale discovery/subscription, JSON-RPC, HTTP/stdio and remote execution errors.
- Symmetric schema validation for decoded and programmatically constructed exported MCP wire DTOs.
- JSON-RPC union fields, duplicate keys and reserved extensions now fail closed; MCP enums, tool names, complete schemas and binary Base64 are validated at the DTO boundary.
- Operation state is published only after strict discovery succeeds; schema-valid empty implementation and prompt strings are preserved.
- Icon metadata uses parser-flag-independent component URI grammar for HTTPS authority/scoped IPv6/IPvFuture/port and US-ASCII structural RFC 2397 media parameters, and icon sizes contain strings only; finite negative progress values remain valid and participate in monotonic ordering.
- Server capability extensions preserve arbitrary JSON values losslessly; official `io.modelcontextprotocol/*` IDs are accepted and explicitly codec-registrable but inert by default. Logging/completions/experimental advertisements remain inert raw data with no legacy runtime API, and no Tasks core capability is modeled or advertised. Caller `Meta.Extra` cannot forge MetaObject namespaces whose second DNS label is `mcp` or `modelcontextprotocol`.

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
- MCP clear-break migration notes in [docs/migration-task34.md](docs/migration-task34.md).

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
