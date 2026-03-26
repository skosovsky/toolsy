# Toolsy: RAG Toolkit (knowledge base search)

**Description:** Bridges any retriever (vector DB, ragy, etc.) to toolsy: one tool that searches the knowledge base and returns formatted results for the LLM.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/rag
```

**Dependencies:** stdlib only; requires `github.com/skosovsky/toolsy` (core). No dependency on ragy or other RAG libraries—implement the `Retriever` interface and pass it in.

## Available tools

| Tool                    | Description                              | Input           |
|-------------------------|------------------------------------------|-----------------|
| `search_knowledge_base` | Search the knowledge base for information | `{"query": "string"}` |

Output: Markdown with numbered results (and optional source/score if your retriever returns them in the string content).

## Configuration and security

- **WithMaxBytes(n):** Truncates combined result text to n bytes (UTF-8 safe). Default 512 KB.
- **WithMaxResults(n):** Limits how many results are included. Default 10.
- **WithName / WithDescription:** Customize tool name and description for the registry.

All errors from the retriever are wrapped with the `toolkit/rag:` prefix.

## Quick start

```go
package main

import (
	"context"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/rag"
)

// Your retriever (e.g. adapter around ragy)
type myRetriever struct{}

func (m *myRetriever) Retrieve(ctx context.Context, query string) ([]string, error) {
	// Call your vector DB or ragy here
	return []string{"result 1", "result 2"}, nil
}

func main() {
	builder := toolsy.NewRegistryBuilder()
	searchTool, err := rag.AsSearchTool(&myRetriever{}, rag.WithMaxBytes(256*1024))
	if err != nil {
		panic(err)
	}
	builder.Add(searchTool)
}
```
