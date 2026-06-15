# Toolsy: Document Toolkit (document)

**Description:** Lets the agent extract plain text from PDF, CSV, and DOCX files (by file path or optionally by URL) for LLM consumption. Transport reads are fail-closed; wire JSON may be capped separately via `format.CapWireJSON`.

## Installation

This toolkit is intended for use within the toolsy monorepo (see root `go.mod` and `replace` in this module). From the repo root, depend on the root module; do not publish or use this package as a standalone module.

**Dependencies:** `github.com/skosovsky/toolsy` (core), `github.com/skosovsky/toolsy/toolkits/httptool` (remote URL fetch), `github.com/ledongthuc/pdf` (PDF). CSV and DOCX use stdlib only.

## Available tools

| Tool                    | Description                                   | Input                                            |
| ----------------------- | --------------------------------------------- | ------------------------------------------------ |
| `document_extract_text` | Extract text from a document (PDF, CSV, DOCX) | `{"file_path": "string"}` or `{"url": "string"}` |

Result: `{"text": "..."}`. `WithMaxBytes` is the **final wire JSON** budget. Transport reads (local/remote) are fail-closed via `ReadLimitedBytes`; wire truncation adds `\n[Truncated]` once via `format.CapWireJSON`.

## Configuration & Security

> **Warning:** When `WithAllowRemote(true)` is used, the toolkit downloads from URLs. Remote fetch uses `httptool.NewSafeHTTPClient` with DNS-rebinding pin and `ValidateRemoteURL` / `IsBlockedIP` (IP-only — not `ValidateRemoteURLWithBlacklist`). Use `WithAllowPrivateIPs(true)` only for tests (e.g. httptest). Redirect validation uses the same `allowPrivateIPs` flag as the initial request. `WithHTTPClient` merges only `Timeout`.

Unlike `web`, remote fetch in `document` is **IP-only**: there is no host blacklist (`WithBlockedDomains` is not supported). Any public URL is allowed subject to private/loopback IP guards. Use `web` when domain blocklists are required.

- **Max bytes:** Use `WithMaxBytes(n)` as the **wire JSON** budget (default 2 MB). Local stat, remote download, and parsers use `contentByteCap = maxWire - envelopeOverhead` for fail-closed `ReadLimitedBytes` (envelope `{"text":"..."}` ≈ 16 bytes). Wire truncation via `format.CapWireJSON` may still apply separately. Download and DOCX unpacking are limited at I/O level to avoid zip bombs and OOM.
- **CSV transport tier:** `parser_csv.go` reads the full file with `ReadLimitedBytes` (fail-closed, no partial payload); markdown table conversion runs only after a successful bounded read.
- **Allow remote:** Use `WithAllowRemote(true)` to allow `url` input. When false, only `file_path` is accepted.
- **IoC:** `WithResultFormatter` / `WithHostResultValidator` validate the default `ExtractWireResult` envelope when no custom formatter is set.
- **Supported formats:** `.csv`, `.pdf`, `.docx` (by path/URL path or Content-Type for URL; query strings are ignored for extension).
- **PDF limits:** `os.Stat` pre-check plus page-level text extraction with a running byte budget (fail-closed per page); avoids full-document `GetPlainText()` allocation. Oversized PDFs return `CodeValidationFailed`, not silent truncation.

## Quick start

```go
package main

import (
	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/document"
)

func main() {
	builder := toolsy.NewRegistryBuilder()

	tool, err := document.AsTool(document.WithMaxBytes(1024 * 1024))
	if err != nil {
		panic(err)
	}
	builder.Add(tool)
}
```
