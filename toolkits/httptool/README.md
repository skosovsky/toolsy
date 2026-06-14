# Toolsy: HTTP Toolkit (safe GET/POST for agents)

**Description:** Lets the agent perform HTTP GET and POST requests to external APIs with SSRF protection (allowed domains whitelist, optional private IP checks) and configurable response body truncation.

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

Result: `{"status": 200, "body": "..."}`. Body is truncated to `maxResponseBody` (default 512KB) with `[Truncated]` suffix if longer. This is a **probe-tool tier**: suffix lives in the `body` field before JSON marshal, not the toolkit wire-cap path (`format.CapWireJSON`).

## Library mode (without tools)

Use exported primitives in host infrastructure:

```go
import (
	"net/http"

	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func newSafeClient(allowed []string) *http.Client {
	return httptool.NewSafeHTTPClient(httptool.SafeDialOptions{
		AllowedHosts: allowed,
	}, httptool.CheckRedirectAllowed(allowed, false))
}

// Read response bodies with UTF-8 safe truncation:
body, err := httptool.ReadBodyLimited(ctx, resp.Body, 512*1024)
```

**SafeDialOptions host policy:**
- `AllowedHosts` non-empty → strict whitelist (only listed hosts; fail-closed on Allowed+Blocked overlap).
- `AllowedHosts` empty → blacklist via `BlockedHosts` plus always `IsBlockedIP` at dial time.

See `IsBlockedIP` (preferred for SSRF dial/resolve) and `IsPrivateIP` (legacy alias) in godoc for details.

## Tool mode

> **Warning:** You must call `WithAllowedDomains(...)` with a non-empty list. Without it, all requests are rejected with a client error.

- **Allowed domains:** Use exact hostnames (e.g. `api.example.com`) or prefix with `.` for subdomains: `.slack.com` allows `api.slack.com`, `hooks.slack.com`, but not `slack.com` or `evil-slack.com`.

- **SSRF protection:** `AsTools` uses `SafeDialTransport` with DNS-rebinding pin and `CheckRedirect` validation by default. URL checks use the same `LookupIPAddr` + `IsBlockedIP` path as dial time. Use `WithAllowPrivateIPs(true)` only in tests (e.g. httptest on 127.0.0.1).

- **Custom client:** `WithHTTPClient` merges only `Timeout` onto the safe client; Transport and CheckRedirect from a custom client are ignored.

- **Response size:** Use `WithMaxResponseBody(n)` to cap response body size (default 512KB). Truncation is UTF-8 safe.

- **Headers:** Use `WithHeaders(map[string]string{...})` to add headers to every request. Prefer not to pass secrets to the agent via headers; use a proxy or server-side auth instead.

- **Response headers:** V1 returns only `Status` and `Body`. A future version may add `Headers map[string]string` (filtered, e.g. without `Set-Cookie`, `Server`) for pagination (e.g. `Link`) or rate limits (`X-RateLimit-Remaining`).

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
