package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/format"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

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

type SearchWireResult struct {
	Results string `json:"results"`
}

type scrapeArgs struct {
	URL string `json:"url"`
}

type ScrapeWireResult struct {
	Markdown  string `json:"markdown"`
	SourceURL string `json:"source_url"`
}

// AsTools returns web_search and web_scrape tools. SearchProvider is required for web_search.
func AsTools(provider SearchProvider, opts ...Option) ([]toolsy.Tool, error) {
	value, _, err := AsToolsWithCleanup(provider, opts...)
	return value, err
}

// AsToolsWithCleanup returns the tools and an owned idle-pool closer. Stop new calls
// before disposal; the closer leaves active calls unaffected and is not terminal Close.
func AsToolsWithCleanup(provider SearchProvider, opts ...Option) ([]toolsy.Tool, func(), error) {
	if provider == nil {
		return nil, nil, errors.New("toolkit/web: SearchProvider is required")
	}
	o, configErr := configure(opts)
	if configErr != nil {
		return nil, nil, configErr
	}
	client, clientErr := newScrapeHTTPClient(&o)
	if clientErr != nil {
		return nil, nil, clientErr
	}
	o.httpClient = client
	success := false
	defer func() {
		if !success {
			client.CloseIdleConnections()
		}
	}()

	searchTool, err := buildSearchTool(provider, &o)
	if err != nil {
		return nil, nil, fmt.Errorf("toolkit/web: build search tool: %w", err)
	}

	scrapeTool, err := buildScrapeTool(&o)
	if err != nil {
		return nil, nil, fmt.Errorf("toolkit/web: build scrape tool: %w", err)
	}

	success = true
	return []toolsy.Tool{searchTool, scrapeTool}, client.CloseIdleConnections, nil
}

func buildSearchTool(provider SearchProvider, o *options) (toolsy.Tool, error) {
	if o.searchFormatter != nil || o.hostResultValidator != nil {
		return toolsy.NewTool[searchArgs, format.JSONResult](
			o.searchName,
			o.searchDesc,
			func(ctx context.Context, _ *toolsy.RunEnv, args searchArgs) (format.JSONResult, error) {
				results, err := searchStructured(ctx, provider, args.Query, o)
				if err != nil {
					return format.JSONResult{}, err
				}
				raw, applyErr := format.ApplyWithEnvelope(
					results,
					func(r []SearchResult) SearchWireResult {
						return SearchWireResult{Results: FormatSearchMarkdown(r)}
					},
					o.searchFormatter,
					o.hostResultValidator,
					o.maxSearchBytes,
				)
				if applyErr != nil {
					return format.JSONResult{}, applyErr
				}
				return format.JSONResult{Raw: raw}, nil
			},
			toolsy.WithReadOnly(),
		)
	}
	return toolsy.NewTool[searchArgs, format.JSONResult](
		o.searchName,
		o.searchDesc,
		func(ctx context.Context, _ *toolsy.RunEnv, args searchArgs) (format.JSONResult, error) {
			res, err := doSearch(ctx, provider, args.Query, o)
			if err != nil {
				return format.JSONResult{}, err
			}
			return format.ToJSONResult(res, o.maxSearchBytes)
		},
		toolsy.WithReadOnly(),
	)
}

func buildScrapeTool(o *options) (toolsy.Tool, error) {
	if o.scrapeFormatter != nil || o.hostResultValidator != nil {
		return toolsy.NewTool[scrapeArgs, format.JSONResult](
			o.scrapeName,
			o.scrapeDesc,
			func(ctx context.Context, _ *toolsy.RunEnv, args scrapeArgs) (format.JSONResult, error) {
				res, err := doScrape(ctx, o, args.URL)
				if err != nil {
					return format.JSONResult{}, err
				}
				var scrapeFmt func(ScrapeWireResult) (any, error)
				if o.scrapeFormatter != nil {
					scrapeFmt = func(sr ScrapeWireResult) (any, error) {
						return o.scrapeFormatter(sr)
					}
				}
				raw, applyErr := format.ApplyWithEnvelope(
					res,
					func(sr ScrapeWireResult) ScrapeWireResult { return sr },
					scrapeFmt,
					o.hostResultValidator,
					o.maxPageBytes,
				)
				if applyErr != nil {
					return format.JSONResult{}, applyErr
				}
				return format.JSONResult{Raw: raw}, nil
			},
			toolsy.WithReadOnly(),
		)
	}
	return toolsy.NewTool[scrapeArgs, format.JSONResult](
		o.scrapeName,
		o.scrapeDesc,
		func(ctx context.Context, _ *toolsy.RunEnv, args scrapeArgs) (format.JSONResult, error) {
			res, err := doScrape(ctx, o, args.URL)
			if err != nil {
				return format.JSONResult{}, err
			}
			return format.ToJSONResult(res, o.maxPageBytes)
		},
		toolsy.WithReadOnly(),
	)
}

func doSearch(ctx context.Context, provider SearchProvider, query string, o *options) (SearchWireResult, error) {
	results, err := searchStructured(ctx, provider, query, o)
	if err != nil {
		return SearchWireResult{}, err
	}
	return SearchWireResult{Results: FormatSearchMarkdown(results)}, nil
}

// parseScrapeResponse reads resp.Body after status check; caller must close the body via httptool.CloseResponseBody.
func parseScrapeResponse(ctx context.Context, resp *http.Response, o *options) (ScrapeWireResult, error) {
	if !httptool.IsSuccessStatus(resp.StatusCode) {
		return ScrapeWireResult{}, toolsy.NewValidationError("fetch failed: " + resp.Status)
	}
	bodyBytes, readErr := textprocessor.ReadLimitedBytes(ctx, resp.Body, o.maxSourceBytes)
	if mapped := toolsy.MapToolkitReadError(
		ctx, readErr, "toolkit/web: read body",
		o.maxSourceBytes, "page", "use WithMaxSourceBytes to raise the source budget",
	); mapped != nil {
		return ScrapeWireResult{}, mapped
	}
	if readErr != nil {
		return ScrapeWireResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/web: read body: %w", readErr))
	}
	body := string(bodyBytes)
	byteCap := o.maxMarkdownBytes
	markdown, convErr := scrapeHTMLToMarkdown(ctx, o.scraper, body, byteCap)
	if convErr != nil {
		if toolsy.IsContextInterrupt(convErr) {
			return ScrapeWireResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/web: convert: %w", convErr))
		}
		if IsMarkdownExceedsLimit(convErr) {
			return ScrapeWireResult{}, markdownLimitError(ctx, byteCap, convErr)
		}
		return ScrapeWireResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/web: convert: %w", convErr))
	}
	if len(markdown) > byteCap {
		return ScrapeWireResult{}, markdownLimitError(ctx, byteCap, WrapMarkdownExceedsLimit(byteCap))
	}
	source := ""
	if resp.Request != nil && resp.Request.URL != nil {
		source = resp.Request.URL.String()
	}
	return ScrapeWireResult{Markdown: markdown, SourceURL: source}, nil
}

func doScrape(ctx context.Context, o *options, rawURL string) (ScrapeWireResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ScrapeWireResult{}, toolsy.NewValidationError("url is required")
	}
	u, err := validateScrapeURL(ctx, rawURL, o.allowPrivateIPs, o.blockedDomains)
	if err != nil {
		return ScrapeWireResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ScrapeWireResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/web: new request: %w", err))
	}
	client := o.httpClient
	resp, doErr := client.Do(req) //nolint:bodyclose // closed via httptool.CloseResponseBody
	if doErr != nil {
		if _, ok := toolsy.AsToolError(doErr); ok {
			return ScrapeWireResult{}, doErr
		}
		return ScrapeWireResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/web: fetch: %w", doErr))
	}
	defer httptool.CloseResponseBody(ctx, resp.Body)
	return parseScrapeResponse(ctx, resp, o)
}

func escapeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
