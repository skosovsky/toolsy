# Changelog

## v0.9.0

### Breaking

- **`NewRunEnv(session *Session, opts ...)`** — first argument binds in-memory state. Use `nil` for DI-only environments. See [docs/migration-task23.md](docs/migration-task23.md).
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
