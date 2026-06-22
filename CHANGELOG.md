# Changelog

## Unreleased (task31/task32 contracts)

### Breaking

- `Registry.View` is the primary capability boundary; `Subset` now delegates to a capability-backed view.
- Calls to tools outside an active view manifest return `CodeCapabilityDenied`; `Session.RunCall` classifies policy/capability denials as infrastructure/pre-tool failures.
- `RestoreView` validates durable snapshot identity and manifest digest before recreating a view.
- Non-empty `ToolRequirements` require an attached requirements policy before execution.
- `TypedToolSpec.Handler` now receives `ValidatedArgs[TArgs]`; raw validation moves to `ArgsBinder[TArgs]`.
- Registered state snapshot slots reject explicit JSON `null` by default; strict-mode unknown `null` keys fail closed.
- `RegistryViewSpec.Policy` now requires a stable `PolicyID`; restore validates the policy digest as part of view identity.
- `SessionSnapshot` is stamped with session binding and cannot be imported into an incompatible registry/view/schema.

### Added

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
