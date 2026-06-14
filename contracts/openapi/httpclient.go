package openapi

import (
	"net/http"

	"github.com/skosovsky/toolsy/toolkits/httptool"
)

const defaultMaxSpecBytes = 8 * 1024 * 1024

func defaultHTTPClient(allowPrivateIPs bool) *http.Client {
	return httptool.NewSafeHTTPClient(
		httptool.SafeDialOptions{AllowPrivateIPs: allowPrivateIPs}, //nolint:exhaustruct // blacklist mode when false
		httptool.CheckRedirectRemote(allowPrivateIPs, nil),
	)
}

func resolveHTTPClient(custom HTTPClient, allowPrivateIPs bool) HTTPClient {
	return httptool.MergeHTTPClient(defaultHTTPClient(allowPrivateIPs), custom)
}
