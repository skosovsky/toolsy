package openapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"unicode/utf8"

	"github.com/skosovsky/toolsy"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type limitReadCloser struct {
	data        []byte
	pos         int
	returned    int
	maxReturned int
}

func (r *limitReadCloser) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	remaining := r.maxReturned - r.returned
	if remaining <= 0 {
		return 0, errors.New("read beyond limit")
	}
	n := len(p)
	n = min(n, len(r.data)-r.pos)
	n = min(n, remaining)
	copy(p[:n], r.data[r.pos:r.pos+n])
	r.pos += n
	r.returned += n
	if r.pos >= len(r.data) {
		return n, io.EOF
	}
	return n, nil
}

func (*limitReadCloser) Close() error { return nil }

func TestExecuteTruncatesOversizedResponse(t *testing.T) {
	body := []byte("abcdefghijklmnopqrstuvwxyz")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/items" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	var got toolsy.Chunk
	err := execute(
		context.Background(),
		toolsy.RunContext{},
		"list_items",
		http.MethodGet,
		"/items",
		nil,
		nil,
		nil,
		[]byte(`{}`),
		&Options{
			BaseURL:          server.URL,
			HTTPClient:       server.Client(),
			MaxResponseBytes: 5,
		},
		func(c toolsy.Chunk) error {
			got = c
			return nil
		},
	)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	expected := append(append([]byte(nil), body[:5]...), []byte(truncationSuffix)...)
	if !bytes.Equal(got.Data, expected) {
		t.Fatalf("unexpected body: got %q want %q", got.Data, expected)
	}
	if got.Event != toolsy.EventResult {
		t.Fatalf("unexpected event: %s", got.Event)
	}
}

func TestExecuteTruncatesOversizedResponseUTF8Safely(t *testing.T) {
	body := []byte("приветмир")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	var got toolsy.Chunk
	err := execute(
		context.Background(),
		toolsy.RunContext{},
		"list_items",
		http.MethodGet,
		"/items",
		nil,
		nil,
		nil,
		[]byte(`{}`),
		&Options{
			BaseURL:          server.URL,
			HTTPClient:       server.Client(),
			MaxResponseBytes: 5,
		},
		func(c toolsy.Chunk) error {
			got = c
			return nil
		},
	)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	if !utf8.Valid(got.Data) {
		t.Fatalf("response must remain valid UTF-8: %q", got.Data)
	}
	expected := []byte("пр" + truncationSuffix)
	if !bytes.Equal(got.Data, expected) {
		t.Fatalf("unexpected body: got %q want %q", got.Data, expected)
	}
}

func TestExecuteReadsAtMostMaxBytesPlusOne(t *testing.T) {
	const maxBytes = 5
	body := []byte("abcdefghijklmnopqrstuvwxyz")
	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: &limitReadCloser{
					data:        body,
					maxReturned: maxBytes + 1,
				},
				Header: make(http.Header),
			}, nil
		}),
	}

	var got toolsy.Chunk
	err := execute(
		context.Background(),
		toolsy.RunContext{},
		"list_items",
		http.MethodGet,
		"/items",
		nil,
		nil,
		nil,
		[]byte(`{}`),
		&Options{
			BaseURL:          "https://example.com",
			HTTPClient:       client,
			MaxResponseBytes: maxBytes,
		},
		func(c toolsy.Chunk) error {
			got = c
			return nil
		},
	)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	expected := []byte("abcde" + truncationSuffix)
	if !bytes.Equal(got.Data, expected) {
		t.Fatalf("unexpected body: got %q want %q", got.Data, expected)
	}
}
