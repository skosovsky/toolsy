# Docker Sandbox Adapter

`docker` runs code inside ephemeral containers with language-specific runtime
templates. By default it supports:

- `python` via `python:3.11-alpine`
- `bash` via `bash:5.2`
- `node` via `node:22-alpine`

Use `WithImageMapping`, `WithNetworkDisabled`, and `WithMemoryLimit` to tighten
runtime policy for production agents.

Container teardown and post-run log collection use bounded cleanup timeouts so
deadline paths cannot hang indefinitely.

Workspace archive reads each file with fail-closed `textprocessor.ReadLimitedBytes`
(`defaultMaxArchiveFileBytes`, 64 MiB per file) — not unbounded `io.ReadAll`.
