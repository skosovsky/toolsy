# Toolsy: Human Toolkit (Suspend-First HITL)

**Description:** Lets the agent request human approval for dangerous actions and ask for clarification without blocking the tool goroutine. Each tool yields one suspend chunk and then returns `toolsy.ErrSuspend`.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/human
```

**Dependencies:** stdlib only; requires `github.com/skosovsky/toolsy` (core).

## Available tools

| Tool                     | Description                          | Input                                              |
|--------------------------|--------------------------------------|----------------------------------------------------|
| `request_approval`       | Request human approval for an action | `{"action": "string", "reason": "string"}`         |
| `ask_human_clarification`| Ask a human for clarification        | `{"question": "string"}`                           |

## Configuration

- **Tool names and descriptions:** Use `WithApprovalName`, `WithApprovalDescription`, `WithClarificationName`, `WithClarificationDescription` to customize.
- **Suspend-first behaviour:** `request_approval` and `ask_human_clarification` each emit one JSON chunk with `EventSuspend` and `MimeTypeJSON`, then return `toolsy.ErrSuspend`. The orchestrator is responsible for checkpointing state, pausing the run, and resuming later with external input.

## Quick start

```go
package main

import (
	"context"
	"errors"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/human"
)

func main() {
	ctx := context.Background()
	builder := toolsy.NewRegistryBuilder()

	tools, err := human.AsTools()
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		builder.Add(tool)
	}
	reg, err := builder.Build()
	if err != nil {
		panic(err)
	}

	err = reg.Execute(ctx, toolsy.ToolCall{
		ToolName: "request_approval",
		Input: toolsy.ToolInput{
			CallID:   "1",
			ArgsJSON: []byte(`{"action":"delete","reason":"user asked"}`),
		},
	}, func(c toolsy.Chunk) error {
		// forward c.Data to your UI or job store; it contains:
		// {"kind":"approval","action":"delete","reason":"user asked"}
		return nil
	})
	if errors.Is(err, toolsy.ErrSuspend) {
		// mark the orchestration run as paused/waiting
	}
}
```

## Payloads

`request_approval` yields:

```json
{"kind":"approval","action":"...","reason":"..."}
```

`ask_human_clarification` yields:

```json
{"kind":"clarification","question":"..."}
```

Both payloads are emitted as `Chunk{Event: toolsy.EventSuspend, MimeType: toolsy.MimeTypeJSON}`.
