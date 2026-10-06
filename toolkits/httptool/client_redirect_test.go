package httptool

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func redirectRequest(t *testing.T, method, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, rawURL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer origin-secret")
	req.Header.Set("Cookie", "session=secret")
	req.Header.Set("Proxy-Authorization", "Basic secret")
	return req
}

func TestRemoteRedirectOriginAndMethod(t *testing.T) {
	for _, fixture := range []struct {
		name, initial, next, target string
		allowed                     bool
	}{
		{"same-origin-get", "GET", "GET", "https://example.com/next", true},
		{"same-origin-head", "HEAD", "HEAD", "https://example.com/next", true},
		{"default-port", "GET", "GET", "https://EXAMPLE.com:443/next", true},
		{"changed-port", "GET", "GET", "https://example.com:8443/next", false},
		{"subdomain", "GET", "GET", "https://api.example.com/next", false},
		{"downgrade", "GET", "GET", "http://example.com/next", false},
		{"upgrade", "GET", "GET", "https://example.com/next", false},
		{"post-replay", "POST", "POST", "https://example.com/next", false},
		{"post-rewritten", "POST", "GET", "https://example.com/next", false},
		{"put-rewritten", "PUT", "GET", "https://example.com/next", false},
		{"patch-replay", "PATCH", "PATCH", "https://example.com/next", false},
		{"delete-replay", "DELETE", "DELETE", "https://example.com/next", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			initialURL := "https://example.com/start"
			if fixture.name == "upgrade" {
				initialURL = "http://example.com/start"
			}
			first := redirectRequest(t, fixture.initial, initialURL)
			next := redirectRequest(t, fixture.next, fixture.target)
			// Act.
			err := CheckRedirectRemote(true, nil)(next, []*http.Request{first})
			// Assert.
			if fixture.allowed {
				require.NoError(t, err)
				require.Equal(t, "Bearer origin-secret", next.Header.Get("Authorization"))
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestAllowedRedirectCredentialsAndMethod(t *testing.T) {
	for _, target := range []string{
		"https://example.com:8443/next", "https://api.example.com/next", "http://example.com/next",
	} {
		t.Run(target, func(t *testing.T) {
			// Arrange.
			first := redirectRequest(t, http.MethodGet, "https://example.com/start")
			next := redirectRequest(t, http.MethodGet, target)
			// Adversarial caller map with a noncanonical key.
			next.Header["authorization"] = []string{
				"Bearer lower-case-secret",
			}
			policy := CheckRedirectAllowed([]string{"example.com", ".example.com"}, true)
			// Act.
			err := policy(next, []*http.Request{first})
			// Assert.
			require.NoError(t, err)
			for _, header := range []string{"Authorization", "Cookie", "Proxy-Authorization"} {
				require.Empty(t, next.Header.Get(header), header)
			}
			require.NotContains(t, next.Header, "authorization")
		})
	}
	for _, nextMethod := range []string{http.MethodGet, http.MethodPost} {
		// Arrange.
		first := redirectRequest(t, http.MethodPost, "https://example.com/start")
		next := redirectRequest(t, nextMethod, "https://example.com/next")
		// Act.
		err := CheckRedirectAllowed([]string{"example.com"}, true)(next, []*http.Request{first})
		// Assert.
		require.Error(t, err)
	}
}

func TestRedirectSafetyGuards(t *testing.T) {
	for _, policy := range []func(*http.Request, []*http.Request) error{
		CheckRedirectAllowed([]string{"example.com"}, false), CheckRedirectRemote(false, []string{"blocked.example.com"}),
	} {
		for _, target := range []string{
			"http://127.0.0.1/next", "ftp://example.com/next", "http://user:secret@example.com/next",
		} {
			// Arrange.
			first := redirectRequest(t, http.MethodGet, target)
			next := redirectRequest(t, http.MethodGet, target)
			// Act.
			err := policy(next, []*http.Request{first})
			// Assert.
			require.Error(t, err)
		}
		// Arrange: a redirect chain at the existing bound.
		first := redirectRequest(t, http.MethodGet, "https://example.com/start")
		next := redirectRequest(t, http.MethodGet, "https://example.com/next")
		via := make([]*http.Request, maxRedirects)
		for i := range via {
			via[i] = first
		}
		// Act / Assert.
		require.Error(t, policy(next, via))
	}
	// Arrange: the remote blacklist remains enforced even for same-origin reads.
	first := redirectRequest(t, http.MethodGet, "http://blocked.example.com/start")
	next := redirectRequest(t, http.MethodGet, "http://blocked.example.com/next")
	// Act / Assert.
	require.Error(t, CheckRedirectRemote(true, []string{"blocked.example.com"})(next, []*http.Request{first}))
}

func TestOriginNormalizesSchemeAndDefaultPort(t *testing.T) {
	// Arrange.
	upper := &url.URL{Scheme: "HTTPS", Host: "EXAMPLE.com"}
	lower := &url.URL{Scheme: "https", Host: "example.com:443"}
	// Act / Assert.
	require.Equal(t, origin(lower), origin(upper))
}

func TestRedirectRefusalClassification(t *testing.T) {
	for _, fixture := range []struct {
		name, target string
		policy       func(*http.Request, []*http.Request) error
		cause        bool
	}{
		{"userinfo", "http://user:secret@example.com/next", CheckRedirectRemote(true, nil), true},
		{"private-IP", "http://127.0.0.1/next", CheckRedirectRemote(false, nil), true},
		{"blocked-host", "http://example.com/next", CheckRedirectRemote(true, []string{"example.com"}), false},
		{"not-allowed", "http://example.com/next", CheckRedirectAllowed([]string{"other.test"}, true), true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			first := redirectRequest(t, http.MethodGet, fixture.target)
			next := redirectRequest(t, http.MethodGet, fixture.target)
			// Act.
			err := fixture.policy(next, []*http.Request{first})
			// Assert: nested validation causes must not authorize input repair.
			var refused *RedirectError
			require.ErrorAs(t, err, &refused)
			te, ok := toolsy.AsToolError(err)
			require.True(t, ok)
			require.Equal(t, toolsy.CodeRemoteExecution, te.Code)
			require.False(t, te.Retryable)
			require.False(t, toolsy.ClientCorrectable(te.Code))
			require.NotContains(t, refused.Error(), "secret")
			if fixture.cause {
				require.ErrorIs(t, err, toolsy.ErrValidation)
			}
		})
	}
}
