package agents

import (
	"net/http"

	"github.com/skosovsky/toolsy/toolkits/httptool"
)

const defaultMaxResponseBytes = 4 * 1024 * 1024

func defaultHTTPClient(allowPrivateIPs bool) *http.Client {
	return httptool.NewSafeHTTPClient(
		httptool.SafeDialOptions{AllowPrivateIPs: allowPrivateIPs}, //nolint:exhaustruct // blacklist mode when false
		httptool.CheckRedirectRemote(allowPrivateIPs, nil),
	)
}
