package agents

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func mustClient(t *testing.T, url string, opts ...func(*ClientOptions)) *Client {
	t.Helper()
	client, err := NewClient(url, opts...)
	require.NoError(t, err)
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func TestHTTPSettingsApplyTimeoutAndSafeTransport(t *testing.T) {
	// Arrange.
	c := mustClient(t, "http://example.com", WithHTTPSettings(httptool.ClientSettings{Timeout: 5 * time.Second}))
	// Act.
	client := c.httpClient()
	// Assert.
	require.NotSame(t, http.DefaultTransport, client.Transport)
	require.Equal(t, 5*time.Second, client.Timeout)
	require.Same(t, client, c.httpClient())
}

func TestHTTPSettingsBlockPrivateIPDial(t *testing.T) {
	// Arrange.
	c := mustClient(t, "http://127.0.0.1:1", WithHTTPSettings(httptool.ClientSettings{Timeout: time.Second}))
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:1", nil)
	require.NoError(t, err)
	// Act.
	_, err = c.httpClient().Do(req)
	// Assert.
	require.Error(t, err)
}
