# Migration guide: Task29 Enterprise Toolkits

## Breaking changes

### `rag.Retriever` removed

**Before (task28):**

```go
type Retriever interface {
    Retrieve(ctx context.Context, query string) ([]string, error)
}
```

**After (task29):**

```go
type DocumentRetriever interface {
    Retrieve(ctx context.Context, query string) ([]Document, error)
}

type Document struct {
    Content   string
    SourceURI string
    Category  string
    Metadata  map[string]string
}
```

Migrate retriever implementations to return `[]Document`. Markdown formatting is optional via `FormatDocumentsMarkdown` or default tool output.

### `timetool.currentResult` → exported `CurrentResult`

Use `timetool.ComputeCurrent(loc)` in library mode. Tool JSON shape unchanged unless `WithResultFormatter` is set.

## New library APIs

| Module     | API                                                                                     |
| ---------- | --------------------------------------------------------------------------------------- |
| `httptool` | `IsPrivateIP`, `IsBlockedIP`, `SafeDialTransport`, `ReadBodyLimited`, `IsSuccessStatus` |
| `timetool` | `ComputeCurrent`, `CurrentResult`                                                       |
| `web`      | `SearchStructured`, `ScrapePage`, `FormatSearchMarkdown`                                |
| `rag`      | `Aggregate`, `Dedup`, `Fallback`, `FormatDocumentsMarkdown`                             |

## IoC formatters

All text/JSON toolkits support host-controlled output shaping:

| Module     | Formatter option                                           | Validator option          |
| ---------- | ---------------------------------------------------------- | ------------------------- |
| `timetool` | `WithResultFormatter`, `WithCalculateResultFormatter`      | `WithHostResultValidator` |
| `web`      | `WithSearchFormatter`, `WithScrapeFormatter`               | `WithHostResultValidator` |
| `rag`      | `WithResultFormatter`                                      | `WithHostResultValidator` |
| `sqltool`  | `WithExecuteResultFormatter`, `WithInspectResultFormatter` | `WithHostResultValidator` |
| `document` | `WithResultFormatter`                                      | `WithHostResultValidator` |

Validator-only mode validates the default tool wire envelope (exported types: `web.SearchWireResult`, `web.ScrapeWireResult`, `rag.SearchMarkdownWire`, `rag.SearchDocumentsWire`, `timetool.CurrentResult`, `timetool.CalculateResult`, `sqltool.InspectResult`, `sqltool.ExecuteResult`, `document.ExtractWireResult`), not raw slices/strings. Use `github.com/skosovsky/toolsy/internal/format.ApplyWithEnvelope` when adding new toolkits.

When a byte budget is configured, `ApplyWithEnvelope` caps **final wire JSON** via `format.CapWireJSON` (including after custom formatters).

**RAG validator-only:** default `ShapeMarkdown` validates `SearchMarkdownWire` (`{"results": "..."}`). Use `WithResultShape(ShapeDocumentsJSON)` for `SearchDocumentsWire`.

## HTTP primitives tier (outside toolkits)

Modules with outbound HTTP should reuse `httptool` library primitives (`NewSafeHTTPClient`, `SafeDialTransport`, `ReadBodyLimited`), not `http.DefaultClient`:

| Module                                   | Default client                     | Notes                                                                                                                             |
| ---------------------------------------- | ---------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `toolkits/web` scrape                    | `httptool` safe stack              | Search HTTP is host-owned via `SearchProvider`                                                                                    |
| `toolkits/document` remote               | `httptool` safe stack              | IP-only (no host blacklist)                                                                                                       |
| `agents`                                 | `httptool.NewSafeHTTPClient`       | `MergeHTTPClient` for custom timeout; bounded via `ReadLimitedBytes`                                                              |
| `contracts/openapi`, `contracts/graphql` | safe client + merge                | Execute: `ReadAndTruncate`; spec/introspection: `ReadLimitedBytes` (fail-closed). See [migration-task30.md](migration-task30.md). |
| `mcp` SSE                                | `httptool` via `WithSSEHTTPClient` | Long-lived stream (`Timeout: 0`); `ValidateRemoteURL` on GET/POST; `WithSSEAllowPrivateIPs` for tests                             |

Custom `*http.Client` values merge **Timeout only**; Transport always comes from the SSRF-safe default.

### Response drain tier

After bounded reads, unread response tails are drained with `httptool.DrainResponseBody` (default cap 64 KiB) or `CloseResponseBody` before close. Non-OK responses drain before close to preserve keep-alive.

All `contracts/openapi` and `contracts/graphql` HTTP paths use `CloseResponseBody` and check HTTP status **before** reading the body (execute, spec fetch, introspection, GraphQL tool execute).

### Body-read tier

> **Superseded by [migration-task30.md](migration-task30.md)** for read I/O contracts (fail-closed transport, `ErrReadLimitExceeded`, opt-in `ReadAndTruncate`).

| Path                                      | API (post-task30)                               | ctx-aware |
| ----------------------------------------- | ----------------------------------------------- | --------- |
| toolkits HTTP (`httptool` probe)          | `ReadBodyLimited` → `[]byte` fail-closed        | yes       |
| `web` scrape                              | `ReadLimitedBytes` → error on exceed            | yes       |
| `fstool` / local files                    | `ReadLimitedBytes` → validation in tool         | yes       |
| contracts execute                         | `ReadAndTruncate` + `ContractsTruncationSuffix` | yes       |
| contracts spec/introspection, agents REST | `ReadLimitedBytes` (hard cap)                   | yes       |
| `document` remote download                | `ReadLimitedBytes`                              | yes       |
| `contracts/grpc`                          | `TruncateBytesToValidUTF8String` on bytes       | n/a       |

Semantic suffixes (`ContractsTruncationSuffix`, `SQLRowsTruncationSuffix`, `SearchResultsTruncationSuffix`, etc.) apply on display/wire tiers — not on transport read primitives.

### Wire byte budget (tool paths)

`WithMax*Bytes` options on rag, web, document, sqltool inspect set the **final wire JSON** size. Transport reads are fail-closed; `format.CapWireJSON` may add `\n[Truncated]` once on the wire envelope. Semantic row/cell caps on execute remain separate from wire budget.

`httptool` probe tools return `CodeValidationFailed` when the response exceeds `maxResponseBody` (no silent body truncate). Library `web.ScrapePage` uses fail-closed HTML read; raise budget with `WithMaxPageBytes`.

### HTTP success status

Outbound fetch tiers (`web` scrape, `document` remote, `contracts/openapi` spec fetch and execute, `contracts/graphql` introspection and execute, `agents` REST, MCP SSE GET, agents SSE stream open) use `httptool.IsSuccessStatus` (any **2xx**), not `http.StatusOK` only. Partial whitelists (e.g. 200|201) are removed in favor of the shared helper.

### Blocking I/O and ctx (best-effort)

| Path                       | Notes                                                                                              |
| -------------------------- | -------------------------------------------------------------------------------------------------- |
| `document` PDF/DOCX/CSV    | Parsers poll `ctx`; PDF `Open`/`GetPlainText` use goroutine + select (library has no cancel API)   |
| `web` scrape HTML→Markdown | Default scraper cancels in-flight `ConvertString` on ctx done; custom `WithScraper` must bound CPU |
| `mail` HTML normalize      | `normalizeBody` cancels in-flight conversion on ctx done (falls back to raw HTML)                  |

Out of scope: `mail` / `prompts` content-only caps; `timetool` IoC `maxWireBytes=0`; `contracts/grpc` `ContractsTruncationSuffix` tier.

### SSE / stdio stream tier

| Transport        | Per-line cap                         | Total stream cap                                     |
| ---------------- | ------------------------------------ | ---------------------------------------------------- |
| agents SSE steps | 1 MiB (`maxSSEScanBytes`)            | 16 MiB default (`httptool.DefaultMaxSSEStreamBytes`) |
| mcp SSE          | 1 MiB (`rpcJSONLineScannerMaxBytes`) | 16 MiB default; `WithSSEMaxStreamBytes`              |
| mcp stdio        | 1 MiB per JSON line                  | 16 MiB default; `WithStdioMaxStreamBytes`            |

Long-lived streams respect context cancellation in read loops (mcp SSE GET, mcp stdio stdout). MCP SSE GET/POST/notify and agents REST use `httptool.IsSuccessStatus` (2xx) before reading or draining the body.

## SSRF unification

- `httptool.AsTools` uses `SafeDialTransport` + `CheckRedirectAllowed` by default.
- `web` scrape uses `httptool.NewSafeHTTPClient` + `ValidateRemoteURLWithBlacklist` + `CheckRedirectRemote`.
- `document` remote fetch uses `httptool.NewSafeHTTPClient` + `CheckRedirectRemote` (**IP-only** — no host blacklist; unlike `web`).
- `fstool` reads local files via `textprocessor`; it does **not** depend on `httptool`.
- `WithAllowPrivateIPs(true)` in `web`/`document`/`httptool` keeps `SafeDialTransport` and only relaxes IP validation (never `http.DefaultClient`).
- Host HTTP clients should use the same transport with `SafeDialOptions`:
  - Non-empty `AllowedHosts` → strict whitelist (fail-closed on Allowed+Blocked overlap).
  - Empty `AllowedHosts` → blacklist via `BlockedHosts` + `IsBlockedIP`.

## Wrapper pattern

Wrap tools before `RegistryBuilder.Add` for audit, rate limits, or PII filtering. See `toolkits/README.md` and `examples/resiliency`.
