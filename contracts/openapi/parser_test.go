package openapi

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

type byteRepeatReader struct{}

func (byteRepeatReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

func TestParseURL_ExceedsSpecSizeLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("large spec body skipped in -short")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.Copy(w, io.LimitReader(byteRepeatReader{}, int64(defaultMaxSpecBytes)+1))
	}))
	defer server.Close()

	_, err := ParseURL(context.Background(), server.URL, Options{
		HTTPClient:      server.Client(),
		AllowPrivateIPs: true,
	})
	if err == nil {
		t.Fatal("expected error for oversized spec")
	}
	if !errors.Is(err, textprocessor.ErrReadLimitExceeded) {
		t.Fatalf("expected ErrReadLimitExceeded in chain, got: %v", err)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(defaultMaxSpecBytes)) {
		t.Fatalf("expected limit in error message, got: %v", err)
	}
}

func TestParseURL_CancelOverReadLimit_InterruptWins(t *testing.T) {
	if testing.Short() {
		t.Skip("large spec body skipped in -short")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.Copy(w, io.LimitReader(byteRepeatReader{}, int64(defaultMaxSpecBytes)+1))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ParseURL(ctx, server.URL, Options{
		HTTPClient:      server.Client(),
		AllowPrivateIPs: true,
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

func TestParseURL_InterruptInChainOverReadLimit_InterruptWins(t *testing.T) {
	composite := fmt.Errorf(
		"read: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	err := mapOpenAPIReadError(context.Background(), composite)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func mapOpenAPIReadError(ctx context.Context, err error) error {
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
		return fmt.Errorf("openapi: spec exceeds %d byte limit: %w", defaultMaxSpecBytes, err)
	}
	return fmt.Errorf("openapi: read spec: %w", err)
}

func TestParseURL_Non2xxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := ParseURL(context.Background(), server.URL, Options{
		HTTPClient:      server.Client(),
		AllowPrivateIPs: true,
	})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 in error, got: %v", err)
	}
}
