# Toolsy: File System Toolkit (fstool)

**Description:** Lets the agent safely list directories, read files, and optionally write files within a sandboxed base directory. All paths are validated to prevent path traversal and symlink escape.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/fstool
```

**Dependencies:** stdlib only; requires `github.com/skosovsky/toolsy` (core).

## Available tools

| Tool           | Description                          | Input                                      |
|----------------|--------------------------------------|--------------------------------------------|
| `fs_list_dir`  | List files and directories in a path | `{"path": "string"}`                       |
| `fs_read_file` | Read contents of a text file         | `{"path": "string"}`                       |
| `fs_write_file`| Write content to a file              | `{"path": "string", "content": "string"}`  |

- List result: `{"entries": [{"name": "...", "is_dir": bool, "size": int64}]}`.
- Read result: `{"content": "..."}`. Content is truncated to `maxBytes` (default 1 MB) with `[Truncated]` suffix if longer (UTF-8 safe).
- Write result: `{"status": "Success"}`. When `WithReadOnly(true)` is set, `fs_write_file` is not registered.

## Configuration & Security

> **Warning:** Path traversal protection is critical. The toolkit uses `filepath.Rel` (not string prefix) so that paths like `/app/sandbox-bypass/secret` are rejected.

- **Base directory:** `AsTools(baseDir, opts...)` requires an existing directory. All agent paths are resolved relative to `baseDir` and validated after symlink resolution.

- **Symlink resolution:** Paths are resolved with `filepath.EvalSymlinks`. If a symlink inside the sandbox points outside, the path is rejected.

- **Read-only mode:** Use `WithReadOnly(true)` to disable `fs_write_file` (e.g. read-only agent).

- **Max bytes:** Use `WithMaxBytes(n)` to limit file read size (default 1 MB). Truncation is UTF-8 safe.

- **Tool names/descriptions:** Use `WithListDirName`, `WithReadFileName`, `WithWriteFileName`, and the corresponding `With*Description` options to customize.

## Quick start

```go
package main

import (
	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/fstool"
)

func main() {
	reg := toolsy.NewRegistry()

	tools, err := fstool.AsTools("/tmp/agent_workspace", fstool.WithReadOnly(true))
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		reg.Register(tool)
	}
}
```
