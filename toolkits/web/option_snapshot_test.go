package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type d32SearchCounter struct{ calls atomic.Int32 }

func (p *d32SearchCounter) Search(context.Context, string) ([]SearchResult, error) {
	p.calls.Add(1)
	return nil, nil
}

func TestD32WebLibraryRejectsBeforeIO(t *testing.T) {
	// Arrange.
	provider := &d32SearchCounter{}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("<p>ok</p>"))
	}))
	defer server.Close()
	for _, opt := range []Option{nil, WithMaxSearchResults(-1), WithMaxSearchItemBytes(-1), WithMaxSearchSourceBytes(-1), WithMaxPageBytes(-1), WithMaxSearchBytes(-1), WithMaxSourceBytes(-1), WithMaxMarkdownBytes(-1)} {
		// Act.
		require.NotPanics(t, func() {
			results, err := SearchStructured(t.Context(), provider, "query", opt)
			// Assert.
			require.Error(t, err)
			require.Nil(t, results)
			content, err := ScrapePage(t.Context(), server.URL, WithAllowPrivateIPs(true), opt)
			require.Error(t, err)
			require.Empty(t, content)
		})
	}
	require.Zero(t, provider.calls.Load())
	require.Zero(t, requests.Load())
}

func TestD32BlockedDomainsSnapshot(t *testing.T) {
	// Arrange: mutate the source after option creation, before its first use.
	domains := []string{"127.0.0.1"}
	option := WithBlockedDomains(domains)
	domains[0] = "unrelated.invalid"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("<p>ok</p>"))
	}))
	defer server.Close()
	// Act: reused option must still block the original host in both configurations.
	for range 2 {
		content, err := ScrapePage(t.Context(), server.URL, WithAllowPrivateIPs(true), option)
		// Assert.
		require.Error(t, err)
		require.Empty(t, content)
	}
	require.Zero(t, requests.Load())
	// Materializing another config cannot borrow a previous config's mutable list.
	var first options
	option(&first)
	first.blockedDomains[0] = "changed.invalid"
	var second options
	option(&second)
	require.Equal(t, []string{"127.0.0.1"}, second.blockedDomains)
}
