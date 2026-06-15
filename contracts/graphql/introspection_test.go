package graphql

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
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
		toolsy.NewRunEnv(nil),
		"graphql_demo",
		server.URL,
		"query { demo }",
		[]byte(`{"id":1}`),
		&Options{
			HTTPClient:       server.Client(),
			MaxResponseBytes: 10,
			AllowPrivateIPs:  true,
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
		toolsy.NewRunEnv(nil),
		"graphql_demo",
		server.URL,
		"query { demo }",
		nil,
		&Options{
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Fatalf("read body: %v", err)
		}
		_, _ = io.Copy(w, &limitReadCloser{
			data:        body,
			maxReturned: maxBytes + 1,
		})
	}))
	defer server.Close()

	var got toolsy.Chunk
	err := executeGraphQL(
		context.Background(),
		toolsy.NewRunEnv(nil),
		"graphql_demo",
		server.URL,
		"query { demo }",
		[]byte(`{"id":1}`),
		&Options{
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
		t.Fatalf("executeGraphQL returned error: %v", err)
	}

	const suffix = "\n[Truncated. Use pagination or filters.]"
	expected := []byte("abcde" + suffix)
	if !bytes.Equal(got.Data, expected) {
		t.Fatalf("unexpected body: got %q want %q", got.Data, expected)
	}
}

func TestPostIntrospection_ExceedsResponseLimit(t *testing.T) {
	const maxBytes = 20
	body := strings.Repeat("x", 100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	_, err := postIntrospection(context.Background(), server.URL, Options{
		HTTPClient:       server.Client(),
		MaxResponseBytes: maxBytes,
		AllowPrivateIPs:  true,
	})
	if err == nil {
		t.Fatal("expected error for oversized introspection response")
	}
	if !errors.Is(err, textprocessor.ErrReadLimitExceeded) {
		t.Fatalf("expected ErrReadLimitExceeded in chain, got: %v", err)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(maxBytes)) {
		t.Fatalf("expected limit in error message, got: %v", err)
	}
}

func TestPostIntrospection_CancelOverReadLimit_InterruptWins(t *testing.T) {
	body := strings.Repeat("x", 100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := postIntrospection(ctx, server.URL, Options{
		HTTPClient:       server.Client(),
		MaxResponseBytes: 10,
		AllowPrivateIPs:  true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if errors.Is(err, textprocessor.ErrReadLimitExceeded) {
		t.Fatalf("read limit must not win over cancel: %v", err)
	}
}

func TestPostIntrospection_InterruptInChainOverReadLimit_InterruptWins(t *testing.T) {
	composite := fmt.Errorf(
		"read: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	err := mapGraphQLReadError(context.Background(), composite, 4096)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func mapGraphQLReadError(ctx context.Context, err error, maxBytes int) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if toolsy.IsContextInterrupt(err) {
		return err
	}
	if textprocessor.IsReadLimitExceeded(err) {
		return fmt.Errorf("graphql: introspection exceeds %d byte limit: %w", maxBytes, err)
	}
	return fmt.Errorf("graphql: read: %w", err)
}

func TestPostIntrospection_Non2xxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := postIntrospection(
		context.Background(),
		server.URL,
		Options{HTTPClient: server.Client(), AllowPrivateIPs: true},
	)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 in error, got: %v", err)
	}
}

func TestExecuteGraphQL_Non2xxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, readErr := io.ReadAll(r.Body); readErr != nil {
			t.Fatalf("read body: %v", readErr)
		}
		http.Error(w, "fail", http.StatusBadGateway)
	}))
	defer server.Close()

	err := executeGraphQL(
		context.Background(),
		toolsy.NewRunEnv(nil),
		"graphql_demo",
		server.URL,
		"query { demo }",
		nil,
		&Options{HTTPClient: server.Client(), AllowPrivateIPs: true},
		func(toolsy.Chunk) error { return nil },
	)
	if err == nil {
		t.Fatal("expected error for 502 response")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected 502 in error, got: %v", err)
	}
}
