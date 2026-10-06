package web

import (
	"context"
	"net/http"
	"net/url"

	"github.com/skosovsky/toolsy/toolkits/httptool"
)

// validateScrapeURL checks scheme, host, IP policy, and blockedDomains. Returns parsed URL for use in request.
func validateScrapeURL(
	ctx context.Context,
	rawURL string,
	allowPrivateIPs bool,
	blockedDomains []string,
) (*url.URL, error) {
	return httptool.ValidateRemoteURLWithBlacklist(ctx, rawURL, allowPrivateIPs, blockedDomains)
}

func newScrapeHTTPClient(o *options) (*http.Client, error) {
	return httptool.NewConfiguredSafeHTTPClient(
		httptool.SafeDialOptions{ //nolint:exhaustruct_v5 // Unset fields retain safe defaults.
			BlockedHosts:    o.blockedDomains,
			AllowPrivateIPs: o.allowPrivateIPs,
		},
		httptool.CheckRedirectRemote(o.allowPrivateIPs, o.blockedDomains),
		o.httpSettings,
	)
}
