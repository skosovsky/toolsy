package httptool

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSafeHTTPSettingsTLSAndOwnership(t *testing.T) {
	// Arrange: custom roots must actually be used without replacing safe dialing.
	server := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }),
	)
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	settings := ClientSettings{
		Timeout:   time.Second,
		TLSConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
	}
	client, err := NewConfiguredSafeHTTPClient(SafeDialOptions{AllowPrivateIPs: true}, nil, settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	// Act: mutate only the original config's scalar; retained root/callback objects stay immutable.
	settings.TLSConfig.MinVersion = tls.VersionTLS13
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	// Assert.
	if err != nil || string(raw) != "ok" {
		t.Fatalf("body=%q err=%v", raw, err)
	}
	transport := client.Transport.(*http.Transport)
	if transport.TLSClientConfig == settings.TLSConfig || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatal("TLS configuration was not snapshotted")
	}
	if transport.IdleConnTimeout <= 0 || transport.MaxIdleConns <= 0 || transport.MaxIdleConnsPerHost <= 0 {
		t.Fatal("idle policy is unbounded")
	}
	if transport.Proxy != nil || transport.DialTLSContext != nil {
		t.Fatal("safe pinned dial bypass")
	}
}

func TestSafeHTTPSettingsRejectNegativeTimeout(t *testing.T) {
	// Arrange / Act.
	client, err := NewConfiguredSafeHTTPClient(SafeDialOptions{}, nil, ClientSettings{Timeout: -time.Second})
	// Assert.
	if err == nil || client != nil {
		t.Fatalf("client=%v err=%v", client, err)
	}
}
