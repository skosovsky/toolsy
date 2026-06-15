# Toolsy: File System Toolkit (fstool)

**Description:** Lets the agent safely list directories, read files, and optionally write files within a sandboxed base directory. All paths are validated to prevent path traversal and symlink escape.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/fstool
```

**Dependencies:** requires `github.com/skosovsky/toolsy` (core); wire JSON capping uses `github.com/skosovsky/toolsy/internal/format` from the same repo checkout (via `replace` in development).

## Available tools

| Tool            | Description                          | Input                                     |
| --------------- | ------------------------------------ | ----------------------------------------- |
| `fs_list_dir`   | List files and directories in a path | `{"path": "string"}`                      |
| `fs_read_file`  | Read contents of a text file         | `{"path": "string"}`                      |
| `fs_write_file` | Write content to a file              | `{"path": "string", "content": "string"}` |

- List result: `{"entries": [{"name": "...", "is_dir": bool, "size": int64}]}`.
- Read result: `{"content": "..."}`. `WithMaxBytes` is the **wire JSON** budget (default 1 MB). Fail-closed reads use `contentByteCap = maxWire - envelopeOverhead` (envelope `{"content":"..."}` ≈ 15 bytes); exceeding the content cap returns **`CodeValidationFailed`** (stat pre-check or read).
- Write result: `{"status": "Success"}`. When `WithReadOnly(true)` is set, `fs_write_file` is not registered.

## Configuration & Security

> **Warning:** Path traversal protection is critical. The toolkit uses `filepath.Rel` (not string prefix) so that paths like `/app/sandbox-bypass/secret` are rejected.

- **Base directory:** `AsTools(baseDir, opts...)` requires an existing directory. All agent paths are resolved relative to `baseDir` and validated after symlink resolution.

- **Symlink resolution:** Paths are resolved with `filepath.EvalSymlinks`. If a symlink inside the sandbox points outside, the path is rejected.

- **Read-only mode:** Use `WithReadOnly(true)` to disable `fs_write_file` (e.g. read-only agent).

- **Max bytes:** Use `WithMaxBytes(n)` as the wire JSON budget for read_file. Transport reads use the content cap derived from envelope overhead; exceeding returns **`CodeValidationFailed`** (fail-closed).

- **Tool names/descriptions:** Use `WithListDirName`, `WithReadFileName`, `WithWriteFileName`, and the corresponding `With*Description` options to customize.

## Quick start

```go
package main

import (
	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/fstool"
)

func main() {
	builder := toolsy.NewRegistryBuilder()

	tools, err := fstool.AsTools("/tmp/agent_workspace", fstool.WithReadOnly(true))
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		builder.Add(tool)
	}
}
```
