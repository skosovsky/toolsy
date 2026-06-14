# Toolsy: Document Toolkit (document)

**Description:** Lets the agent extract plain text from PDF, CSV, and DOCX files (by file path or optionally by URL) for LLM consumption. Output is truncated to a configurable size.

## Installation

This toolkit is intended for use within the toolsy monorepo (see root `go.mod` and `replace` in this module). From the repo root, depend on the root module; do not publish or use this package as a standalone module.

**Dependencies:** `github.com/skosovsky/toolsy` (core), `github.com/skosovsky/toolsy/toolkits/httptool` (remote URL fetch), `github.com/ledongthuc/pdf` (PDF). CSV and DOCX use stdlib only.

## Available tools

| Tool                    | Description                                   | Input                                            |
| ----------------------- | --------------------------------------------- | ------------------------------------------------ |
| `document_extract_text` | Extract text from a document (PDF, CSV, DOCX) | `{"file_path": "string"}` or `{"url": "string"}` |

Result: `{"text": "..."}`. `WithMaxBytes` is the **final wire JSON** budget; parser content is pre-capped without `\n[Truncated]` and wire truncation adds the suffix once via `format.CapWireJSON`.

## Configuration & Security

> **Warning:** When `WithAllowRemote(true)` is used, the toolkit downloads from URLs. Remote fetch uses `httptool.NewSafeHTTPClient` with DNS-rebinding pin and `ValidateRemoteURL` / `IsBlockedIP` (IP-only — not `ValidateRemoteURLWithBlacklist`). Use `WithAllowPrivateIPs(true)` only for tests (e.g. httptest). Redirect validation uses the same `allowPrivateIPs` flag as the initial request. `WithHTTPClient` merges only `Timeout`.

Unlike `web`, remote fetch in `document` is **IP-only**: there is no host blacklist (`WithBlockedDomains` is not supported). Any public URL is allowed subject to private/loopback IP guards. Use `web` when domain blocklists are required.

- **Max bytes:** Use `WithMaxBytes(n)` as the wire JSON budget (default 2 MB). File stat and remote download use the same limit; parser content uses `maxBytes - envelopeOverhead` without a truncation suffix. Download and DOCX unpacking are limited at I/O level to avoid zip bombs and OOM.
- **Allow remote:** Use `WithAllowRemote(true)` to allow `url` input. When false, only `file_path` is accepted.
- **IoC:** `WithResultFormatter` / `WithHostResultValidator` validate the default `ExtractWireResult` envelope when no custom formatter is set.
- **Supported formats:** `.csv`, `.pdf`, `.docx` (by path/URL path or Content-Type for URL; query strings are ignored for extension).

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
