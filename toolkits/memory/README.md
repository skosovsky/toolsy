# Toolsy: Memory Toolkit (session scratchpad)

**Description:** Lets the agent keep short-lived key-value facts in session memory (a scratchpad) without a database.

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
- Data is stored only in memory and is lost when the process exits. No disk or network access.

## Quick start

```go
package main

import (
	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/memory"
)

func main() {
	reg := toolsy.NewRegistry()

	sessionMemory := memory.NewScratchpad(memory.WithMaxFacts(100))
	tools, err := sessionMemory.AsTools()
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		reg.Register(tool)
	}
}
```
