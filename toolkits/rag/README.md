# Toolsy: RAG Toolkit (knowledge base search)

**Description:** Bridges any retriever (vector DB, ragy, etc.) to toolsy with structured `Document` DTOs and optional Markdown or JSON output. Retrieval routing,
aggregation, identity/deduplication, fallback and retries belong to the host.

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

// Compose routing in the host retriever, then hand one retriever to the adapter.
docs, err := (&myRetriever{}).Retrieve(ctx, "query")
if err != nil { return err }
md := rag.FormatDocumentsMarkdown(docs)
```

## Configuration

- **WithMaxBytes / WithMaxResults** — the byte budget applies to final wire JSON, including custom formatters. An oversized final representation returns `CodeValidationFailed`. Serialized JSON is never sliced. Zero limits select finite defaults (10 results); negative limits reject construction. The pipeline checks provider item/source bounds, applies `WithScopeFilter`, rechecks item/source bounds, then checks the filtered result count and final output bytes.
- **WithResultShape** — `ShapeMarkdown` (default) or `ShapeDocumentsJSON`.
- **WithScopeFilter** — RBAC hook to filter documents per request context.
- **WithResultFormatter / WithHostResultValidator** — host DTO and validation before JSON marshal. When both are set, the validator receives **formatter output**, not the default envelope. Validator-only with default `ShapeMarkdown` validates `SearchMarkdownWire` (`{"results": "..."}`). Use `WithResultShape(ShapeDocumentsJSON)` for `SearchDocumentsWire`.

## Quick start

```go
searchTool, err := rag.AsSearchTool(&myRetriever{}, rag.WithMaxBytes(256*1024))
```

See [docs/migration-task29.md](../../docs/migration-task29.md) for breaking changes from `Retriever` (`[]string`). See [docs/migration-task30.md](../../docs/migration-task30.md) for fail-closed wire budget checks (`CodeValidationFailed`).
D30/D36 removes the redundant wire pre-encode/shallow clone; one final encoded
representation is checked inclusively. See [task41 migration](../../docs/migration-task41.md).

## Task38 contract

The host maps its DTOs into retrieval units; `Document.ID` identifies an optional chunk, independent of `SourceURI`. The host chooses retrieval-unit identity and whether to deduplicate. Markdown includes source and ID even when content is present; unknown sources are labelled unavailable. `ID` is the host-selected public retrieval-unit identifier; it may encode all public document/chunk/page identifiers needed for citation. `Metadata` is opaque host data: Markdown does not interpret or publish its keys, including names such as `chunk_id`, `document_id`, or `page`. The host must map identifiers intended for publication into `ID` and `SourceURI` before retrieval units reach the formatter. `ShapeDocumentsJSON` includes the full supplied metadata; omit private metadata in the adapter or use a host formatter for a public DTO. Retrieval is data, never trusted instructions.

Negative budgets reject construction; zero budgets select finite defaults: 10 results, 64 KiB per JSON-encoded unit, 512 KiB total provider JSON and 512 KiB final wire JSON. `WithMaxItemBytes` and `WithMaxSourceBytes` configure the independent provider bounds. Provider bounds are enforced before host filters and formatters; count is enforced after filtering. Exceeding any limit returns a validation error without dropping units or slicing JSON. The retriever has no cursor capability, so no continuation token is fabricated. Host providers own retrieval allocations, cancellation and access control; the toolkit bounds accepted results, not provider internals. Custom output DTOs must preserve provenance themselves.

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

## Host routing and data ownership (D30/D36)

`Aggregate`, `Dedup`, `DedupBy` and `Fallback` are removed. `AsSearchTool` calls its
supplied `DocumentRetriever` at most once and never silently retries or falls back on an
error or empty result. Nil and typed-nil required retrievers are rejected at
construction. Provider errors remain inspectable through `errors.Is`;
cancellation/deadline is terminal. Once the RAG handler is entered, it checks
context before and after provider/filter/formatter/validator callbacks and preserves
both the standard interrupt and custom parent cause. An already-interrupted call
can instead be rejected by core before entering this handler: that existing core
boundary preserves the standard interrupt, but does not promise the custom cause.
No retriever is invoked in that case. Cooperative callbacks must still observe
their context; this toolkit does not preempt them.

The runnable [host recipe](examples/host/main.go) composes primary/secondary and
supplemental providers. It validates all required functions, uses an explicit
unavailable-index/empty-success fallback predicate, stops on cancellation, and
preserves both causes if primary and secondary fail. It merges results and dedups
by public source/chunk IDs selected by that host, preserving distinct chunks and
unidentified units. This example is host policy, not a replacement library router;
ragy/routery integration is host-specific and adds no dependency here.

```sh
GOWORK=off go run ./examples/host  # from toolkits/rag
```

Documents, slices and metadata are borrowed host data. Keep them stable while a
call runs and use read-only formatters/validators, or arrange explicit ownership
before mutating in a host callback. No universal/deep copy is performed. The old
JSON precheck's shallow slice clone did not protect nested metadata or arbitrary
host callback effects.

The pipeline is provider item/source bounds → scope filter → item/source/count
checks → host formatter or default envelope → host result validator → one final
JSON wire cap. Per-unit encoding remains necessary to account for escaped JSON
item/source bytes; the entire default envelope is not encoded twice. Host validators
run before the final wire check and must not rely on an earlier JSON-envelope cap
suppressing their invocation. Oversized output rejects the whole result with the
inspectable wire-limit cause; no document, ID or JSON bytes are sliced or omitted.
All accepted inputs/results are bounded, but provider and callback allocations,
CPU, access control and stable provenance remain host responsibilities.


Nil options reject construction. Host ports and callbacks are borrowed; the host
owns their lifetime and synchronization. See the [shared constructor and ownership
contract](../README.md#constructor-configuration-and-ownership) for option snapshots
and the distinction between configuration containers and mutable host ports.
