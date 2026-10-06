# Wazero Sandbox Adapter

`wazero` runs a precompiled WASI guest interpreter and exposes it as a single
LLM-safe language. The generic `exec_code` tool should never expose `wasm`
directly; instead, configure this adapter with the text language your guest
interpreter understands, such as `jq` or `rego`.

## Execution contract

Each run creates a fresh runtime and temporary workspace. Custom runtime configs
retain mandatory context termination and the adapter's memory limit: these settings
are applied last. `WithMemoryLimitPages` selects 1–65536 WASM pages (64 KiB each);
the default is 1024 pages (64 MiB) per guest linear memory. This is enforced by
wazero at module validation and memory growth. It is not a hard limit on the Go
process, compiler allocations, guest tables, or filesystem usage. A host needing
hard process memory isolation must choose another backend. Execution time is
bounded by the caller's context; supply a deadline for untrusted guests.

No parent environment is inherited. Guests receive only `RunRequest.Env`, WASI
imports, and a read-only workspace mounted through `os.Root.FS()`. Relative
traversal and symlinks cannot escape the workspace. Guests cannot create, modify
or delete workspace files; use stdout to return results. stdout and stderr are
independently capped at the shared sandbox output limit. A nonzero guest exit is a normal
`RunResult`; compilation, setup and collection errors are infrastructure failures.

Runtime cleanup uses a fresh five-second context even after caller cancellation.
Cleanup failures are returned as typed `exectool.CleanupError` diagnostics while
preserving the primary error and any completed result. Temporary workspace removal
uses its own five-second context with checks between paged directory reads and
relative deletions. Individual local filesystem calls have no cancellation or
hard wall-clock guarantee, so a stalled syscall can delay return. This adapter
does not claim bounded filesystem I/O or hard filesystem quotas. Runtime close
errors and workspace removal errors are never silently discarded.
