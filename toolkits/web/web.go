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

type SearchWireResult struct {
	Results string `json:"results"`
}

type scrapeArgs struct {
	URL string `json:"url"`
}

type ScrapeWireResult struct {
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

	searchTool, err := buildSearchTool(provider, &o)
	if err != nil {
		return nil, fmt.Errorf("toolkit/web: build search tool: %w", err)
	}

	scrapeTool, err := buildScrapeTool(&o)
	if err != nil {
		return nil, fmt.Errorf("toolkit/web: build scrape tool: %w", err)
	}

	return []toolsy.Tool{searchTool, scrapeTool}, nil
}

func buildSearchTool(provider SearchProvider, o *options) (toolsy.Tool, error) {
	if o.searchFormatter != nil || o.hostResultValidator != nil {
		return toolsy.NewTool[searchArgs, format.JSONResult](
			o.searchName,
			o.searchDesc,
			func(ctx context.Context, _ *toolsy.RunEnv, args searchArgs) (format.JSONResult, error) {
				results, err := SearchStructured(ctx, provider, args.Query)
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
			res, err := doSearch(ctx, provider, args.Query)
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
						return o.scrapeFormatter(sr.Markdown)
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

func doSearch(ctx context.Context, provider SearchProvider, query string) (SearchWireResult, error) {
	results, err := SearchStructured(ctx, provider, query)
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
	bodyBytes, readErr := textprocessor.ReadLimitedBytes(ctx, resp.Body, scrapeContentByteCap(o.maxPageBytes))
	if mapped := toolsy.MapToolkitReadError(
		ctx, readErr, "toolkit/web: read body",
		scrapeContentByteCap(o.maxPageBytes), "page", "use WithMaxPageBytes to raise the budget",
	); mapped != nil {
		return ScrapeWireResult{}, mapped
	}
	if readErr != nil {
		return ScrapeWireResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/web: read body: %w", readErr))
	}
	body := string(bodyBytes)
	byteCap := scrapeContentByteCap(o.maxPageBytes)
	markdown, convErr := scrapeHTMLToMarkdown(ctx, o.scraper, body, byteCap)
	if convErr != nil {
		if ie := toolsy.ToolkitContextError(ctx, "toolkit/web: convert"); ie != nil {
			return ScrapeWireResult{}, ie
		}
		if IsMarkdownExceedsLimit(convErr) {
			return ScrapeWireResult{}, toolsy.MapToolkitCapError(
				ctx,
				"toolkit/web: convert",
				byteCap,
				"markdown",
				"use WithMaxPageBytes to raise the budget",
			)
		}
		return ScrapeWireResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/web: convert: %w", convErr))
	}
	return ScrapeWireResult{Markdown: markdown}, nil
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
	client, err := scrapeHTTPClient(o)
	if err != nil {
		return ScrapeWireResult{}, err
	}
	resp, doErr := client.Do(req) //nolint:bodyclose // closed via httptool.CloseResponseBody
	if doErr != nil {
		if toolErrorClientCorrectable(doErr) {
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

func toolErrorClientCorrectable(err error) bool {
	te, ok := toolsy.AsToolError(err)
	return ok && toolsy.ClientCorrectable(te.Code)
}
