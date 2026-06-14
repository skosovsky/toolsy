package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	custom := &mockScraper{fn: func(_ string, _ int) (string, error) {
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
	fn func(html string, maxBytes int) (string, error)
}

func (m *mockScraper) HTMLToMarkdown(html string, maxBytes int) (string, error) {
	return m.fn(html, maxBytes)
}

func TestAsTools_NilProvider_Error(t *testing.T) {
	_, err := AsTools(nil)
	require.Error(t, err)
}

func TestHTMLScraper_StripsTags(t *testing.T) {
	html := `<p>Text</p><script>huge();</script><style>body{}</style>`
	s := newHTMLScraper()
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
	out, err := s.HTMLToMarkdown(html, 10000)
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
	body := "<html><body><p>" + strings.Repeat("x", 220) + "</p></body></html>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	const maxWire = 250
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

func TestHTMLScraper_TruncatesWithoutSuffix(t *testing.T) {
	html := "<html><body><p>" + strings.Repeat("x", 500) + "</p></body></html>"
	s := newHTMLScraper()
	out, err := s.HTMLToMarkdown(html, 50)
	require.NoError(t, err)
	require.LessOrEqual(t, len(out), 50)
	require.NotContains(t, out, "[Truncated]")
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
