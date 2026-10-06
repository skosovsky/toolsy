# Remote bridge contract

The `agents` and `mcp` modules adapt remote observations to toolsy. The host owns
credentials, authenticated identity, authorization, persistence, continuation
and reconciliation. Neither bridge creates an agent loop or grants authority
from model text or remote metadata.

## Agent delegation

The [agents profile](../agents/README.md) separates the pinned Agent Protocol REST
schema from the toolsy step-stream extension. Only a completed final step is a
successful business result. Remote failure, remote cancellation, malformed data,
EOF without a terminal and uncertain observations have typed outcomes. Progress
or useful partial output does not establish completion.

An error after task creation preserves the task reference where known. Local
cancellation and deadline causes remain inspectable; an uncertain remote action
must not receive permission for blind retry. Core normalization preserves an
adapter's explicit nonretryable timeout and retains observation-error causes.
Stream reconnection resumes observations only. It does not repeat task creation.
Bytes across all connections share one budget; event size, reconnect count,
backoff and timeout follow the selected stream policy.

Background delegation returns an accepted task reference. The host saves that
reference and decides how to retrieve status or continue work. This API does not
provide durable tracking or completion of the delegated business operation.

## MCP trust and authentication

The [MCP client](../mcp/README.md) keeps its pinned protocol and negotiated
capability boundary. Source annotations remain untrusted descriptors. The
optional typed host mapper supplies execution properties; these properties are
not an authorization grant. Remote idempotency hints alone cannot activate
ResultCache. Current host policy and current discovery generation protect both
dispatch and cached delivery. Host identity types remain unrestricted.
Generation checks reject stale proxies; the host's cache partition binds
connection identity and dependency/discovery freshness for newly rediscovered
proxies. The bridge does not automatically invalidate stored cache entries.

HTTP 401/403 exposes a bounded, narrow challenge projection for explicit host
inspection. Error text and ordinary formatting/logging omit challenge values;
no raw response body, credential header or full header map is copied. URLs in
challenge DTOs remain untrusted hints, as described in the
[auth contract](../mcp/docs/http-auth-challenges.md). Receiving one does not cause
discovery requests, token refresh or effectful-request retries.

The Client advertises only implemented capabilities. ExtensionRegistry stores
explicit codecs; it does not enable an extension runtime. Unsupported features
remain typed failures. OAuth lifecycle, elicitation UI, MCP Tasks runtime, A2A
and persistent remote scheduling require a separate consumer-backed activation.

## Verification boundary

Pinned schema fixtures, actual local HTTP servers and adversarial cache/policy
integration tests establish the supported contract. They do not establish live
interoperability with an arbitrary remote agent or MCP deployment. Live checks
must be recorded separately against an actual service and configuration.
