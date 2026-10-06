# HTTP authentication failures

The MCP 2026-07-28 Streamable HTTP transport returns `*HTTPError` for both
401 and 403 before reading the response body. `errors.As` exposes the status,
a bounded `Challenges` slice and `ChallengesTruncated` to a host handler.
A challenge never triggers a metadata request, OAuth discovery, refresh,
credential replacement or retry of the POST. The transport terminates on the
failure; the host owns any subsequent connection and action decision.

The supported representation is intentionally narrower than the complete
[HTTP authentication grammar](https://www.rfc-editor.org/rfc/rfc9110.html#section-11.6.1):

- At most 4096 bytes across authentication header values (including one byte
  of accounting per field), four challenges and
  1024 bytes per retained parameter. Oversized headers are dropped whole, and
  exhausting the header/challenge budget sets `ChallengesTruncated`.
- Challenge schemes are ASCII HTTP tokens, at most 32 bytes. Token68 data is
  discarded. Quoted commas and multiple challenges/header values are handled;
  malformed quotes and control characters reject that header value.
- Only Bearer `error` codes `invalid_request`, `invalid_token` and
  `insufficient_scope` are retained. Other error strings, `error_description`,
  realm, scope, credentials and unknown parameters are discarded.
- Bearer `resource_metadata` is retained only as a bounded HTTPS URL without
  userinfo, query or fragment. The URL remains untrusted server input. It is
  not proof of issuer identity or permission to contact that origin. The host
  validates identity and network policy if it explicitly uses this hint.

`Error()`, all `fmt` formatting verbs and `slog.LogValue` on HTTPError and
HTTPAuthChallenge omit challenge values. They cannot expose an authentication
response body, unrelated response headers or outbound Authorization credentials.
A host explicitly reading fields can still disclose those fields; this API is
not a generic secret detector, and arbitrary server URLs can contain sensitive
path components. Do not marshal the DTO or log its fields as telemetry.

`http_auth_test.go` runs actual local HTTP 401/403 requests, adversarial secret
markers, safe formatted/structured logging, bounds and zero discovery/retry
assertions. This is fixture conformance, not live OAuth interoperability.
