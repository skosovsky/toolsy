package agents

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPClientMerge_AppliesTimeoutKeepsSafeTransport(t *testing.T) {
	unsafe := &http.Client{
		Transport: http.DefaultTransport,
		Timeout:   5 * time.Second,
	}
	c := NewClient("http://example.com", WithHTTPClient(unsafe))
	client := c.httpClient()
	require.NotSame(t, http.DefaultTransport, client.Transport)
	require.Equal(t, 5*time.Second, client.Timeout)
}

func TestHTTPClientMerge_BlocksPrivateIPDial(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", WithHTTPClient(&http.Client{
		Transport: http.DefaultTransport,
		Timeout:   time.Second,
	}))
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:1", nil)
	require.NoError(t, err)
	_, err = c.httpClient().Do(req)
	require.Error(t, err)
}
