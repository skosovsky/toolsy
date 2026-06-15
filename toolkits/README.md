# Toolsy Toolkits

Enterprise-ready building blocks for AI agents: each module supports **library mode** (pure functions/DTOs) and **tool mode** (`AsTools` / `AsTool` factories).

## Dual mode

| Module     | Library API                            | Tool factory   |
| ---------- | -------------------------------------- | -------------- |
| `timetool` | `ComputeCurrent`, `CalculateResult`    | `AsTools`      |
| `httptool` | `SafeDialTransport`, `ReadBodyLimited` | `AsTools`      |
| `web`      | `SearchStructured`, `ScrapePage`       | `AsTools`      |
| `rag`      | `Document`, router primitives          | `AsSearchTool` |
| `sqltool`  | `InspectResult`, `ExecuteResult`       | `AsTools`      |
| `document` | `ExtractWireResult`                    | `AsTool`       |

## IoC: custom output shape

Host applications can inject formatters returning `any` (domain DTOs). Toolsy serializes the result to JSON on the wire.

```go
timetool.AsTools(
    timetool.WithResultFormatter(func(r timetool.CurrentResult) (any, error) {
        return map[string]string{"ts": r.UTC}, nil
    }),
    timetool.WithHostResultValidator(func(v any) error {
        // PII / injection checks before marshal
        return nil
    }),
)
```

Supported in: `timetool` (`WithResultFormatter`, `WithCalculateResultFormatter`), `web` (`WithSearchFormatter`, `WithScrapeFormatter`), `rag`, `sqltool` (`WithExecuteResultFormatter`, `WithInspectResultFormatter`), `document`. `fstool` is tool-mode only (no library exports).

When a byte budget option is set (`WithMaxBytes`, `WithMaxPageBytes`, `WithMaxSearchBytes`, `WithMaxSchemaBytes`, sqltool row/cell limits), it applies to **final wire JSON** (`github.com/skosovsky/toolsy/internal/format` — `MarshalWireCap` / `ApplyWithEnvelope` + `CapWireJSON`), including default tool paths without a formatter. **Exception:** `httptool` probe (`http_get` / `http_post`) caps only the **body field** via `ReadBodyLimited`, not `CapWireJSON` — see Probe tier below.

### Byte budget and suffix taxonomy

| Tier           | Where                                                   | Suffix                                                                                    | Notes                                                                            |
| -------------- | ------------------------------------------------------- | ----------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| **Wire**       | `internal/format.CapWireJSON` on tool paths             | `\n[Truncated]` (`textprocessor.TruncationSuffix`)                                        | One user-visible suffix per tool response                                        |
| **Semantic**   | sqltool rows/cells, web search hit list                 | `SQLRowsTruncationSuffix`, `SQLCellTruncationSuffix`, `SearchResultsTruncationSuffix`     | Domain limits (rows, cells, hits); independent of wire cap                       |
| **DoS / read** | HTML scrape, httptool probe, document/fstool wire tools | fail-closed (`ReadLimitedBytes` / `ReadBodyLimited`); display tier uses `ReadAndTruncate` | Memory safety; probe tier uses body-field budget; envelope tools use content cap |
| **Probe**      | `httptool.AsTools` DTO                                  | limit → validation error in tool mode                                                     | Status in JSON; separate from toolkit wire cap                                   |

Content pre-cap: `document` parsers and `rag` JSON shape (`capDocumentsForWire`) fail-closed when results cannot fit the byte budget; wire marshal may still add `\n[Truncated]` via `internal/format.CapWireJSON`.

### Default transport read budgets (toolkits)

| Package                          | Default         | API                                                                |
| -------------------------------- | --------------- | ------------------------------------------------------------------ |
| `document` / `web` scrape        | 2 MB            | `WithMaxBytes` / `WithMaxPageBytes`                                |
| `fstool` read                    | 1 MB            | `WithMaxBytes`                                                     |
| `httptool` probe                 | 512 KB          | `defaultMaxResponseBody` (body field; wire JSON slightly larger)   |
| `mail` body                      | 256 KB          | `defaultMaxBodyBytes` (display/wire truncate after fetch)          |
| `prompts` output                 | toolkit default | display/wire truncate after template render                        |
| `agents` REST (`agents` package) | 4 MB            | `defaultMaxResponseBytes` — library client, not a toolkit wire DTO |

These are independent of contracts spec/introspection budgets (see [`contracts/README.md`](../contracts/README.md)). The `agents` REST client uses fail-closed `ReadLimitedBytes`; see [`agents/README.md`](../agents/README.md).

Cross-package validators can type-assert exported wire DTOs (`CalculateResult`, `SearchWireResult`, `SearchMarkdownWire`, etc.).

### Validator vs formatter priority

| Module                                      | Formatter set                    | Validator receives                                   |
| ------------------------------------------- | -------------------------------- | ---------------------------------------------------- |
| `rag` (default markdown)                    | no                               | `SearchMarkdownWire`                                 |
| `rag` (+ formatter or `ShapeDocumentsJSON`) | yes                              | formatter output or `SearchDocumentsWire`            |
| `web` search                                | `[]SearchResult` to formatter    | `SearchWireResult` envelope only when validator-only |
| `web` scrape                                | `string` (markdown) to formatter | `ScrapeWireResult` envelope only when validator-only |
| `timetool` / `sqltool`                      | typed DTO to formatter           | default envelope only when validator-only            |

When both formatter and validator are set, validation runs on the formatter return value (`ApplyWithEnvelope` order).

## Wrapper pattern

Wrap toolkit tools with host middleware **before** registering:

```go
builder := toolsy.NewRegistryBuilder()
tools, _ := httptool.AsTools(httptool.WithAllowedDomains([]string{"api.example.com"}))
for _, t := range tools {
    builder.Add(hostMiddleware.Wrap(t)) // audit, rate limit, PII filter
}
```

See [`examples/resiliency`](../../examples/resiliency/main.go) for core toolsy middleware patterns.

## SSRF

HTTP egress protection is centralized in `httptool` (`SafeDialTransport`, `NewSafeHTTPClient`, `ValidateRemoteURL`, `CheckRedirectRemote`). Only modules with **HTTP egress** depend on `httptool`:

| Consumer    | Uses httptool for                                  |
| ----------- | -------------------------------------------------- |
| `web`       | scrape client, redirect validation                 |
| `document`  | remote URL fetch (IP-only; no host blacklist)      |
| `agents`    | REST client, SSE stream steps                      |
| `contracts` | OpenAPI/GraphQL execute and spec fetch             |
| `mcp`       | SSE GET/POST (`WithSSEHTTPClient`, URL validation) |
| `httptool`  | `AsTools` HTTP GET                                 |

Leaf modules without HTTP (`fstool`, `sqltool`, `timetool`, `rag`, `mail`, …) must **not** import `httptool`. Local file reads use `textprocessor` (see `fstool.readFileLimited`).

`ReadBodyLimited` is an HTTP response primitive; do not use it for filesystem reads.

```mermaid
flowchart TB
  httptool[httptool]
  textprocessor[textprocessor]
  web[web]
  document[document]
  fstool[fstool]
  httptool --> web
  httptool --> document
  textprocessor --> fstool
  textprocessor --> httptool
```

Host HTTP clients should reuse `SafeDialTransport`. `web` scrape and `document` remote fetch delegate to `httptool.NewSafeHTTPClient`.
