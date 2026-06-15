package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

type mockSearchProvider struct {
	results []SearchResult
	err     error
}

func (m *mockSearchProvider) Search(ctx context.Context, query string) ([]SearchResult, error) {
	_ = ctx
	_ = query
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func decodeWebChunk[T any](t *testing.T, c toolsy.Chunk) T {
	t.Helper()
	require.Equal(t, toolsy.MimeTypeJSON, c.MimeType)
	var out T
	require.NoError(t, json.Unmarshal(c.Data, &out))
	return out
}

func TestWebSearch_ReturnsMarkdown(t *testing.T) {
	provider := &mockSearchProvider{results: []SearchResult{
		{Title: "Example", URL: "https://example.com", Snippet: "An example site."},
	}}
	tools, err := AsTools(provider)
	require.NoError(t, err)
	searchTool := tools[0]

	var result SearchWireResult
	require.NoError(
		t,
		searchTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"test"}`)},
			func(c toolsy.Chunk) error {
				result = decodeWebChunk[SearchWireResult](t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Results, "Example")
	require.Contains(t, result.Results, "https://example.com")
	require.Contains(t, result.Results, "An example site")
}

func TestWebSearch_EmptyQuery_ValidationToolError(t *testing.T) {
	tools, err := AsTools(&mockSearchProvider{})
	require.NoError(t, err)
	err = tools[0].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"  "}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestWebScrape_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>Hello <strong>world</strong></p></body></html>"))
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider, WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)
	scrapeTool := tools[1]

	var result ScrapeWireResult
	require.NoError(
		t,
		scrapeTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
			func(c toolsy.Chunk) error {
				result = decodeWebChunk[ScrapeWireResult](t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Markdown, "Hello")
	require.Contains(t, result.Markdown, "world")
}

func TestWebScrape_ExceedsMaxBody(t *testing.T) {
	const maxPage = 4096
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>" + strings.Repeat("x", 50000) + "</p></body></html>"))
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(
		provider,
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithMaxPageBytes(maxPage),
	)
	require.NoError(t, err)
	scrapeTool := tools[1]

	err = scrapeTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, strconv.Itoa(scrapeContentByteCap(maxPage)))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestWebScrape_MarkdownExpansionExceedsCap(t *testing.T) {
	const maxPage = 250
	contentCap := scrapeContentByteCap(maxPage)
	// Many <hr/> tags: HTML fits content cap but markdown expands (--- per rule).
	body := "<html><body>" + strings.Repeat("<hr/>", 41) + "</body></html>"
	require.LessOrEqual(t, len(body), contentCap)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(
		provider,
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithMaxPageBytes(maxPage),
	)
	require.NoError(t, err)
	scrapeTool := tools[1]

	err = scrapeTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "markdown exceeds")
	require.Contains(t, te.Reason, strconv.Itoa(contentCap))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestParseScrapeResponse_CancelOverMarkdownCap_ReturnsInternal(t *testing.T) {
	const maxPage = 4096
	contentCap := scrapeContentByteCap(maxPage)
	o := &options{}
	WithMaxPageBytes(maxPage)(o)
	applyDefaults(o)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader("<p>x</p>")),
	}
	_, err := parseScrapeResponse(ctx, resp, o)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)

	// With active ctx, markdown exceed still maps validation.
	activeCtx := context.Background()
	WithScraper(&mockScraper{fn: func(_ context.Context, _ string, maxBytes int) (string, error) {
		return "", WrapMarkdownExceedsLimit(maxBytes)
	}})(o)
	_, err = parseScrapeResponse(activeCtx, resp, o)
	require.Error(t, err)
	te, ok = toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, strconv.Itoa(contentCap))
}

func TestScrape_CancelOverHTMLReadLimit_InterruptWins(t *testing.T) {
	const maxPage = 4096
	contentCap := scrapeContentByteCap(maxPage)
	o := &options{}
	WithMaxPageBytes(maxPage)(o)
	applyDefaults(o)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	huge := strings.Repeat("x", contentCap+100)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(huge)),
	}
	_, err := parseScrapeResponse(ctx, resp, o)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestScrape_InterruptInChainOverReadLimit_InterruptWins(t *testing.T) {
	composite := fmt.Errorf(
		"read: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	mapped := toolsy.MapToolkitReadError(
		context.Background(),
		composite,
		"toolkit/web: read body",
		scrapeContentByteCap(4096),
		"page",
		"use WithMaxPageBytes to raise the budget",
	)
	require.Error(t, mapped)
	require.ErrorIs(t, mapped, context.Canceled)
	te, ok := toolsy.AsToolError(mapped)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
}

func TestParseScrapeResponse_MarkdownExceedsCap(t *testing.T) {
	const maxPage = 4096
	contentCap := scrapeContentByteCap(maxPage)
	o := &options{}
	WithMaxPageBytes(maxPage)(o)
	WithScraper(&mockScraper{fn: func(_ context.Context, _ string, maxBytes int) (string, error) {
		return "", WrapMarkdownExceedsLimit(maxBytes)
	}})(o)
	applyDefaults(o)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader("<p>x</p>")),
	}
	_, err := parseScrapeResponse(context.Background(), resp, o)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "markdown exceeds")
	require.Contains(t, te.Reason, strconv.Itoa(contentCap))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestParseScrapeResponse_CancelDuringConvert(t *testing.T) {
	o := &options{}
	WithScraper(&mockScraper{fn: func(ctx context.Context, _ string, _ int) (string, error) {
		return "", ctx.Err()
	}})(o)
	applyDefaults(o)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader("<p>x</p>")),
	}
	_, err := parseScrapeResponse(ctx, resp, o)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
	require.ErrorIs(t, te, context.Canceled)
}

func TestWebScrape_CancelDuringDo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte("<p>x</p>"))
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(
		provider,
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
	)
	require.NoError(t, err)
	scrapeTool := tools[1]

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- scrapeTool.Execute(
			ctx,
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
			func(toolsy.Chunk) error { return nil },
		)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	err = <-done
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestParseScrapeResponse_CustomScraperMarkdownMessage(t *testing.T) {
	const maxPage = 4096
	o := &options{}
	WithMaxPageBytes(maxPage)(o)
	WithScraper(&mockScraper{fn: func(_ context.Context, _ string, maxBytes int) (string, error) {
		return "", fmt.Errorf("custom markdown exceeds %d byte limit", maxBytes)
	}})(o)
	applyDefaults(o)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader("<p>x</p>")),
	}
	_, err := parseScrapeResponse(context.Background(), resp, o)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
}

func TestWebScrape_CustomScraper_ExceedsCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<p>x</p>"))
	}))
	defer server.Close()

	const maxPage = 4096
	contentCap := scrapeContentByteCap(maxPage)
	custom := &mockScraper{fn: func(_ context.Context, _ string, maxBytes int) (string, error) {
		require.Equal(t, contentCap, maxBytes)
		return "", fmt.Errorf("toolkit/web: markdown exceeds %d byte limit: %w", maxBytes, ErrMarkdownExceedsLimit)
	}}
	provider := &mockSearchProvider{}
	tools, err := AsTools(
		provider,
		WithScraper(custom),
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithMaxPageBytes(maxPage),
	)
	require.NoError(t, err)

	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, strconv.Itoa(contentCap))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestWebScrape_ScriptAndStyleStripped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		html := `<html><body><p>Content</p><script>alert("x")</script><style>.x{}</style></body></html>`
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider, WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)
	scrapeTool := tools[1]

	var result ScrapeWireResult
	require.NoError(
		t,
		scrapeTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
			func(c toolsy.Chunk) error {
				result = decodeWebChunk[ScrapeWireResult](t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Markdown, "Content")
	require.NotContains(t, result.Markdown, "alert")
	require.NotContains(t, result.Markdown, ".x{}")
}

func TestWebScrape_SSRFBlocked(t *testing.T) {
	provider := &mockSearchProvider{}
	tools, err := AsTools(provider)
	require.NoError(t, err)

	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"http://127.0.0.1:9999/"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "private")
}

func TestWebScrape_UnspecifiedIP_Blocked(t *testing.T) {
	provider := &mockSearchProvider{}
	tools, err := AsTools(provider)
	require.NoError(t, err)

	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"http://0.0.0.0:80/"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestWebScrape_BlockedDomain_SubdomainBlocked(t *testing.T) {
	// Subdomain of a blocked domain is also blocked (exact match or host ends with .blocked)
	_, err := validateScrapeURL(
		context.Background(),
		"http://api.evil.example.com/",
		true,
		[]string{"evil.example.com"},
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "blocked")
}

func TestWebScrape_RedirectToLoopbackBlocked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:80/", http.StatusFound)
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider, WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)

	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	// Redirect to loopback is rejected by CheckRedirect (validation ToolError) or connection fails; scrape must not succeed
}

func TestWebScrape_WithCustomScraper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<p>raw</p>"))
	}))
	defer server.Close()

	customCalled := false
	custom := &mockScraper{fn: func(_ context.Context, _ string, _ int) (string, error) {
		customCalled = true
		return "custom output", nil
	}}
	provider := &mockSearchProvider{}
	tools, err := AsTools(provider, WithScraper(custom), WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)

	var result ScrapeWireResult
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
			func(c toolsy.Chunk) error {
				result = decodeWebChunk[ScrapeWireResult](t, c)
				return nil
			},
		),
	)
	require.True(t, customCalled)
	require.Equal(t, "custom output", result.Markdown)
}

type mockScraper struct {
	fn func(ctx context.Context, html string, maxBytes int) (string, error)
}

func (m *mockScraper) HTMLToMarkdown(ctx context.Context, html string, maxBytes int) (string, error) {
	return m.fn(ctx, html, maxBytes)
}

func TestAsTools_NilProvider_Error(t *testing.T) {
	_, err := AsTools(nil)
	require.Error(t, err)
}

func TestHTMLScraper_StripsTags(t *testing.T) {
	html := `<p>Text</p><script>huge();</script><style>body{}</style>`
	s := newHTMLScraper()
	out, err := s.HTMLToMarkdown(context.Background(), html, 10000)
	require.NoError(t, err)
	require.Contains(t, out, "Text")
	require.False(t, strings.Contains(out, "huge") || strings.Contains(out, "body{}"))
}

func TestWebScrape_BlockedRedirectDomain_Rejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://blocked-internal.example/", http.StatusFound)
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider, WithHTTPClient(server.Client()), WithAllowPrivateIPs(true),
		WithBlockedDomains([]string{"blocked-internal.example"}))
	require.NoError(t, err)

	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "blocked")
}

func TestHTMLScraper_StripsLayoutElements(t *testing.T) {
	html := `<header><p>Site header</p></header><main><p>Main content here</p></main><nav>Links</nav><aside>Sidebar</aside><footer>Copyright</footer>`
	s := newHTMLScraper()
	out, err := s.HTMLToMarkdown(context.Background(), html, 10000)
	require.NoError(t, err)
	require.Contains(t, out, "Main content here")
	require.NotContains(t, out, "Site header")
	require.NotContains(t, out, "Links")
	require.NotContains(t, out, "Sidebar")
	require.NotContains(t, out, "Copyright")
}

func TestAsTools_ReadOnlyManifest(t *testing.T) {
	provider := &mockSearchProvider{}
	tools, err := AsTools(provider)
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.True(t, tools[0].Manifest().ReadOnly)
	require.True(t, tools[1].Manifest().ReadOnly)
}

func TestWebSearch_FormatterAndValidator(t *testing.T) {
	provider := &mockSearchProvider{results: []SearchResult{{Title: "T", URL: "https://x.com", Snippet: "s"}}}
	tools, err := AsTools(provider,
		WithSearchFormatter(func(_ []SearchResult) (any, error) {
			return map[string]int{"hits": 1}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]int)
			if !ok {
				return errors.New("expected formatter output map")
			}
			if payload["hits"] != 1 {
				return errors.New("unexpected hits")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var payload map[string]int
	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"q"}`)},
			func(c toolsy.Chunk) error {
				return json.Unmarshal(c.Data, &payload)
			},
		),
	)
	require.Equal(t, 1, payload["hits"])
}

func TestWebSearch_WithHostResultValidator_Reject(t *testing.T) {
	provider := &mockSearchProvider{results: []SearchResult{{Title: "T", URL: "https://x.com", Snippet: "s"}}}
	tools, err := AsTools(provider, WithHostResultValidator(func(_ any) error {
		return assert.AnError
	}))
	require.NoError(t, err)
	err = tools[0].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"q"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestWebScrape_WithHostResultValidator_Reject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body><p>Hi</p></body></html>"))
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider,
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithHostResultValidator(func(_ any) error {
			return assert.AnError
		}),
	)
	require.NoError(t, err)
	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestWebSearch_WithHostResultValidator_Envelope(t *testing.T) {
	provider := &mockSearchProvider{results: []SearchResult{{Title: "T", URL: "https://x.com", Snippet: "s"}}}
	tools, err := AsTools(provider, WithHostResultValidator(func(v any) error {
		_, ok := v.(SearchWireResult)
		if !ok {
			return assert.AnError
		}
		return nil
	}))
	require.NoError(t, err)
	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"q"}`)},
			func(toolsy.Chunk) error { return nil },
		),
	)
}

func TestWebScrape_WithHostResultValidator_Envelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body><p>Hi</p></body></html>"))
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider,
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithHostResultValidator(func(v any) error {
			_, ok := v.(ScrapeWireResult)
			if !ok {
				return assert.AnError
			}
			return nil
		}),
	)
	require.NoError(t, err)
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
			func(toolsy.Chunk) error { return nil },
		),
	)
}

func TestWebScrape_BlockedDomain_DotSuffix(t *testing.T) {
	_, err := validateScrapeURL(
		context.Background(),
		"http://api.evil.com/",
		true,
		[]string{".evil.com"},
	)
	require.Error(t, err)
}

func TestWebScrape_WithScrapeFormatter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body><p>Hi</p></body></html>"))
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider,
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithScrapeFormatter(func(_ string) (any, error) {
			return map[string]string{"fmt": "custom"}, nil
		}),
	)
	require.NoError(t, err)
	var payload map[string]string
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
			func(c toolsy.Chunk) error {
				require.NoError(t, json.Unmarshal(c.Data, &payload))
				return nil
			},
		),
	)
	require.Equal(t, "custom", payload["fmt"])
}

func TestWebSearch_WithSearchFormatter(t *testing.T) {
	provider := &mockSearchProvider{results: []SearchResult{{Title: "A", URL: "https://a.com", Snippet: "s"}}}
	tools, err := AsTools(provider, WithSearchFormatter(func(_ []SearchResult) (any, error) {
		return map[string]int{"hits": 1}, nil
	}))
	require.NoError(t, err)
	var payload map[string]int
	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"q"}`)},
			func(c toolsy.Chunk) error {
				require.NoError(t, json.Unmarshal(c.Data, &payload))
				return nil
			},
		),
	)
	require.Equal(t, 1, payload["hits"])
}

func TestWebScrape_FormatterAndValidator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body><p>Hi</p></body></html>"))
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider,
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithScrapeFormatter(func(_ string) (any, error) {
			return map[string]string{"page": "ok"}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]string)
			if !ok {
				return errors.New("expected formatter output map")
			}
			if payload["page"] != "ok" {
				return errors.New("unexpected page")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var payload map[string]string
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
			func(c toolsy.Chunk) error {
				return json.Unmarshal(c.Data, &payload)
			},
		),
	)
	require.Equal(t, "ok", payload["page"])
}

func TestWebScrape_WithMaxPageBytes_WithResultFormatter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body><p>Hi</p></body></html>"))
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider,
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithMaxPageBytes(60),
		WithScrapeFormatter(func(_ string) (any, error) {
			return map[string]string{"blob": strings.Repeat("z", 500)}, nil
		}),
	)
	require.NoError(t, err)
	var wire []byte
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), 60+len(textprocessor.TruncationSuffix)+2)
}

func TestWebScrape_WireCapSingleTruncSuffix(t *testing.T) {
	// HTML must fit scrapeContentByteCap(maxWire) — read is fail-closed.
	const maxWire = 250
	contentCap := scrapeContentByteCap(maxWire)
	overhead := len("<html><body><p>") + len("</p></body></html>")
	repeat := max(contentCap-overhead, 1)
	body := "<html><body><p>" + strings.Repeat("x", repeat) + "</p></body></html>"
	require.LessOrEqual(t, len(body), contentCap)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider,
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithMaxPageBytes(maxWire),
	)
	require.NoError(t, err)

	var wire []byte
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), maxWire+len(textprocessor.TruncationSuffix)+2)
	require.LessOrEqual(t, strings.Count(string(wire), "[Truncated]"), 1)
	if json.Valid(wire) {
		var result ScrapeWireResult
		require.NoError(t, json.Unmarshal(wire, &result))
		require.NotContains(t, result.Markdown, "[Truncated]")
	}
}

func TestHTMLScraper_ExceedsMaxBytesReturnsError(t *testing.T) {
	html := "<html><body><p>" + strings.Repeat("x", 500) + "</p></body></html>"
	s := newHTMLScraper()
	_, err := s.HTMLToMarkdown(context.Background(), html, 50)
	require.Error(t, err)
	require.True(t, IsMarkdownExceedsLimit(err))
	require.Contains(t, err.Error(), "exceeds 50 byte limit")
}

func TestHTMLScraper_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	html := "<html><body><p>" + strings.Repeat("x", 500) + "</p></body></html>"
	_, err := scrapeHTMLToMarkdown(ctx, newHTMLScraper(), html, 0)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestWebScrape_Non2xxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider, WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)
	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Contains(t, te.Reason, "500")
}

func TestWebScrape_Accepts2xxWithoutBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider, WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)
	var result ScrapeWireResult
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
			func(c toolsy.Chunk) error {
				result = decodeWebChunk[ScrapeWireResult](t, c)
				return nil
			},
		),
	)
	require.Empty(t, result.Markdown)
}

func TestWebSearch_WithMaxSearchBytes_WireCap(t *testing.T) {
	results := make([]SearchResult, 5)
	for i := range results {
		results[i] = SearchResult{
			Title:   strings.Repeat("T", 200),
			URL:     "https://example.com/" + strings.Repeat("u", 50),
			Snippet: strings.Repeat("s", 200),
		}
	}
	provider := &mockSearchProvider{results: results}
	const maxWire = 300
	tools, err := AsTools(provider, WithMaxSearchBytes(maxWire))
	require.NoError(t, err)

	var wire []byte
	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"q"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), maxWire+len(textprocessor.TruncationSuffix)+2)
	if json.Valid(wire) {
		var result SearchWireResult
		require.NoError(t, json.Unmarshal(wire, &result))
		require.NotContains(t, result.Results, textprocessor.TruncationSuffix)
	}
}

func TestWebSearch_SemanticAndWireCapsIndependent(t *testing.T) {
	results := make([]SearchResult, 51)
	for i := range results {
		results[i] = SearchResult{Title: "Hit", URL: "https://x.com", Snippet: "snippet"}
	}
	markdown := FormatSearchMarkdown(results)
	require.Contains(t, markdown, strings.TrimSuffix(textprocessor.SearchResultsTruncationSuffix, "\n"))
	require.NotContains(t, markdown, textprocessor.TruncationSuffix)

	provider := &mockSearchProvider{results: results}
	const maxWire = 400
	tools, err := AsTools(provider, WithMaxSearchBytes(maxWire))
	require.NoError(t, err)

	var wire []byte
	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"q"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), maxWire+len(textprocessor.TruncationSuffix)+2)
	require.LessOrEqual(t, strings.Count(string(wire), "[Truncated]"), 1)
}

func TestWebSearch_TripleIoC_MaxBytesFormatterValidator(t *testing.T) {
	provider := &mockSearchProvider{results: []SearchResult{{Title: "T", URL: "https://x.com", Snippet: "s"}}}
	tools, err := AsTools(provider,
		WithMaxSearchBytes(80),
		WithSearchFormatter(func(_ []SearchResult) (any, error) {
			return map[string]string{"blob": strings.Repeat("z", 500)}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]string)
			if !ok || payload["blob"] == "" {
				return errors.New("invalid payload")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var wire []byte
	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"q"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), 80+len(textprocessor.TruncationSuffix)+2)
}

func TestWebScrape_TripleIoC_MaxBytesFormatterValidator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body><p>Hi</p></body></html>"))
	}))
	defer server.Close()

	provider := &mockSearchProvider{}
	tools, err := AsTools(provider,
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithMaxPageBytes(80),
		WithScrapeFormatter(func(_ string) (any, error) {
			return map[string]string{"blob": strings.Repeat("z", 500)}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]string)
			if !ok || payload["blob"] == "" {
				return errors.New("invalid payload")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var wire []byte
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), 80+len(textprocessor.TruncationSuffix)+2)
}
