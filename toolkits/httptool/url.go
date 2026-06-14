package httptool

import (
	"net/url"
	"strings"

	"github.com/skosovsky/toolsy"
)

// parseHTTPURL parses rawURL and validates http/https scheme and non-empty host.
func parseHTTPURL(rawURL string) (*url.URL, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", toolsy.NewValidationError("invalid URL: " + err.Error())
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, "", toolsy.NewValidationError("only http and https schemes are allowed")
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return nil, "", toolsy.NewValidationError("URL host is missing")
	}
	return u, strings.ToLower(host), nil
}
