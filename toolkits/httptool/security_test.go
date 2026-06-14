package httptool

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
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
	var ce *toolsy.ToolError
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.ErrorAs(t, err, &ce)
	require.Contains(t, ce.Reason, "domain not allowed")
}

func TestValidateURL_EmptyAllowedDomains(t *testing.T) {
	_, err := validateURL(context.Background(), "https://api.example.com/", []string{}, false)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "no allowed domains configured")
}

func TestValidateURL_InvalidScheme(t *testing.T) {
	_, err := validateURL(context.Background(), "file:///etc/passwd", []string{"localhost"}, false)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "only http and https")
}

func TestValidateURL_InvalidURL(t *testing.T) {
	_, err := validateURL(context.Background(), "://no-scheme", []string{"x"}, false)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
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
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "domain not allowed")
}

func TestValidateURL_WildcardDoesNotMatchSuffixOnly(t *testing.T) {
	_, err := validateURL(context.Background(), "https://evil-slack.com/", []string{".slack.com"}, false)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
}

func TestValidateURL_LocalhostBlocked(t *testing.T) {
	_, err := validateURL(context.Background(), "http://127.0.0.1/", []string{"127.0.0.1"}, false)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "private or loopback IP")
}

func TestValidateURL_ZeroIPBlocked(t *testing.T) {
	addrs := []net.IPAddr{{IP: net.ParseIP("0.0.0.0")}}
	err := ValidateResolvedIPs(addrs, false)
	require.Error(t, err)
}

func TestValidateURL_MulticastBlocked(t *testing.T) {
	_, err := validateURL(context.Background(), "http://224.0.0.1/", []string{"224.0.0.1"}, false)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Contains(t, te.Reason, "private or loopback IP")
}

func TestValidateURL_PrivateIPBlocked(t *testing.T) {
	_, err := validateURL(context.Background(), "http://169.254.169.254/", []string{"169.254.169.254"}, false)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "private or loopback IP")
}

func TestValidateURL_AllowPrivateIPsForTests(t *testing.T) {
	u, err := validateURL(context.Background(), "http://127.0.0.1/", []string{"127.0.0.1"}, true)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", u.Hostname())
}

func TestIsPrivateIP_ExportedMatchesValidateURL(t *testing.T) {
	require.True(t, IsPrivateIP(net.ParseIP("10.0.0.1")))
	require.False(t, IsPrivateIP(net.ParseIP("8.8.8.8")))
}

func TestIsBlockedIP_Exported(t *testing.T) {
	require.True(t, IsBlockedIP(net.ParseIP("127.0.0.1")))
	require.True(t, IsBlockedIP(net.ParseIP("224.0.0.1")))
	require.False(t, IsBlockedIP(net.ParseIP("8.8.8.8")))
}
