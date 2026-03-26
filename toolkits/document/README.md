# Toolsy: Document Toolkit (document)

**Description:** Lets the agent extract plain text from PDF, CSV, and DOCX files (by file path or optionally by URL) for LLM consumption. Output is truncated to a configurable size.

## Installation

This toolkit is intended for use within the toolsy monorepo (see root `go.mod` and `replace` in this module). From the repo root, depend on the root module; do not publish or use this package as a standalone module.

**Dependencies:** `github.com/skosovsky/toolsy` (core), `github.com/ledongthuc/pdf` (PDF). CSV and DOCX use stdlib only.

## Available tools

| Tool                   | Description                                  | Input                                              |
|------------------------|----------------------------------------------|----------------------------------------------------|
| `document_extract_text`| Extract text from a document (PDF, CSV, DOCX)| `{"file_path": "string"}` or `{"url": "string"}`   |

Result: `{"text": "..."}`. Text is truncated to `maxBytes` (default 2 MB) with `[Truncated]` suffix when longer.

## Configuration & Security

> **Warning:** When `WithAllowRemote(true)` is used, the toolkit downloads from URLs. Built-in SSRF protection blocks loopback, link-local, and private IPs; use `WithAllowPrivateIPs(true)` only for tests (e.g. httptest). Redirects are validated: redirect to private/loopback is always rejected (even when `WithAllowPrivateIPs(true)` is set). You can tighten policy further with a custom `http.Client` via `WithHTTPClient` (e.g. allowed domains).

- **Max bytes:** Use `WithMaxBytes(n)` to limit file size and output (default 2 MB). Download and DOCX unpacking are limited at I/O level to avoid zip bombs and OOM.
- **Allow remote:** Use `WithAllowRemote(true)` to allow `url` input. When false, only `file_path` is accepted.
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
