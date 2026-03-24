package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/skosovsky/toolsy"
)

// maxSearchResultsDisplayed is the maximum number of search hits included in the markdown list (before truncation).
const maxSearchResultsDisplayed = 50

// SearchResult is a single search hit from SearchProvider.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchProvider is implemented by the orchestrator (Tavily, DuckDuckGo, Google Custom Search, etc.).
type SearchProvider interface {
	Search(ctx context.Context, query string) ([]SearchResult, error)
}

type searchArgs struct {
	Query string `json:"query"`
}

type searchResult struct {
	Results string `json:"results"`
}

type scrapeArgs struct {
	URL string `json:"url"`
}

type scrapeResult struct {
	Markdown string `json:"markdown"`
}

// AsTools returns web_search and web_scrape tools. SearchProvider is required for web_search.
func AsTools(provider SearchProvider, opts ...Option) ([]toolsy.Tool, error) {
	if provider == nil {
		return nil, errors.New("toolkit/web: SearchProvider is required")
	}
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)

	searchTool, err := toolsy.NewTool[searchArgs, searchResult](
		o.searchName,
		o.searchDesc,
		func(ctx context.Context, args searchArgs) (searchResult, error) {
			return doSearch(ctx, provider, args.Query)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/web: build search tool: %w", err)
	}

	scrapeTool, err := toolsy.NewTool[scrapeArgs, scrapeResult](
		o.scrapeName,
		o.scrapeDesc,
		func(ctx context.Context, args scrapeArgs) (scrapeResult, error) {
			return doScrape(ctx, &o, args.URL)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/web: build scrape tool: %w", err)
	}

	return []toolsy.Tool{searchTool, scrapeTool}, nil
}

func doSearch(ctx context.Context, provider SearchProvider, query string) (searchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return searchResult{}, &toolsy.ClientError{
			Reason:    "query is required",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	results, err := provider.Search(ctx, query)
	if err != nil {
		return searchResult{}, fmt.Errorf("toolkit/web: search: %w", err)
	}
	var b strings.Builder
	for i, r := range results {
		b.WriteString("- **")
		b.WriteString(escapeMarkdown(r.Title))
		b.WriteString("**: ")
		b.WriteString(r.URL)
		if r.Snippet != "" {
			b.WriteString(" — ")
			b.WriteString(escapeMarkdown(r.Snippet))
		}
		b.WriteString("\n")
		if i+1 == maxSearchResultsDisplayed {
			b.WriteString("... [truncated]\n")
			break
		}
	}
	return searchResult{Results: strings.TrimSuffix(b.String(), "\n")}, nil
}

func doScrape(ctx context.Context, o *options, rawURL string) (scrapeResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return scrapeResult{}, &toolsy.ClientError{
			Reason:    "url is required",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	u, err := validateScrapeURL(ctx, rawURL, o.allowPrivateIPs, o.blockedDomains)
	if err != nil {
		return scrapeResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return scrapeResult{}, fmt.Errorf("toolkit/web: new request: %w", err)
	}
	client := *o.httpClient
	client.CheckRedirect = checkRedirect(o.allowPrivateIPs, o.blockedDomains)
	if !o.allowPrivateIPs {
		// Pin to resolved IP per request to prevent DNS rebinding
		if client.Transport == nil {
			client.Transport = &http.Transport{DialContext: rebindingSafeDialContext}
		} else if t, ok := client.Transport.(*http.Transport); ok {
			t2 := t.Clone()
			t2.DialContext = rebindingSafeDialContext
			client.Transport = t2
		} else {
			client.Transport = &http.Transport{DialContext: rebindingSafeDialContext}
		}
	}
	// URL validated by validateScrapeURL; redirects validated by CheckRedirect with same blockedDomains
	resp, err := client.Do(req) // #nosec G704
	if err != nil {
		if toolsy.IsClientError(err) {
			return scrapeResult{}, err
		}
		return scrapeResult{}, fmt.Errorf("toolkit/web: fetch: %w", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return scrapeResult{}, &toolsy.ClientError{
			Reason:    "fetch failed: " + resp.Status,
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(o.maxPageBytes)+1))
	if err != nil {
		return scrapeResult{}, fmt.Errorf("toolkit/web: read body: %w", err)
	}
	if len(body) > o.maxPageBytes {
		return scrapeResult{}, &toolsy.ClientError{
			Reason:    "page too large",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	markdown, err := o.scraper.HTMLToMarkdown(string(body), o.maxPageBytes)
	if err != nil {
		return scrapeResult{}, fmt.Errorf("toolkit/web: convert: %w", err)
	}
	return scrapeResult{Markdown: markdown}, nil
}

func escapeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
