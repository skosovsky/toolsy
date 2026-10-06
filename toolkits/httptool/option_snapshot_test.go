package httptool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestD32HTTPPolicySnapshots(t *testing.T) {
	// Arrange.
	domains := []string{"api.example.com"}
	origins := []string{"https://api.example.com"}
	headers := map[string]string{"X-Metadata": "original"}
	options := []Option{WithAllowedDomains(domains), WithCredentialOrigins(origins), WithHeaders(headers)}
	domains[0] = "attacker.example"
	origins[0] = "https://attacker.example"
	headers["X-Metadata"] = "changed"
	// Act.
	var first optionsFixture
	first.apply(options)
	require.NoError(t, normalizeCredentialOrigins(&first.o))
	// Assert: use the actual host matcher and credential origin normalization.
	require.True(t, HostMatchesAllowedDomains("api.example.com", first.o.allowedDomains))
	require.False(t, HostMatchesAllowedDomains("attacker.example", first.o.allowedDomains))
	require.Equal(t, []string{"https://api.example.com:443"}, first.o.credentialOrigins)
	require.Equal(t, "original", first.o.headers["X-Metadata"])
	first.o.allowedDomains[0] = "changed.invalid"
	first.o.credentialOrigins[0] = "https://changed.invalid:443"
	first.o.headers["X-Metadata"] = "first-only"
	var second optionsFixture
	second.apply(options)
	require.NoError(t, normalizeCredentialOrigins(&second.o))
	require.True(t, HostMatchesAllowedDomains("api.example.com", second.o.allowedDomains))
	require.Equal(t, []string{"https://api.example.com:443"}, second.o.credentialOrigins)
	require.Equal(t, "original", second.o.headers["X-Metadata"])
}

type optionsFixture struct{ o options }

func (f *optionsFixture) apply(opts []Option) {
	for _, opt := range opts {
		opt(&f.o)
	}
}
