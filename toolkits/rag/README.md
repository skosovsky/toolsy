# Toolsy: RAG Toolkit (knowledge base search)

**Description:** Bridges any retriever (vector DB, ragy, etc.) to toolsy with structured `Document` DTOs, router primitives, and optional Markdown or JSON output.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/rag
```

**Dependencies:** requires `github.com/skosovsky/toolsy` (core). Implement `DocumentRetriever` and pass it to `AsSearchTool`.

## Available tools

| Tool                    | Description                               | Input                 |
| ----------------------- | ----------------------------------------- | --------------------- |
| `search_knowledge_base` | Search the knowledge base for information | `{"query": "string"}` |

Default output: Markdown `{"results": "1. ...\n2. ..."}`. Use `WithResultShape(ShapeDocumentsJSON)` for `{"documents": [...]}`.

## Library mode

```go
type myRetriever struct{}

func (m *myRetriever) Retrieve(ctx context.Context, query string) ([]rag.Document, error) {
    return []rag.Document{{Content: "answer", SourceURI: "doc://1"}}, nil
}

router := rag.Dedup(rag.Fallback(primary, secondary))
docs, _ := router.Retrieve(ctx, "query")
md := rag.FormatDocumentsMarkdown(docs)
```

## Configuration

- **WithMaxBytes / WithMaxResults** — byte budget applies to **final wire JSON** (including when `WithResultFormatter` is set). JSON shape pre-cap (`capDocumentsForWire`) trims document content without `\n[Truncated]`; wire suffix is applied once by `format.CapWireJSON`. `WithMaxResults(0)` means no limit (default when unset: 10). `WithScopeFilter` runs before `maxResults`.
- **WithResultShape** — `ShapeMarkdown` (default) or `ShapeDocumentsJSON`.
- **WithScopeFilter** — RBAC hook to filter documents per request context.
- **WithResultFormatter / WithHostResultValidator** — host DTO and validation before JSON marshal. When both are set, the validator receives **formatter output**, not the default envelope. Validator-only with default `ShapeMarkdown` validates `SearchMarkdownWire` (`{"results": "..."}`). Use `WithResultShape(ShapeDocumentsJSON)` for `SearchDocumentsWire`.

## Quick start

```go
searchTool, err := rag.AsSearchTool(&myRetriever{}, rag.WithMaxBytes(256*1024))
```

See [docs/migration-task29.md](../../docs/migration-task29.md) for breaking changes from `Retriever` (`[]string`).
