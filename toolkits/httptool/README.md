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

Result: `{"status": 200, "body": "..."}`. Tool response bodies must contain valid UTF-8 bytes; valid bodies roundtrip unchanged through the JSON string, and invalid UTF-8 returns an error before conversion. There is no silent truncation or replacement. Content-Type/charset does not trigger guessing or transcoding. The toolkit does not interpret response text as trusted instructions.

## Contract and bounds

- `WithMaxResponseBody`: source body read budget, default 512 KiB.
- `WithMaxRequestBody`: POST JSON input byte budget, default 512 KiB; reject before dispatch and preserve approved bytes.
- `WithMaxWireBytes`: encoded status/body JSON budget (excluding outer host envelopes), default 4 MiB, checked after JSON escaping. Body and wire limits are independent; escaping can make a body within its read budget exceed the wire budget.
- Nonpositive values select these finite defaults. URLs are limited to 8192 bytes.
- Pre-dispatch request bounds and GET response bounds retain validation errors. POST response-read/wire bounds instead return CodeInternal ResultContractError with the original limit cause, preventing argument repair or blind retry. No successful result is emitted. A POST may already have occurred when its response exceeds a result limit; response bounds are not an operation rollback guarantee. Compose host operation profiles for durable approval/idempotency.
- The safe tool client has a 30-second default timeout; a positive `ClientSettings.Timeout` overrides it and context cancellation still applies. The host selects allowed destinations and private-IP exceptions.
- No generic pagination is promised. A host can expose API-specific query/cursor parameters in the URL; stable continuation depends on that API. Status/body are returned without synthesizing a token.

## Credentials and destinations

Run credentials are used only when the host supplies `WithCredentialOrigins([]string{"https://api.example.com"})`. Bindings compare the exact scheme, hostname and effective port (443/80 defaults). Allowed domains permit network access; they do not authorize credentials. An unbound allowed destination receives no credential and does not call the credentials provider.

GET/HEAD redirects may follow the domain whitelist. Cross-origin redirects remove Authorization, Cookie and Proxy-Authorization, including redirects to another allowed domain or port and HTTPS-to-HTTP redirects. A redirect never acquires credentials for its destination. Same-origin read redirects retain credentials. POST and other methods never redirect, including same-origin 307/308 body replay and 301/302/303 rewriting to GET. URL userinfo and static Authorization/Proxy-Authorization/Cookie headers are rejected. `WithHeaders` is for non-secret request metadata; arbitrary application-specific secret headers are the host's responsibility and must not be supplied there. Use the origin-bound credentials provider or a host proxy for authentication. Tool names alone do not establish a destination grant.

`CheckRedirectRemote` is stricter: only same-origin GET/HEAD redirects are allowed. It is used by agents/OpenAPI/GraphQL; GraphQL POST queries and introspection do not redirect. Refused redirects return `*httptool.RedirectError` through the error chain. The original request was already sent and may have produced effects; this error does not authorize argument repair or retry. Hosts configure the final endpoint explicitly. `NewSafeHTTPClient` with a host-supplied redirect callback uses that callback's contract; a nil callback disables redirects.

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
// Keep data as []byte for binary adapters, or explicitly choose a text/binary encoding.
```

**SafeDialOptions host policy:**
- `AllowedHosts` non-empty → strict whitelist (only listed hosts; fail-closed on Allowed+Blocked overlap).
- Matching deny entries always win over allows, including broad allow plus a narrower deny. Host rejection occurs before DNS or dialing. A configured nonempty allowlist containing only blank entries denies all hosts.
- `AllowedHosts` empty → blacklist via `BlockedHosts` plus always `IsBlockedIP` at dial time.

Both lists use the same syntax: `example.com` matches only that exact hostname; `.example.com` matches descendants only and excludes the apex. Use both entries for apex plus descendants. Matching ignores case, outer whitespace and a terminal DNS root dot. Domains with different ports share host policy; credential origins remain bound to scheme/hostname/effective port.
Request hosts with empty labels, a leading dot or repeated terminal dots are rejected before DNS/dial. This normalization does not infer aliases or expand a bare entry into descendants.

See `IsBlockedIP` in godoc for details.

## Tool mode

> **Warning:** You must call `WithAllowedDomains(...)` with a non-empty list. Without it, all requests are rejected with a client error.

- **Allowed domains:** Use exact hostnames (e.g. `api.example.com`) or prefix with `.` for subdomains: `.slack.com` allows `api.slack.com`, `hooks.slack.com`, but not `slack.com` or `evil-slack.com`.

- **SSRF protection:** `AsTools` uses `SafeDialTransport` with DNS-rebinding pin and `CheckRedirect` validation by default. URL checks use the same `LookupIPAddr` + `IsBlockedIP` path as dial time. Use `WithAllowPrivateIPs(true)` only in tests (e.g. httptest on 127.0.0.1).

- **HTTP settings:** `WithHTTPSettings(httptool.ClientSettings{Timeout: ..., TLSConfig: ...})` applies timeout and TLS settings to an owned safe transport. Custom Do/transport/proxy/redirect ports are unsupported.

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

A tool set owns one reusable pool, with at most 32 idle connections overall, two
per host, and 90-second idle expiry. `AsToolsWithCleanup` additionally returns an
idle closer for host disposal. Stop starting calls before cleanup; active calls
remain unaffected. `NewConfiguredSafeHTTPClient` applies `ClientSettings` and
returns construction errors (including negative timeout); its caller owns the
returned client. TLSConfig is cloned, while referenced certificates, roots and
callback state must stay immutable. Defaults do not use environment proxies.

Safe dialing checks all DNS answers, then attempts those pinned IPs sequentially
within one total dial deadline, including lookup. Each attempt receives a share
of the remaining deadline; caller cancellation stops further attempts. DNS is
never resolved again between attempts. Stream readers use stop-after-budget
semantics: exactly exhausting a byte cap without an EOF in the last Read yields
a limit error on the next nonempty Read, without probing beyond the budget. Empty
reads consume nothing; cancellation takes precedence.

Invalid tool response encoding is CodeInternal with ResultContractError kind
http_response_encoding and an inspectable ResponseEncodingError (Method/Status),
matching ErrInvalidUTF8Response. It is non-retryable and not an argument error.
Empty bodies, NUL, CRLF and valid Unicode remain unchanged; there is no charset
sniffing, replacement character insertion or implicit Base64 wrapper. HTTP status
is preserved for valid UTF-8 regardless of Content-Type or success/error status.
A failed POST result does not mean its remote side effect was undone. Hosts own
reconciliation/idempotency; cancellation also never promises rollback. The exported
ReadBodyLimited/stream readers remain byte-oriented for binary host adapters.
