package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
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

func TestWebSearch_ReturnsMarkdown(t *testing.T) {
	provider := &mockSearchProvider{results: []SearchResult{
		{Title: "Example", URL: "https://example.com", Snippet: "An example site."},
	}}
	tools, err := AsTools(provider)
	require.NoError(t, err)
	searchTool := tools[0]

	var result searchResult
	require.NoError(t, searchTool.Execute(context.Background(), []byte(`{"query":"test"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(searchResult); ok {
				result = r
			}
		}
		return nil
	}))
	require.Contains(t, result.Results, "Example")
	require.Contains(t, result.Results, "https://example.com")
	require.Contains(t, result.Results, "An example site")
}

func TestWebSearch_EmptyQuery_ClientError(t *testing.T) {
	tools, err := AsTools(&mockSearchProvider{})
	require.NoError(t, err)
	err = tools[0].Execute(context.Background(), []byte(`{"query":"  "}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
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

	var result scrapeResult
	require.NoError(
		t,
		scrapeTool.Execute(context.Background(), []byte(`{"url":"`+server.URL+`"}`), func(c toolsy.Chunk) error {
			if c.RawData != nil {
				if r, ok := c.RawData.(scrapeResult); ok {
					result = r
				}
			}
			return nil
		}),
	)
	require.Contains(t, result.Markdown, "Hello")
	require.Contains(t, result.Markdown, "world")
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

	var result scrapeResult
	require.NoError(
		t,
		scrapeTool.Execute(context.Background(), []byte(`{"url":"`+server.URL+`"}`), func(c toolsy.Chunk) error {
			if c.RawData != nil {
				if r, ok := c.RawData.(scrapeResult); ok {
					result = r
				}
			}
			return nil
		}),
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
		[]byte(`{"url":"http://127.0.0.1:9999/"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "private")
}

func TestWebScrape_UnspecifiedIP_Blocked(t *testing.T) {
	provider := &mockSearchProvider{}
	tools, err := AsTools(provider)
	require.NoError(t, err)

	err = tools[1].Execute(
		context.Background(),
		[]byte(`{"url":"http://0.0.0.0:80/"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
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
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "blocked")
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
		[]byte(`{"url":"`+server.URL+`"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	// Redirect to loopback is rejected by CheckRedirect (ClientError) or connection fails; scrape must not succeed
}

func TestWebScrape_WithCustomScraper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<p>raw</p>"))
	}))
	defer server.Close()

	customCalled := false
	custom := &mockScraper{fn: func(_ string, _ int) (string, error) {
		customCalled = true
		return "custom output", nil
	}}
	provider := &mockSearchProvider{}
	tools, err := AsTools(provider, WithScraper(custom), WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)

	var result scrapeResult
	require.NoError(
		t,
		tools[1].Execute(context.Background(), []byte(`{"url":"`+server.URL+`"}`), func(c toolsy.Chunk) error {
			if c.RawData != nil {
				if r, ok := c.RawData.(scrapeResult); ok {
					result = r
				}
			}
			return nil
		}),
	)
	require.True(t, customCalled)
	require.Equal(t, "custom output", result.Markdown)
}

type mockScraper struct {
	fn func(html string, maxBytes int) (string, error)
}

func (m *mockScraper) HTMLToMarkdown(html string, maxBytes int) (string, error) {
	return m.fn(html, maxBytes)
}

func TestAsTools_NilProvider_Error(t *testing.T) {
	_, err := AsTools(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "SearchProvider")
}

func TestDefaultScraper_StripsTags(t *testing.T) {
	html := `<p>Text</p><script>huge();</script><style>body{}</style>`
	s := newDefaultScraper()
	out, err := s.HTMLToMarkdown(html, 10000)
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
		[]byte(`{"url":"`+server.URL+`"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "blocked")
}

func TestDefaultScraper_StripsLayoutElements(t *testing.T) {
	html := `<header><p>Site header</p></header><main><p>Main content here</p></main><nav>Links</nav><aside>Sidebar</aside><footer>Copyright</footer>`
	s := newDefaultScraper()
	out, err := s.HTMLToMarkdown(html, 10000)
	require.NoError(t, err)
	require.Contains(t, out, "Main content here")
	require.NotContains(t, out, "Site header")
	require.NotContains(t, out, "Links")
	require.NotContains(t, out, "Sidebar")
	require.NotContains(t, out, "Copyright")
}
