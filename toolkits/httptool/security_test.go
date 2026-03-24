package httptool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestValidateURL_DomainAllowed(t *testing.T) {
	// allowPrivateIPs=true to avoid DNS lookup in test (we only assert domain whitelist)
	u, err := validateURL(context.Background(), "https://api.example.com/path", []string{"api.example.com"}, true)
	require.NoError(t, err)
	require.Equal(t, "https", u.Scheme)
	require.Equal(t, "api.example.com", u.Hostname())
}

func TestValidateURL_DomainNotAllowed(t *testing.T) {
	_, err := validateURL(context.Background(), "https://evil.com/", []string{"api.example.com"}, false)
	require.Error(t, err)
	var ce *toolsy.ClientError
	require.True(t, toolsy.IsClientError(err))
	require.ErrorAs(t, err, &ce)
	require.Contains(t, ce.Reason, "domain not allowed")
}

func TestValidateURL_EmptyAllowedDomains(t *testing.T) {
	_, err := validateURL(context.Background(), "https://api.example.com/", []string{}, false)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "no allowed domains configured")
}

func TestValidateURL_InvalidScheme(t *testing.T) {
	_, err := validateURL(context.Background(), "file:///etc/passwd", []string{"localhost"}, false)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "only http and https")
}

func TestValidateURL_InvalidURL(t *testing.T) {
	_, err := validateURL(context.Background(), "://no-scheme", []string{"x"}, false)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
}

func TestValidateURL_CaseInsensitiveDomain(t *testing.T) {
	u, err := validateURL(context.Background(), "https://API.Example.COM/", []string{"api.example.com"}, true)
	require.NoError(t, err)
	require.Equal(t, "API.Example.COM", u.Hostname()) // URL preserves original case; matching is case-insensitive
}

func TestValidateURL_WildcardSubdomain(t *testing.T) {
	u, err := validateURL(context.Background(), "https://api.slack.com/", []string{".slack.com"}, true)
	require.NoError(t, err)
	require.Equal(t, "api.slack.com", u.Hostname())

	u2, err := validateURL(context.Background(), "https://hooks.slack.com/", []string{".slack.com"}, true)
	require.NoError(t, err)
	require.Equal(t, "hooks.slack.com", u2.Hostname())
}

func TestValidateURL_WildcardDoesNotMatchBareDomain(t *testing.T) {
	_, err := validateURL(context.Background(), "https://slack.com/", []string{".slack.com"}, false)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "domain not allowed")
}

func TestValidateURL_WildcardDoesNotMatchSuffixOnly(t *testing.T) {
	_, err := validateURL(context.Background(), "https://evil-slack.com/", []string{".slack.com"}, false)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
}

func TestValidateURL_LocalhostBlocked(t *testing.T) {
	_, err := validateURL(context.Background(), "http://127.0.0.1/", []string{"127.0.0.1"}, false)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "private IP")
}

func TestValidateURL_PrivateIPBlocked(t *testing.T) {
	_, err := validateURL(context.Background(), "http://169.254.169.254/", []string{"169.254.169.254"}, false)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "private IP")
}

func TestValidateURL_AllowPrivateIPsForTests(t *testing.T) {
	u, err := validateURL(context.Background(), "http://127.0.0.1/", []string{"127.0.0.1"}, true)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", u.Hostname())
}
