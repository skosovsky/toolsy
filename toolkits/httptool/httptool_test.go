package httptool

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestHTTPGet_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	tools, err := AsTools(
		WithAllowedDomains([]string{"127.0.0.1"}),
		WithAllowPrivateIPs(true),
	)
	require.NoError(t, err)
	getTool := tools[0]

	var result httpResult
	require.NoError(t, getTool.Execute(context.Background(), []byte(`{"url":"`+srv.URL+`"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(httpResult); ok {
				result = r
			}
		}
		return nil
	}))
	require.Equal(t, 200, result.Status)
	require.Equal(t, "hello", result.Body)
}

func TestHTTPGet_DomainBlocked(t *testing.T) {
	tools, err := AsTools(WithAllowedDomains([]string{"api.example.com"}))
	require.NoError(t, err)
	getTool := tools[0]

	err = getTool.Execute(context.Background(), []byte(`{"url":"https://evil.com/path"}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
}

func TestHTTPGet_Truncation(t *testing.T) {
	largeBody := make([]byte, 100)
	for i := range largeBody {
		largeBody[i] = 'x'
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(largeBody)
	}))
	defer srv.Close()

	tools, err := AsTools(
		WithAllowedDomains([]string{"127.0.0.1"}),
		WithAllowPrivateIPs(true),
		WithMaxResponseBody(20),
	)
	require.NoError(t, err)
	getTool := tools[0]

	var result httpResult
	require.NoError(t, getTool.Execute(context.Background(), []byte(`{"url":"`+srv.URL+`"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(httpResult); ok {
				result = r
			}
		}
		return nil
	}))
	require.Equal(t, 200, result.Status)
	require.Contains(t, result.Body, "[Truncated]")
	require.LessOrEqual(t, len(result.Body), 20+len(truncationSuffix)+2)
}

func TestHTTPPost_Success(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 100)
		n, _ := r.Body.Read(b)
		received = string(b[:n])
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tools, err := AsTools(
		WithAllowedDomains([]string{"127.0.0.1"}),
		WithAllowPrivateIPs(true),
	)
	require.NoError(t, err)
	postTool := tools[1]

	var result httpResult
	require.NoError(t, postTool.Execute(context.Background(), []byte(`{"url":"`+srv.URL+`","json_body":{"key":"value"}}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(httpResult); ok {
				result = r
			}
		}
		return nil
	}))
	require.Equal(t, 200, result.Status)
	require.Equal(t, "ok", result.Body)
	require.Contains(t, received, "key")
	require.Contains(t, received, "value")
}

func TestHTTPPost_ContentTypeSet(t *testing.T) {
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tools, err := AsTools(
		WithAllowedDomains([]string{"127.0.0.1"}),
		WithAllowPrivateIPs(true),
	)
	require.NoError(t, err)
	postTool := tools[1]

	require.NoError(t, postTool.Execute(context.Background(), []byte(`{"url":"`+srv.URL+`","json_body":{"a":1}}`), func(toolsy.Chunk) error { return nil }))
	require.Equal(t, "application/json", gotContentType)
}

func TestHTTPPost_EmptyBody(t *testing.T) {
	var bodyLen int
	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		bodyLen = len(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tools, err := AsTools(
		WithAllowedDomains([]string{"127.0.0.1"}),
		WithAllowPrivateIPs(true),
	)
	require.NoError(t, err)
	postTool := tools[1]

	// POST with URL only, no json_body
	require.NoError(t, postTool.Execute(context.Background(), []byte(`{"url":"`+srv.URL+`"}`), func(toolsy.Chunk) error { return nil }))
	require.Equal(t, 0, bodyLen, "body must be empty when json_body is omitted")
	require.Empty(t, contentType, "Content-Type must not be set when body is empty")
}

func TestHTTPGet_HeadersApplied(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tools, err := AsTools(
		WithAllowedDomains([]string{"127.0.0.1"}),
		WithAllowPrivateIPs(true),
		WithHeaders(map[string]string{"X-Custom": "test-value"}),
	)
	require.NoError(t, err)
	getTool := tools[0]

	require.NoError(t, getTool.Execute(context.Background(), []byte(`{"url":"`+srv.URL+`"}`), func(toolsy.Chunk) error { return nil }))
	require.Equal(t, "test-value", gotHeader)
}

func TestHTTPGet_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer srv.Close()

	tools, err := AsTools(
		WithAllowedDomains([]string{"127.0.0.1"}),
		WithAllowPrivateIPs(true),
	)
	require.NoError(t, err)
	getTool := tools[0]

	var result httpResult
	require.NoError(t, getTool.Execute(context.Background(), []byte(`{"url":"`+srv.URL+`"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(httpResult); ok {
				result = r
			}
		}
		return nil
	}))
	require.Equal(t, 500, result.Status)
	require.Equal(t, "server error", result.Body)
}

func TestAsTools_ToolCount(t *testing.T) {
	tools, err := AsTools(WithAllowedDomains([]string{"example.com"}))
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "http_get", tools[0].Name())
	require.Equal(t, "http_post", tools[1].Name())
}

func TestAsTools_CustomToolNames(t *testing.T) {
	tools, err := AsTools(
		WithAllowedDomains([]string{"example.com"}),
		WithGetName("fetch"),
		WithPostName("push"),
	)
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "fetch", tools[0].Name())
	require.Equal(t, "push", tools[1].Name())
}
