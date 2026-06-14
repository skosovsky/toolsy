package openapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/skosovsky/toolsy"
)

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
		toolsy.NewRunEnv(nil),
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
			AllowPrivateIPs:  true,
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
		toolsy.NewRunEnv(nil),
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
			AllowPrivateIPs:  true,
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.Copy(w, &limitReadCloser{
			data:        body,
			maxReturned: maxBytes + 1,
		})
	}))
	defer server.Close()

	var got toolsy.Chunk
	err := execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
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
			MaxResponseBytes: maxBytes,
			AllowPrivateIPs:  true,
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

func TestExecute_Non2xxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	err := execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		"list_items",
		http.MethodGet,
		"/items",
		nil,
		nil,
		nil,
		[]byte(`{}`),
		&Options{
			BaseURL:         server.URL,
			HTTPClient:      server.Client(),
			AllowPrivateIPs: true,
		},
		func(toolsy.Chunk) error { return nil },
	)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 in error, got: %v", err)
	}
}
