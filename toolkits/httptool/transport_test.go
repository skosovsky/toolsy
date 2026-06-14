package httptool

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPrivateIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"loopback", "127.0.0.1", true},
		{"private", "10.0.0.1", true},
		{"link local", "169.254.1.1", true},
		{"public", "8.8.8.8", false},
		{"nil", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var ip net.IP
			if tt.ip != "" {
				ip = net.ParseIP(tt.ip)
			}
			assert.Equal(t, tt.want, IsPrivateIP(ip))
		})
	}
}

func TestIsBlockedIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"loopback", "127.0.0.1", true},
		{"private", "192.168.1.1", true},
		{"multicast", "224.0.0.1", true},
		{"zero v4", "0.0.0.0", true},
		{"zero v6", "::", true},
		{"public", "1.1.1.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			assert.Equal(t, tt.want, IsBlockedIP(ip))
		})
	}
}

func TestHostAllowed_WhitelistMode(t *testing.T) {
	t.Parallel()
	policy := normalizeHostPolicy([]string{"api.example.com"}, nil)
	assert.True(t, hostAllowed("api.example.com", policy))
	assert.True(t, hostAllowed("API.example.com", policy))
	assert.False(t, hostAllowed("evil.com", policy))
}

func TestHostAllowed_BlacklistMode(t *testing.T) {
	t.Parallel()
	policy := normalizeHostPolicy(nil, []string{"internal.corp"})
	assert.False(t, hostAllowed("internal.corp", policy))
	assert.False(t, hostAllowed("sub.internal.corp", policy))
	assert.True(t, hostAllowed("example.com", policy))
}

func TestHostAllowed_IntersectionFailClosed(t *testing.T) {
	t.Parallel()
	policy := normalizeHostPolicy([]string{"example.com"}, []string{"example.com"})
	assert.False(t, hostAllowed("example.com", policy), "host in both lists must be denied")
}

func TestSafeDialTransport_WhitelistAllowsPublicHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	host := hostFromTestServer(t, srv)

	client := &http.Client{
		Transport: SafeDialTransport(SafeDialOptions{
			AllowedHosts:    []string{host},
			AllowPrivateIPs: true,
		}),
	}
	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSafeDialTransport_WhitelistBlocksUnlistedHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{
		Transport: SafeDialTransport(SafeDialOptions{
			AllowedHosts:    []string{"other.example.com"},
			AllowPrivateIPs: true,
		}),
	}
	_, err := client.Get(srv.URL)
	require.Error(t, err)
}

func TestSafeDialTransport_BlacklistBlocksHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host := hostFromTestServer(t, srv)

	client := &http.Client{
		Transport: SafeDialTransport(SafeDialOptions{
			BlockedHosts:    []string{host},
			AllowPrivateIPs: true,
		}),
	}
	_, err := client.Get(srv.URL)
	require.Error(t, err)
}

func TestSafeDialTransport_BlocksPrivateIPWithoutAllow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host := hostFromTestServer(t, srv)

	client := &http.Client{
		Transport: SafeDialTransport(SafeDialOptions{
			AllowedHosts: []string{host},
		}),
	}
	_, err := client.Get(srv.URL)
	require.Error(t, err)
}

func hostFromTestServer(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	host, _, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	return host
}

func TestSafeDialContext_CancelledContext(t *testing.T) {
	policy := normalizeHostPolicy(nil, nil)
	dial := safeDialContext(IsBlockedIP, true, 0, policy)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := dial(ctx, "tcp", "example.com:80")
	require.Error(t, err)
}

func TestSafeDialTransport_IntersectionIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host := hostFromTestServer(t, srv)
	client := &http.Client{
		Transport: SafeDialTransport(SafeDialOptions{
			AllowedHosts:    []string{host},
			BlockedHosts:    []string{host},
			AllowPrivateIPs: true,
		}),
	}
	_, err := client.Get(srv.URL)
	require.Error(t, err)
}

func TestSafeDialTransport_DNSPinBlocksPrivateAtDial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	host := hostFromTestServer(t, srv)
	client := NewSafeHTTPClient(SafeDialOptions{
		AllowedHosts: []string{host},
	}, nil)
	_, err := client.Get(srv.URL)
	require.Error(t, err)
}
