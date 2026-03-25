package graphql

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

func TestExecuteGraphQLTruncatesOversizedResponse(t *testing.T) {
	body := []byte(`{"data":{"demo":"abcdefghijklmnopqrstuvwxyz"}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Fatalf("read body: %v", err)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	var got toolsy.Chunk
	err := executeGraphQL(
		context.Background(),
		toolsy.RunContext{},
		"graphql_demo",
		server.URL,
		"query { demo }",
		[]byte(`{"id":1}`),
		&Options{
			HTTPClient:       server.Client(),
			MaxResponseBytes: 10,
		},
		func(c toolsy.Chunk) error {
			got = c
			return nil
		},
	)
	if err != nil {
		t.Fatalf("executeGraphQL returned error: %v", err)
	}

	const suffix = "\n[Truncated. Use pagination or filters.]"
	expected := append(append([]byte(nil), body[:10]...), []byte(suffix)...)
	if !bytes.Equal(got.Data, expected) {
		t.Fatalf("unexpected body: got %q want %q", got.Data, expected)
	}
	if got.Event != toolsy.EventResult {
		t.Fatalf("unexpected event: %s", got.Event)
	}
}

func TestExecuteGraphQLTruncatesOversizedResponseUTF8Safely(t *testing.T) {
	body := []byte("приветмир")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Fatalf("read body: %v", err)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	var got toolsy.Chunk
	err := executeGraphQL(
		context.Background(),
		toolsy.RunContext{},
		"graphql_demo",
		server.URL,
		"query { demo }",
		nil,
		&Options{
			HTTPClient:       server.Client(),
			MaxResponseBytes: 5,
		},
		func(c toolsy.Chunk) error {
			got = c
			return nil
		},
	)
	if err != nil {
		t.Fatalf("executeGraphQL returned error: %v", err)
	}

	if !utf8.Valid(got.Data) {
		t.Fatalf("response must remain valid UTF-8: %q", got.Data)
	}
	const suffix = "\n[Truncated. Use pagination or filters.]"
	expected := []byte("пр" + suffix)
	if !bytes.Equal(got.Data, expected) {
		t.Fatalf("unexpected body: got %q want %q", got.Data, expected)
	}
}

func TestExecuteGraphQLReadsAtMostMaxBytesPlusOne(t *testing.T) {
	const maxBytes = 5
	body := []byte("abcdefghijklmnopqrstuvwxyz")
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if _, err := io.ReadAll(r.Body); err != nil {
				return nil, err
			}
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
	err := executeGraphQL(
		context.Background(),
		toolsy.RunContext{},
		"graphql_demo",
		"https://example.com/graphql",
		"query { demo }",
		[]byte(`{"id":1}`),
		&Options{
			HTTPClient:       client,
			MaxResponseBytes: maxBytes,
		},
		func(c toolsy.Chunk) error {
			got = c
			return nil
		},
	)
	if err != nil {
		t.Fatalf("executeGraphQL returned error: %v", err)
	}

	const suffix = "\n[Truncated. Use pagination or filters.]"
	expected := []byte("abcde" + suffix)
	if !bytes.Equal(got.Data, expected) {
		t.Fatalf("unexpected body: got %q want %q", got.Data, expected)
	}
}
