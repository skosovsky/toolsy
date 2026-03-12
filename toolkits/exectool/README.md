# Toolsy: Exec Toolkit (exectool)

**Description:** Lets the agent run Python or Bash code in an isolated sandbox. The toolkit never executes code on the host; execution is delegated via the Sandbox interface (Docker, Lambda, E2B, etc.).

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/exectool
```

**Dependencies:** stdlib only; requires `github.com/skosovsky/toolsy` (core).

## Available tools

| Tool          | Description                    | Input             |
|---------------|--------------------------------|-------------------|
| `exec_python` | Run a Python script in sandbox | `{"code": "string"}` |
| `exec_bash`   | Run a Bash script in sandbox   | `{"code": "string"}` |

Result: `{"output": "Exit Code: N\nStdout:\n...\nStderr:\n..."}`. Empty stdout or stderr blocks are omitted. Each block is truncated to at most `n` bytes (default 512KB). Truncation is strict: total length of each block never exceeds `n`; for very small `n` the suffix `[Truncated]` is omitted if it would exceed the limit.

## Configuration & Security

> **Warning:** Code is never run by the toolkit. You must provide a Sandbox implementation (e.g. gRPC to a Docker container, AWS Lambda, E2B). At least one of `WithPython()` or `WithBash()` must be set.

- **WithPython()** / **WithBash()**: Enable the corresponding tool. Only enabled tools are registered.
- **WithMaxOutputBytes(n)**: Cap stdout and stderr length each (default 512KB). UTF-8 safe; output never exceeds `n` bytes per block (for small `n`, no suffix is appended).

## Quick start

```go
package main

import (
	"context"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/exectool"
)

// Minimal in-memory Sandbox for a compilable example (no code is executed).
type noopSandbox struct{}
func (noopSandbox) Run(ctx context.Context, language, code string) (*exectool.Result, error) {
	return &exectool.Result{ExitCode: 0}, nil
}

func main() {
	reg := toolsy.NewRegistry()
	tools, err := exectool.AsTools(noopSandbox{}, exectool.WithPython(), exectool.WithBash())
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		reg.Register(tool)
	}
}
```
