package openapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

func TestExecuteRejectsOversizedResponse(t *testing.T) {
	for _, contentType := range []string{"text/plain", "application/json"} {
		t.Run(contentType, func(t *testing.T) {
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", contentType)
				_, _ = w.Write([]byte(`{"message":"oversized"}`))
			}))
			defer server.Close()
			yielded := false
			// Act.
			err := execute(
				t.Context(),
				toolsy.NewRunEnv(nil),
				"list_items",
				http.MethodGet,
				"/items",
				&operationContract{},
				[]byte(`{}`),
				&Options{
					BaseURL:          server.URL,
					MaxResponseBytes: 5,
					AllowPrivateIPs:  true,
				},
				func(toolsy.Chunk) error { yielded = true; return nil },
			)
			// Assert.
			if !errors.Is(err, textprocessor.ErrReadLimitExceeded) || yielded {
				t.Fatalf("err=%v yielded=%v", err, yielded)
			}
		})
	}
}
func TestExecuteNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "missing", http.StatusNotFound) },
		),
	)
	defer server.Close()
	err := execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		"list_items",
		http.MethodGet,
		"/items",
		&operationContract{},
		[]byte(`{}`),
		&Options{
			BaseURL:         server.URL,
			AllowPrivateIPs: true,
		},
		func(toolsy.Chunk) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("unexpected error: %v", err)
	}
}
