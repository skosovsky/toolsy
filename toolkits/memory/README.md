# Toolsy: Memory Toolkit (session scratchpad)

**Description:** Lets the agent keep short-lived key-value facts in a session scratchpad backed by `run.State`.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/memory
```

**Dependencies:** stdlib only; requires `github.com/skosovsky/toolsy` (core).

## Available tools

| Tool               | Description                    | Input                          |
|--------------------|--------------------------------|--------------------------------|
| `memory_pin_fact`  | Save a fact to session memory  | `{"key": "string", "value": "string"}` |
| `memory_read_all`  | Read all stored facts         | `{}`                           |
| `memory_unpin_fact`| Remove a fact from session     | `{"key": "string"}`            |

## Configuration and security

- **MaxFacts:** Optional limit on how many facts can be stored. Use `WithMaxFacts(n)` when creating the scratchpad. When the limit is reached, `memory_pin_fact` returns a client error so the LLM can adjust.
- The toolkit does not keep in-process mutable memory. Persistence and lifetime are defined by the `toolsy.StateStore` passed in `RunContext.State` at execution time.

## Quick start

```go
package main

import (
	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/memory"
)

func main() {
	builder := toolsy.NewRegistryBuilder()

	sessionMemory := memory.NewScratchpad(memory.WithMaxFacts(100))
	tools, err := sessionMemory.AsTools()
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		builder.Add(tool)
	}

	// Important: execute calls with RunContext{State: yourStateStore}.
	// Without StateStore, memory tools return a validation error.
}
```
