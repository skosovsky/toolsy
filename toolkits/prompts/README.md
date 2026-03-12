# Toolsy: Prompts Toolkit (dynamic roles)

**Description:** Bridges any prompt provider (prompty, file, Git) to toolsy: one tool that returns rendered system instructions for a role and optional template variables.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/prompts
```

**Dependencies:** stdlib only; requires `github.com/skosovsky/toolsy` (core). No dependency on prompty—implement the `Provider` interface and pass it in.

## Available tools

| Tool                     | Description                        | Input                                                    |
|--------------------------|------------------------------------|----------------------------------------------------------|
| `get_agent_instructions` | Get system prompt for a given role | `{"role_id": "string", "variables": {"key": "value"}}`  |

Output: Rendered instructions text.

## Configuration and security

- **WithName / WithDescription:** Customize tool name and description for the registry.
- **WithMaxBytes(n):** Truncates returned instructions to n bytes (UTF-8 safe). Default 512 KB.

All errors from the provider are wrapped with the `toolkit/prompts:` prefix.

## Quick start

```go
package main

import (
	"context"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/prompts"
)

// Your provider (e.g. adapter around prompty)
type myProvider struct{}

func (m *myProvider) Get(ctx context.Context, roleID string, variables map[string]any) (string, error) {
	// Load manifest, render template with variables
	return "You are a helpful assistant.", nil
}

func main() {
	reg := toolsy.NewRegistry()
	promptsTool, err := prompts.AsTool(&myProvider{})
	if err != nil {
		panic(err)
	}
	reg.Register(promptsTool)
}
```
