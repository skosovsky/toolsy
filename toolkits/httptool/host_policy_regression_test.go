package httptool

import (
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestHostPolicyDenyPrecedence(t *testing.T) {
	for _, fixture := range []struct {
		name, host       string
		allowed, blocked []string
		permit           bool
	}{
		{"identical-exact", "example.com", []string{"example.com"}, []string{"example.com"}, false},
		{"identical-suffix", "api.example.com", []string{".example.com"}, []string{".example.com"}, false},
		{"broad-allow-narrow-deny", "private.example.com", []string{".example.com"}, []string{"private.example.com"}, false},
		{"allowed-sibling", "public.example.com", []string{".example.com"}, []string{"private.example.com"}, true},
		{"apex-not-in-suffix", "example.com", []string{".example.com"}, nil, false},
		{"exact-not-descendants", "api.example.com", []string{"example.com"}, nil, false},
		{"deny-suffix-not-apex", "example.com", []string{"example.com"}, []string{".example.com"}, true},
		{"exact-allow-suffix-deny", "api.example.com", []string{"api.example.com"}, []string{".example.com"}, false},
		{"normalization", " API.Example.COM. ", []string{" .Example.COM. "}, []string{" API.example.com. "}, false},
		{"blacklist-exact", "example.com", nil, []string{"example.com"}, false},
		{"blacklist-exact-sibling", "api.example.com", nil, []string{"example.com"}, true},
		{"blacklist-suffix", "api.example.com", nil, []string{".example.com"}, false},
		{"blank-allow-is-deny", "example.com", []string{" "}, nil, false},
		{"repeated-root-dot", "example.com..", nil, []string{"example.com"}, false},
		{"empty-label", "api..example.com", nil, nil, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			policy := normalizeHostPolicy(fixture.allowed, fixture.blocked)
			// Act.
			permitted := hostAllowed(fixture.host, policy)
			// Assert.
			require.Equal(t, fixture.permit, permitted)
		})
	}
}

func TestSafeTransportRejectsOverlapBeforeDNS(t *testing.T) {
	for _, blocked := range [][]string{{".example.invalid"}, {"api.example.invalid"}} {
		// Arrange: .invalid cannot resolve; rejection must be the host-policy error,
		// not a resolver error or a connection attempt. Public DialContext is used.
		transport := SafeDialTransport(SafeDialOptions{
			AllowedHosts: []string{".example.invalid"}, BlockedHosts: blocked, AllowPrivateIPs: true,
		})
		// Act.
		conn, err := transport.DialContext(t.Context(), "tcp", net.JoinHostPort("api.example.invalid", "80"))
		// Assert.
		if conn != nil {
			_ = conn.Close()
			t.Fatal("denied host was dialed")
		}
		require.ErrorIs(t, err, toolsy.ErrValidation)
		require.ErrorContains(t, err, "host not allowed")
	}
}

func TestHostSyntaxSharedAcrossHelpers(t *testing.T) {
	for _, fixture := range []struct {
		host, pattern string
		match         bool
	}{
		{"example.com", "example.com", true},
		{"api.example.com", "example.com", false},
		{"example.com", ".example.com", false},
		{"api.example.com", ".example.com", true},
		{"evil-example.com", ".example.com", false},
		{"a.b.example.com", ".example.com", true},
		{" API.Example.COM. ", " api.example.com ", true},
		{"API.Example.COM.", " .example.com. ", true},
		{"", "", false},
		{"example.com", ".", false},
		{"example.com..", "example.com..", false},
		{"api..example.com", ".example.com", false},
	} {
		t.Run(fixture.host+"/"+fixture.pattern, func(t *testing.T) {
			// Arrange.
			entries := []string{fixture.pattern}
			// Act / Assert.
			require.Equal(t, fixture.match, MatchHost(fixture.host, fixture.pattern))
			require.Equal(t, fixture.match, HostBlocked(fixture.host, entries))
			require.Equal(t, fixture.match, HostMatchesAllowedDomains(fixture.host, entries))
		})
	}
}

func TestBlacklistURLAndRedirectRejectBeforeDNS(t *testing.T) {
	// Arrange: lookup of this .invalid hostname would fail, so the exact host
	// denial cause demonstrates that neither public path reached DNS resolution.
	const endpoint = "http://api.example.invalid/next"
	blocked := []string{".example.invalid"}
	first := redirectRequest(t, http.MethodGet, endpoint)
	next := redirectRequest(t, http.MethodGet, endpoint)
	// Act.
	_, initialErr := ValidateRemoteURLWithBlacklist(t.Context(), endpoint, false, blocked)
	redirectErr := CheckRedirectRemote(false, blocked)(next, []*http.Request{first})
	// Assert.
	require.ErrorContains(t, initialErr, "domain is blocked")
	var refused *RedirectError
	require.ErrorAs(t, redirectErr, &refused)
	require.ErrorContains(t, refused.Unwrap(), "domain is blocked")
	te, ok := toolsy.AsToolError(redirectErr)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeRemoteExecution, te.Code)
	require.False(t, toolsy.ClientCorrectable(te.Code))
}
