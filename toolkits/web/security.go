package web

import (
	"context"
	"errors"
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

func scrapeHTTPClient(o *options) (*http.Client, error) {
	if o.httpClient != nil {
		if _, ok := o.httpClient.(*http.Client); !ok {
			return nil, errors.New(
				"toolkit/web: default SSRF protection requires *http.Client; pass WithHTTPClient(&http.Client{...})",
			)
		}
	}
	dialOpts := httptool.SafeDialOptions{ //nolint:exhaustruct // blacklist mode; IP policy via AllowPrivateIPs
		BlockedHosts:    o.blockedDomains,
		AllowPrivateIPs: o.allowPrivateIPs,
	}
	safe := httptool.NewSafeHTTPClient(
		dialOpts,
		httptool.CheckRedirectRemote(o.allowPrivateIPs, o.blockedDomains),
	)
	return httptool.MergeHTTPClient(safe, o.httpClient), nil
}
