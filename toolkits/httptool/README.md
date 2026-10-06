# Toolsy: HTTP Toolkit (safe GET/POST for agents)

**Description:** Lets the agent perform HTTP GET and POST requests to external APIs with SSRF protection (allowed domains whitelist, optional private IP checks) and configurable response body limits (fail-closed).

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/httptool
```

**Dependencies:** stdlib only; requires `github.com/skosovsky/toolsy` (core).

## Available tools

| Tool        | Description                         | Input                                                   |
| ----------- | ----------------------------------- | ------------------------------------------------------- |
| `http_get`  | Perform an HTTP GET request         | `{"url": "string"}`                                     |
| `http_post` | Perform an HTTP POST with JSON body | `{"url": "string", "json_body": {"key": "value", ...}}` |

Result: `{"status": 200, "body": "..."}`. Responses retain their body unchanged or return a limit error; there is no silent truncation. The toolkit does not interpret response text as trusted instructions.

## Contract and bounds

- `WithMaxResponseBody`: source body read budget, default 512 KiB.
- `WithMaxRequestBody`: POST JSON input byte budget, default 512 KiB; reject before dispatch and preserve approved bytes.
- `WithMaxWireBytes`: complete encoded result budget, default 4 MiB, checked after JSON escaping. Body and wire limits are independent; escaping can make a body within its read budget exceed the wire budget.
- Nonpositive values select these finite defaults. URLs are limited to 8192 bytes.
- Exceeding a limit returns a validation error with no successful result. A POST may already have occurred when its response exceeds a result limit; response bounds are not an operation rollback guarantee. Compose host operation profiles for durable approval/idempotency.
- The safe tool client has a 30-second default timeout; a positive custom `http.Client.Timeout` overrides it and context cancellation still applies. The host selects allowed destinations and private-IP exceptions.
- No generic pagination is promised. A host can expose API-specific query/cursor parameters in the URL; stable continuation depends on that API. Status/body are returned without synthesizing a token.

## Credentials and destinations

Run credentials are used only when the host supplies `WithCredentialOrigins([]string{"https://api.example.com"})`. Bindings compare the exact scheme, hostname and effective port (443/80 defaults). Allowed domains permit network access; they do not authorize credentials. An unbound allowed destination receives no credential and does not call the credentials provider.

Cross-origin redirects always remove Authorization, including redirects to another allowed domain or port and HTTPS-to-HTTP redirects. A redirect never acquires credentials for its destination. Same-origin redirects can retain credentials. URL userinfo and static Authorization/Proxy-Authorization/Cookie headers are rejected. `WithHeaders` is for non-secret request metadata; arbitrary application-specific secret headers are the host's responsibility and must not be supplied there. Use the origin-bound credentials provider or a host proxy for authentication. Tool names alone do not establish a destination grant.

## Library mode (without tools)

Use exported primitives in host infrastructure:

```go
import (
	"context"
	"errors"
	"net/http"

	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func newSafeClient(allowed []string) *http.Client {
	return httptool.NewSafeHTTPClient(httptool.SafeDialOptions{
		AllowedHosts: allowed,
	}, httptool.CheckRedirectAllowed(allowed, false))
}

// Read response bodies (fail-closed):
data, err := httptool.ReadBodyLimited(ctx, resp.Body, 512*1024)
if errors.Is(err, textprocessor.ErrReadLimitExceeded) {
    // handle limit
}
body := string(data)
```

**SafeDialOptions host policy:**
- `AllowedHosts` non-empty → strict whitelist (only listed hosts; fail-closed on Allowed+Blocked overlap).
- `AllowedHosts` empty → blacklist via `BlockedHosts` plus always `IsBlockedIP` at dial time.

See `IsBlockedIP` in godoc for details.

## Tool mode

> **Warning:** You must call `WithAllowedDomains(...)` with a non-empty list. Without it, all requests are rejected with a client error.

- **Allowed domains:** Use exact hostnames (e.g. `api.example.com`) or prefix with `.` for subdomains: `.slack.com` allows `api.slack.com`, `hooks.slack.com`, but not `slack.com` or `evil-slack.com`.

- **SSRF protection:** `AsTools` uses `SafeDialTransport` with DNS-rebinding pin and `CheckRedirect` validation by default. URL checks use the same `LookupIPAddr` + `IsBlockedIP` path as dial time. Use `WithAllowPrivateIPs(true)` only in tests (e.g. httptest on 127.0.0.1).

- **Custom client:** `WithHTTPClient` merges only `Timeout` onto the safe client; Transport and CheckRedirect from a custom client are ignored.

- **Headers:** `WithHeaders` sets non-secret metadata; authentication uses the explicit origin binding above.

- **Response headers:** Results contain only status and body. API-specific continuation headers require a separate host adapter.

## Quick start

```go
package main

import (
	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func main() {
	builder := toolsy.NewRegistryBuilder()

	tools, err := httptool.AsTools(
		httptool.WithAllowedDomains([]string{"api.example.com", ".slack.com"}),
		httptool.WithMaxResponseBody(256 * 1024),
	)
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		builder.Add(tool)
	}
}
```

## Testing with httptest

For tests that use `httptest.NewServer` (e.g. on 127.0.0.1), allow the loopback host and enable private IPs only in tests:

```go
tools, err := httptool.AsTools(
	httptool.WithAllowedDomains([]string{"127.0.0.1"}),
	httptool.WithAllowPrivateIPs(true), // testing only
)
```
