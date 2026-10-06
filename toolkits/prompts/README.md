# Toolsy: Prompts Toolkit (dynamic roles)

**Description:** Bridges any prompt provider (prompty, file, Git) to toolsy: one tool that returns rendered system instructions for a role and optional template variables.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/prompts
```

**Dependencies:** stdlib only; requires `github.com/skosovsky/toolsy` (core). No dependency on prompty—implement the `Provider` interface and pass it in.

## Available tools

| Tool                     | Description                        | Input                                                  |
| ------------------------ | ---------------------------------- | ------------------------------------------------------ |
| `get_agent_instructions` | Get system prompt for a given role | `{"role_id": "string", "variables": {"key": "value"}}` |

Output: JSON containing rendered `instructions` and optional provider-supplied `source` and `version`.

## Configuration and security

`Provider.Get` returns a `Document`. The host selects a trusted provider and controls which roles the caller may access. Returned text is data: the toolkit does not install it as a system message, authorize actions, or make provider content trusted by its presence in a tool result. Source/version are provenance supplied by the provider, not verified identity or authority. No prompt repository is built in.

Defaults are 1 MiB total returned source field bytes, 512 KiB instructions bytes, 4096 bytes per provenance field, and 1 MiB final JSON wire bytes after escaping. Override via `WithMaxSourceBytes`, `WithMaxBytes`, `WithMaxProvenanceBytes`, `WithMaxOutputBytes`. Zero selects the default; negative configuration fails construction. Nil (including typed nil) providers fail construction. Oversize or invalid UTF-8 results fail explicitly: instructions are never silently truncated. No continuation token is available from this port. Host providers must bound their own source transport and render allocations, respect context and enforce access controls; checking the returned document cannot constrain allocations made inside the provider.

`WithName` and `WithDescription` customize registry metadata. Provider errors retain their causes.

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

func (m *myProvider) Get(ctx context.Context, roleID string, variables map[string]any) (prompts.Document, error) {
	// Load manifest, render template with variables
	return prompts.Document{Instructions: "You are a helpful assistant.", Source: "host://roles/helper", Version: "1"}, nil
}

func main() {
	builder := toolsy.NewRegistryBuilder()
	promptsTool, err := prompts.AsTool(&myProvider{})
	if err != nil {
		panic(err)
	}
	builder.Add(promptsTool)
}
```


Nil options reject construction. Host ports and callbacks are borrowed; the host
owns their lifetime and synchronization. See the [shared constructor and ownership
contract](../README.md#constructor-configuration-and-ownership) for option snapshots
and the distinction between configuration containers and mutable host ports.
