# Toolsy: HTTP Toolkit (safe GET/POST for agents)

**Description:** Lets the agent perform HTTP GET and POST requests to external APIs with SSRF protection (allowed domains whitelist, optional private IP checks) and configurable response body truncation.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/httptool
```

**Dependencies:** stdlib only; requires `github.com/skosovsky/toolsy` (core).

## Available tools

| Tool       | Description                          | Input                                                    |
|------------|--------------------------------------|----------------------------------------------------------|
| `http_get` | Perform an HTTP GET request          | `{"url": "string"}`                                      |
| `http_post`| Perform an HTTP POST with JSON body  | `{"url": "string", "json_body": {"key": "value", ...}}`  |

Result: `{"status": 200, "body": "..."}`. Body is truncated to `maxResponseBody` (default 512KB) with `[Truncated]` suffix if longer.

## Configuration & Security

> **Warning:** You must call `WithAllowedDomains(...)` with a non-empty list. Without it, all requests are rejected with a client error.

- **Allowed domains:** Use exact hostnames (e.g. `api.example.com`) or prefix with `.` for subdomains: `.slack.com` allows `api.slack.com`, `hooks.slack.com`, but not `slack.com` or `evil-slack.com`.

- **SSRF protection:** The toolkit validates scheme (http/https only), host against the whitelist, and optionally resolves the host and blocks private IP ranges (127.0.0.0/8, 10/8, 172.16/12, 192.168/16, 169.254/16, ::1, fe80::/10). This is a defense-in-depth layer.

> **Warning (DNS Rebinding):** The built-in private IP check is a basic defense-in-depth layer. For strict SSRF protection against DNS rebinding attacks, provide a custom `http.Client` with a secured `DialContext` via `WithHTTPClient`.

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
	reg := toolsy.NewRegistry()

	tools, err := httptool.AsTools(
		httptool.WithAllowedDomains([]string{"api.example.com", ".slack.com"}),
		httptool.WithMaxResponseBody(256 * 1024),
	)
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		reg.Register(tool)
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
