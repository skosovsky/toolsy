package graphql

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

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
		"demo",
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
