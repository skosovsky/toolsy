# Toolsy: Memory Toolkit (session scratchpad)

**Description:** Lets the agent keep short-lived key-value facts in a session scratchpad backed by `run.State`.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/memory
```

**Dependencies:** stdlib only; requires `github.com/skosovsky/toolsy` (core).

## Available tools

| Tool                | Description                   | Input                                  |
| ------------------- | ----------------------------- | -------------------------------------- |
| `memory_pin_fact`   | Save a fact to session memory | `{"key": "string", "value": "string"}` |
| `memory_read_all`   | Read all stored facts         | `{}`                                   |
| `memory_unpin_fact` | Remove a fact from session    | `{"key": "string"}`                    |

## Configuration and security

- Host owns persistence, lifetime and session isolation through `RunEnv.StateStore`.
- Defaults: 128 facts, 256 UTF-8 bytes per key, 4096 per value, 512 KiB stored JSON, 1 MiB final JSON response. `WithMaxFacts`, `WithMaxKeyBytes`, `WithMaxValueBytes`, `WithMaxStoreBytes`, `WithMaxOutputBytes` override them. Zero restores the default; negative values fail `AsTools` construction. There is no unlimited mode.
- Pin validates the complete proposed state before Save; facts are never truncated or evicted. Invalid or over-budget existing state fails closed before mutation. Read returns all facts or a limit error; this backend has no pagination contract.
- Use one Scratchpad instance as the sole writer for each session state key. Its mutex serializes concurrent calls through that instance only. Multiple instances, processes or other writers require host coordination; `StateStore.Load/Save` is not CAS and this toolkit promises no distributed atomicity.
- Store callbacks can allocate before returning; the host must bound transport/storage reads too. The toolkit checks returned bytes before JSON decoding. Keys and values must be valid UTF-8, and keys nonempty.

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

	// Important: pass env with StateStore: toolsy.NewRunEnv(nil, toolsy.WithStateStore(yourStateStore)).
	// Without StateStore, memory tools return a validation error.
}
```
