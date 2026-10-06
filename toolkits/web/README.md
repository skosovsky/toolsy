# Toolsy: Web Toolkit (web)

**Description:** Lets the agent search the web (via SearchProvider) and scrape URLs to Markdown. Scraping strips script/style/noscript/iframe/nav/header/footer/aside and is SSRF-protected (private/loopback/unspecified 0.0.0.0/::/multicast blocked; blockedDomains include subdomains; redirect validation).

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/web
```

**Dependencies:** `github.com/skosovsky/toolsy`, `github.com/skosovsky/toolsy/toolkits/httptool`, `github.com/JohannesKaufmann/html-to-markdown/v2`. Search is via your SearchProvider only.

## Library mode

```go
results, err := web.SearchStructured(ctx, provider, "query")
md, err := web.ScrapePage(ctx, "https://example.com", web.WithAllowPrivateIPs(true)) // tests only
```

Library `ScrapePage` returns markdown directly (no JSON envelope). HTML read uses fail-closed `textprocessor.ReadLimitedBytes`; markdown conversion uses a separate **semantic cap** (`ErrMarkdownExceedsLimit` — check with `web.IsMarkdownExceedsLimit`, not `textprocessor.IsReadLimitExceeded`). Oversized HTML or markdown return an error — raise `WithMaxPageBytes` for larger budgets.

Scrape uses `httptool.SafeDialTransport` for SSRF protection and DNS-rebinding pin. **Search HTTP egress** is entirely host-owned: implement `SearchProvider` with your own client; for untrusted URLs inject `httptool.NewSafeHTTPClient`.

## Available tools

| Tool         | Description                | Input                 |
| ------------ | -------------------------- | --------------------- |
| `web_search` | Run a search query         | `{"query": "string"}` |
| `web_scrape` | Fetch URL and get Markdown | `{"url": "string"}`   |

Result: search returns a Markdown list of links and snippets; scrape returns `{"markdown": "...", "source_url": "..."}`. Scraper strips script, style, noscript, iframe, nav, header, footer, aside, then converts HTML to Markdown. Byte budgets apply to **final wire JSON** (including JSON envelope overhead).

## Configuration & Security

> **Warning:** Scraping validates URLs: only http/https, host required. Private/loopback IPs are blocked unless `WithAllowPrivateIPs(true)` (tests only). Redirects are validated with the same rules and blocked domains. DNS rebinding is mitigated by pinning the connection to the resolved IP at dial time (`SafeDialTransport`); URL validation resolves IPs at validate time with the same `IsBlockedIP` policy.

- **WithMaxSearchBytes(n):** Cap `web_search` wire JSON (default 256KB). Applies to default and formatter paths. Search provider count/item/source bounds are separate from this wire budget.
- **WithMaxPageBytes(n):** Cap `web_scrape` wire JSON (default 2MB). HTML read and markdown conversion are fail-closed; oversized HTML or expanded markdown return `CodeValidationFailed` — raise `WithMaxPageBytes` for larger budgets. Oversized final JSON returns `CodeValidationFailed`; serialized JSON is never sliced.
- **WithBlockedDomains(domains):** Blacklist of hostnames; exact match and subdomains are blocked (e.g. blocking `evil.com` blocks `api.evil.com`). Checked on initial URL and on redirects.
- **WithScraper(s):** Replace default HTML-to-Markdown scraper (e.g. for JS-rendered pages). Custom scrapers must enforce `maxBytes` fail-closed in `HTMLToMarkdown(ctx, html, maxBytes)` (error when markdown exceeds cap; no silent truncate). They should respect caller context and bound CPU; the default scraper checks cancellation before and after synchronous HTML conversion. Use `WrapMarkdownExceedsLimit` for custom scraper cap errors.
- **WithAllowPrivateIPs(true):** For tests with httptest on 127.0.0.1 only.
- **IoC:** `WithSearchFormatter`, `WithScrapeFormatter`, and `WithHostResultValidator`. Validator-only mode validates `SearchWireResult` / `ScrapeWireResult` wire envelopes (not raw slices/strings).

## Quick start

```go
package main

import (
	"context"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/web"
)

// Minimal in-memory SearchProvider for a compilable example.
type inMemorySearch struct{}
func (inMemorySearch) Search(ctx context.Context, query string) ([]web.SearchResult, error) {
	return nil, nil
}

func main() {
	builder := toolsy.NewRegistryBuilder()
	tools, err := web.AsTools(inMemorySearch{})
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		builder.Add(tool)
	}
}
```

## Task38 contract

Extracted HTML and search snippets are untrusted data. Removing tags does not make their text safe instructions. Default scrape output carries `source_url` (the actual final response URL). `WithScrapeFormatter` receives the complete `ScrapeWireResult`, allowing host DTOs to retain provenance.

Nonpositive budgets select finite defaults. Search accepts at most 50 results (`WithMaxSearchResults`), 16 KiB per encoded hit (`WithMaxSearchItemBytes`), and 256 KiB total encoded provider hits (`WithMaxSearchSourceBytes`), independently of the final wire budget. Overlimit provider results fail before formatters; no hits disappear silently and no cursor is invented because `SearchProvider` has no continuation capability. Providers own their internal allocations and network policy. Scraping separates HTML source (`WithMaxSourceBytes`, default final wire budget minus 18 bytes), extracted markdown (`WithMaxMarkdownBytes`, same default), and final JSON (`WithMaxPageBytes`). Actual custom scraper output is checked again by the toolkit. The default parser uses bounded input but does not promise a hard allocation or CPU quota; hostile parsing requiring such guarantees belongs in a host sandbox. Final JSON budgets include escaping and provenance.
