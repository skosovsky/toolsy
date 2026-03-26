# Toolsy: Web Toolkit (web)

**Description:** Lets the agent search the web (via SearchProvider) and scrape URLs to Markdown. Scraping strips script/style/noscript/iframe/nav/header/footer/aside and is SSRF-protected (private/loopback/unspecified 0.0.0.0/::/multicast blocked; blockedDomains include subdomains; redirect validation).

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/web
```

**Dependencies:** `github.com/skosovsky/toolsy`, `github.com/JohannesKaufmann/html-to-markdown/v2`. Search is via your SearchProvider only.

## Available tools

| Tool         | Description                    | Input             |
|--------------|--------------------------------|-------------------|
| `web_search` | Run a search query             | `{"query": "string"}` |
| `web_scrape` | Fetch URL and get Markdown     | `{"url": "string"}`   |

Result: search returns a Markdown list of links and snippets; scrape returns `{"markdown": "..."}`. Scraper strips script, style, noscript, iframe, nav, header, footer, aside, then converts HTML to Markdown (truncated to MaxPageBytes).

## Configuration & Security

> **Warning:** Scraping validates URLs: only http/https, host required. Private/loopback IPs are blocked unless `WithAllowPrivateIPs(true)` (tests only). Redirects are validated with the same rules and blocked domains. DNS rebinding is mitigated by pinning the connection to the resolved IP per request.

- **WithMaxPageBytes(n):** Cap scraped page size (default 2MB).
- **WithBlockedDomains(domains):** Blacklist of hostnames; exact match and subdomains are blocked (e.g. blocking `evil.com` blocks `api.evil.com`). Checked on initial URL and on redirects.
- **WithScraper(s):** Replace default HTML-to-Markdown scraper (e.g. for JS-rendered pages).
- **WithAllowPrivateIPs(true):** For tests with httptest on 127.0.0.1 only.

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
