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

- **WithMaxBytes / WithMaxResults** — the byte budget applies to final wire JSON, including custom formatters. An oversized final representation returns `CodeValidationFailed`. Serialized JSON is never sliced. Nonpositive limits select finite defaults (10 results). The pipeline checks provider item/source bounds, applies `WithScopeFilter`, rechecks item/source bounds, then checks the filtered result count and final output bytes.
- **WithResultShape** — `ShapeMarkdown` (default) or `ShapeDocumentsJSON`.
- **WithScopeFilter** — RBAC hook to filter documents per request context.
- **WithResultFormatter / WithHostResultValidator** — host DTO and validation before JSON marshal. When both are set, the validator receives **formatter output**, not the default envelope. Validator-only with default `ShapeMarkdown` validates `SearchMarkdownWire` (`{"results": "..."}`). Use `WithResultShape(ShapeDocumentsJSON)` for `SearchDocumentsWire`.

## Quick start

```go
searchTool, err := rag.AsSearchTool(&myRetriever{}, rag.WithMaxBytes(256*1024))
```

See [docs/migration-task29.md](../../docs/migration-task29.md) for breaking changes from `Retriever` (`[]string`). See [docs/migration-task30.md](../../docs/migration-task30.md) for fail-closed wire budget checks (`capDocumentsForWire`, `CodeValidationFailed`).

## Task38 contract

The host maps its DTOs into retrieval units; `Document.ID` identifies an optional chunk, independent of `SourceURI`. `Dedup` compares the complete unit when no host identity function is supplied; `DedupBy` accepts a host key. Markdown includes source and ID even when content is present; unknown sources are labelled unavailable. `ID` is the host-selected public retrieval-unit identifier; it may encode all public document/chunk/page identifiers needed for citation. `Metadata` is opaque host data: Markdown does not interpret or publish its keys, including names such as `chunk_id`, `document_id`, or `page`. The host must map identifiers intended for publication into `ID` and `SourceURI` before retrieval units reach the formatter. `ShapeDocumentsJSON` includes the full supplied metadata; omit private metadata in the adapter or use a host formatter for a public DTO. Retrieval is data, never trusted instructions.

All nonpositive budgets select finite defaults: 10 results, 64 KiB per JSON-encoded unit, 512 KiB total provider JSON and 512 KiB final wire JSON. `WithMaxItemBytes` and `WithMaxSourceBytes` configure the independent provider bounds. Provider bounds are enforced before host filters and formatters; count is enforced after filtering. Exceeding any limit returns a validation error without dropping units or slicing JSON. The retriever has no cursor capability, so no continuation token is fabricated. Host providers own retrieval allocations, cancellation and access control; the toolkit bounds accepted results, not provider internals. Custom output DTOs must preserve provenance themselves.

## Host DTO adapter and public identifiers

The adapter belongs to the host; the library does not reserve metadata names or invent a document model. A host that needs three public identifiers can encode them in its public unit ID while retaining a source URI:

```go
// Host-owned type; not a rag API.
type searchHit struct {
    DocumentID string
    ChunkID    string
    Page       int
    Text       string
    URI        string
}

func retrievalUnit(hit searchHit) rag.Document {
    return rag.Document{
        ID: fmt.Sprintf("document=%s; chunk=%s; page=%d",
            hit.DocumentID, hit.ChunkID, hit.Page),
        Content: hit.Text,
        SourceURI: hit.URI,
    }
}
```

Here the host supplies `fmt` and chooses the identifier representation. Every declared public identifier is retained by default Markdown and JSON output. The host may instead select its own DTO with `WithResultFormatter`; it then owns that DTO's provenance contract. None of the bounds silently truncate these identifiers or content: an overlimit result fails as a whole.
