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

Library `ScrapePage` returns markdown directly (no JSON envelope). HTML is pre-capped via `textprocessor.ReadLimited` without a truncation suffix (`scrapeContentByteCap` derived from `WithMaxPageBytes`); `\n[Truncated]` appears only on the tool wire path via `format.CapWireJSON`.

Scrape uses `httptool.SafeDialTransport` for SSRF protection and DNS-rebinding pin. **Search HTTP egress** is entirely host-owned: implement `SearchProvider` with your own client; for untrusted URLs inject `httptool.NewSafeHTTPClient`.

## Available tools

| Tool         | Description                | Input                 |
| ------------ | -------------------------- | --------------------- |
| `web_search` | Run a search query         | `{"query": "string"}` |
| `web_scrape` | Fetch URL and get Markdown | `{"url": "string"}`   |

Result: search returns a Markdown list of links and snippets; scrape returns `{"markdown": "..."}`. Scraper strips script, style, noscript, iframe, nav, header, footer, aside, then converts HTML to Markdown. Byte budgets apply to **final wire JSON** (including JSON envelope overhead).

## Configuration & Security

> **Warning:** Scraping validates URLs: only http/https, host required. Private/loopback IPs are blocked unless `WithAllowPrivateIPs(true)` (tests only). Redirects are validated with the same rules and blocked domains. DNS rebinding is mitigated by pinning the connection to the resolved IP at dial time (`SafeDialTransport`); URL validation resolves IPs at validate time with the same `IsBlockedIP` policy.

- **WithMaxSearchBytes(n):** Cap `web_search` wire JSON (default 256KB). Applies to default and formatter paths. `SearchResultsTruncationSuffix` (50 hits) is a **semantic** cap, separate from the wire budget.
- **WithMaxPageBytes(n):** Cap `web_scrape` wire JSON (default 2MB). Raw HTML is pre-capped without `\n[Truncated]` before Markdown conversion; wire suffix applies to final JSON envelope only.
- **WithBlockedDomains(domains):** Blacklist of hostnames; exact match and subdomains are blocked (e.g. blocking `evil.com` blocks `api.evil.com`). Checked on initial URL and on redirects.
- **WithScraper(s):** Replace default HTML-to-Markdown scraper (e.g. for JS-rendered pages). Custom scrapers should respect caller context and bound CPU; only the default scraper cancels in-flight HTML conversion when the scrape context is done.
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
