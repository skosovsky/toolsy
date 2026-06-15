package httptool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

func decodeHTTPResult(t *testing.T, c toolsy.Chunk) httpResult {
	t.Helper()
	out, err := toolsy.DecodeChunkAs[httpResult](c)
	require.NoError(t, err)
	return *out
}

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
	require.NoError(
		t,
		getTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + srv.URL + `"}`)},
			func(c toolsy.Chunk) error {
				result = decodeHTTPResult(t, c)
				return nil
			},
		),
	)
	require.Equal(t, 200, result.Status)
	require.Equal(t, "hello", result.Body)
}

func TestHTTPGet_DomainBlocked(t *testing.T) {
	tools, err := AsTools(WithAllowedDomains([]string{"api.example.com"}))
	require.NoError(t, err)
	getTool := tools[0]

	err = getTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"https://evil.com/path"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestHTTPGet_ExceedsLimitReturnsValidationError(t *testing.T) {
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

	err = getTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + srv.URL + `"}`)},
		func(_ toolsy.Chunk) error {
			return nil
		},
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "20")
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestHTTPPost_ExceedsLimitReturnsValidationError(t *testing.T) {
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
	postTool := tools[1]

	err = postTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + srv.URL + `"}`)},
		func(_ toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "20")
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestReadBodyLimited_CancelMidRead_MapsInternal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	slow := &slowStreamReader{ctx: ctx}
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := ReadBodyLimited(ctx, slow, 1<<20)
	mapped := toolsy.MapToolkitReadError(ctx, err, "toolkit/httptool: read body", 1<<20, "response body", "")
	if mapped != nil {
		err = mapped
	} else if err != nil {
		err = toolsy.NewInternalError(fmt.Errorf("toolkit/httptool: read body: %w", err))
	}
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestReadBodyLimited_InterruptInChainOverReadLimit_MapsInternal(t *testing.T) {
	composite := fmt.Errorf(
		"read: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	mapped := toolsy.MapToolkitReadError(
		context.Background(),
		composite,
		"toolkit/httptool: read body",
		1<<20,
		"response body",
		"",
	)
	require.Error(t, mapped)
	require.ErrorIs(t, mapped, context.Canceled)
	te, ok := toolsy.AsToolError(mapped)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
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
	require.NoError(
		t,
		postTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + srv.URL + `","json_body":{"key":"value"}}`)},
			func(c toolsy.Chunk) error {
				result = decodeHTTPResult(t, c)
				return nil
			},
		),
	)
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

	require.NoError(
		t,
		postTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + srv.URL + `","json_body":{"a":1}}`)},
			func(toolsy.Chunk) error { return nil },
		),
	)
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
	require.NoError(
		t,
		postTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + srv.URL + `"}`)},
			func(toolsy.Chunk) error { return nil },
		),
	)
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

	require.NoError(
		t,
		getTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + srv.URL + `"}`)},
			func(toolsy.Chunk) error { return nil },
		),
	)
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
	require.NoError(
		t,
		getTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + srv.URL + `"}`)},
			func(c toolsy.Chunk) error {
				result = decodeHTTPResult(t, c)
				return nil
			},
		),
	)
	require.Equal(t, 500, result.Status)
	require.Equal(t, "server error", result.Body)
}

func TestAsTools_ToolCount(t *testing.T) {
	tools, err := AsTools(WithAllowedDomains([]string{"example.com"}))
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "http_get", tools[0].Manifest().Name)
	require.Equal(t, "http_post", tools[1].Manifest().Name)
	require.True(t, tools[0].Manifest().ReadOnly)
	require.True(t, tools[1].Manifest().Dangerous)
	require.True(t, tools[1].Manifest().RequiresConfirmation)
}

func TestAsTools_CustomToolNames(t *testing.T) {
	tools, err := AsTools(
		WithAllowedDomains([]string{"example.com"}),
		WithGetName("fetch"),
		WithPostName("push"),
	)
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "fetch", tools[0].Manifest().Name)
	require.Equal(t, "push", tools[1].Manifest().Name)
}

func TestHTTPGet_BlocksPrivateIPWithoutAllow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tools, err := AsTools(WithAllowedDomains([]string{"127.0.0.1"}))
	require.NoError(t, err)
	err = tools[0].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + srv.URL + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
}

func TestHTTPGet_CancelDuringDo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tools, err := AsTools(
		WithAllowedDomains([]string{"127.0.0.1"}),
		WithAllowPrivateIPs(true),
		WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)
	getTool := tools[0]

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- getTool.Execute(
			ctx,
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + srv.URL + `"}`)},
			func(toolsy.Chunk) error { return nil },
		)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	err = <-done
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

type cancelCredentials struct{}

func (cancelCredentials) GetAuth(context.Context, string) (string, error) {
	return "", context.Canceled
}

func TestHTTPGet_CancelDuringGetAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tools, err := AsTools(
		WithAllowedDomains([]string{"127.0.0.1"}),
		WithAllowPrivateIPs(true),
		WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)

	err = tools[0].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil, toolsy.WithCredentials(cancelCredentials{})),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + srv.URL + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}
