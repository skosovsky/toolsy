# agents

This optional remote task adapter converts creation and observation through the custom toolsy step-stream profile into a `toolsy.Tool`. Core does not depend on Agent Protocol. The host supplies credentials, decides whether creation is authorized and owns task persistence and continuation.

## Supported contract

The base envelopes follow [Agent Protocol v1 OpenAPI 3.0.1 at commit ecbffe0b9e45bdd3ead76299af6d980a1132b32a](https://github.com/agi-inc/agent-protocol/blob/ecbffe0b9e45bdd3ead76299af6d980a1132b32a/schemas/openapi.yml), retained with its license in [testdata](testdata/README.md). Normative `Step.status` values are `created`, `running` and `completed`. Normative Task has no status. BYO object arguments are sent as `additional_input`; the optional string `input` is omitted. Creation accepts a direct Task envelope with a nonempty `task_id` and an `artifacts` array. The old wrapped `{ "task": ... }` response is unsupported.

The wire DTO is a supported projection, not a full normative Artifact validator: it reads `artifact_id` and `file_name`; normative `agent_created` and `relative_path` are not exposed. Inline `mime_type` and base64 `data` are extension observations rendered for the caller. Nonempty step/task identifiers and required envelope field presence are validated; this bridge does not certify complete upstream schema conformance.

This bridge requires the **toolsy step-stream v1 extension** on the remote server: `GET /ap/v1/agent/tasks/{id}/steps?stream=true` returns SSE step envelopes; `POST /ap/v1/agent/tasks/{id}/cancel` requests cancellation. Terminal extension statuses `failed` and `cancelled` add explicit failure reporting. These endpoints and states are not normative Agent Protocol v1 or A2A requirements. The bridge does not execute the normative effectful POST-step loop or implement another agent protocol.

| Step status | `is_last` | Bridge behavior |
| --- | --- | --- |
| `created`, `running` | false | Progress observation |
| `completed` | false | Progress; task completion remains unconfirmed |
| `completed` | true | Confirmed success; one successful result |
| `failed` | true | Typed remote failure; partial output in error field |
| `cancelled` | true | Typed remote cancellation; partial output in error field |
| Other status or incompatible last flag | any | Typed malformed outcome; no successful result |
| EOF without a terminal frame, after allowed reconnects | — | Typed incomplete outcome; no successful result |

`RemoteOutcomeError` exposes `Phase` (`create` or `observe`), `Outcome`, `TaskID`, optional `Step` and `Cause`. A create response/transport failure can leave acceptance unknown, including an empty reference when none could be recovered. An observation transport/limit failure leaves remote completion unknown while preserving the known reference. Error text omits remote output, task identifiers and transport text; inspect typed fields deliberately. Required step fields, duplicate JSON keys and mismatched task identifiers fail closed.

Local cancellation and timeout retain `errors.Is` semantics and do not claim that the remote action stopped. Once creation may have occurred, timeout errors explicitly set `Retryable=false`; the host reconciles the reference before any new action. Consumer callback errors are preserved and immediately stop observation. A cancelled parent triggers one best-effort cancel request using a fresh five-second context; successful HTTP acknowledgement is not confirmation of remote cancellation. No stream reconnect repeats `CreateTask` or executes a remote action.

## Bounded observations

`WithMaxResponseBody` caps REST responses (default 4 MiB). `WithMaxSSEStreamBytes` caps physical bytes read for **one logical stream across every connection** (default `httptool.DefaultMaxSSEStreamBytes`). Duplicates, comments and replayed bytes count. The reader never resets its remaining budget on reconnect. AsTool keeps the typed outcome/reference when mapping read-limit errors to tool validation errors.

`WithStreamPolicy(agents.StreamPolicy{...})` replaces all observation limits. `DefaultStreamPolicy()` allows two reconnects with a one-second backoff, a five-minute total deadline and a 1 MiB event limit. Zero reconnects disables resume; zero backoff is allowed. Timeout and event size must be positive; event size cannot exceed the supported 16 MiB ceiling. Invalid policy fails before creation. The earlier host deadline takes precedence, including while idle or backing off; the HTTP request context owns body interruption.

The extension accepts UTF-8 SSE with LF or CRLF separators. A blank line dispatches a frame; an unterminated frame at EOF is discarded and cannot confirm success. Physical event bytes, including CRLF and all small lines, share the event cap. Fields remove only the optional single space after `:`; leading/trailing data whitespace is retained. `event:` and `retry:` do not alter the host policy. Empty frames clear their field buffers. An explicit empty `id` (including bare `id`) clears the resume cursor; NUL-bearing IDs are ignored.

On reconnect `Last-Event-ID` carries the last complete frame's cursor. Within a logical stream, exact JSON data replay with the same nonempty ID is suppressed; conflicting data for that ID is malformed. Empty or absent IDs have no deduplication guarantee. ID/data history is bounded by the same cumulative byte budget and released at the end of the iterator. This is observation deduplication, not effect idempotency or durable cross-invocation tracking. Iterator consumer stop closes the response and performs no further request.

## Usage and background acknowledgement

```go
client, err := agents.NewClient("https://agent.example.com",
    agents.WithStreamPolicy(agents.DefaultStreamPolicy()),
)
if err != nil {
    return err
}
defer client.CloseIdleConnections()
tool, err := agents.AsTool("delegate", "Delegate remote work", inputSchema, client)
if err != nil {
    return err
}
```

`AsBackgroundTool` returns the declared JSON result `{"task_id":"...","accepted":true}` as `AcceptedTaskReference`. This confirms start acknowledgement only. It does not fabricate a completed business outcome or guarantee durable tracking. The host stores the reference and chooses its own status retrieval and continuation; see the executable [background example](background_example_test.go).

Credentials are resolved separately through `RunEnv.Credentials` for `agents.create_task`, `agents.stream_steps` and `agents.cancel_task`. `WithHTTPSettings(httptool.ClientSettings{Timeout: ..., TLSConfig: ...})` applies explicit settings to the SSRF-safe transport. `NewClient` returns a construction error for invalid settings; custom Do/transport/proxy ports are unsupported. `WithAllowPrivateIPs` is an explicit host choice for private deployments.

Redirects are allowed only for GET/HEAD reads within the original scheme, hostname and effective port. Create/cancel POST requests never redirect, including same-origin redirects and redirects that rewrite POST to GET. Hosts configure the final endpoint explicitly. A refused redirect exposes `*httptool.RedirectError` through the error chain; the original request may have produced effects. Create failures retain their unknown-outcome classification. Redirect refusal never authorizes argument repair or blind redispatch.

## Verification boundary

Tests use the pinned normative envelopes, a distinct extension terminal table, local HTTP/SSE fixtures, aggregate byte/event limits, resume duplicates, idle deadlines and consumer aborts. No live remote interoperability is claimed. There is no A2A runtime, persistent scheduler, automatic retry permission or hidden status manager here.

Each Client owns one pool for REST and SSE calls. On configuration disposal, stop
new calls and invoke `client.CloseIdleConnections()`; active calls are unaffected.
Idle connections are bounded and expire after 90 seconds even without explicit
cleanup. A zero HTTP timeout leaves requests bounded by their context (logical
SSE reads additionally use StreamPolicy). A positive HTTP timeout also limits SSE
responses, so choose it for the intended stream duration. TLS roots, certificates
and callback state referenced by the cloned TLSConfig must remain immutable.

## Parent interruption and optional cancellation diagnostics

`WithCancellationObserver(agents.CancellationObserver)` adds host-only observations
for `AsTool` cleanup after successful creation of a known task reference. Nil or an
absent observer disables diagnostics. Direct `CancelTask` calls already return their
error to the host and do not invoke this observer. `AsBackgroundTool` acknowledges
creation and does not acquire observation/cancellation ownership.

| Read interruption | Remote cancellation behavior | Primary execution outcome |
|---|---|---|
| Parent cancel/deadline after creation | One best-effort custom cancel request with a fresh five-second context; credential failure prevents the request | Parent interrupt/unknown remote outcome preserved; timeout remains nonretryable |
| StreamPolicy deadline with active parent | No implicit remote cancel | Nonretryable observation timeout; task reference retained |
| Consumer callback stops with active parent | No implicit remote cancel | Original callback cause preserved through the error chain; no completion claim |

If the parent is also interrupted when callback handling returns, parent-triggered
cleanup still applies; diagnostics do not replace the callback cause. Core may wrap
a consumer error in `ErrStreamAborted`; use `errors.Is`/`errors.As`, not identity
comparison with the returned execution error. Cleanup uses
the parent context values with cancellation removed, followed by a five-second
child deadline. Credential resolution, HTTP request and observer share that budget.
Credentials/observers are opaque host callbacks and must cooperate with the context;
there is no forced goroutine, panic recovery or hard preemption of a blocking port.

`CancellationDiagnostic` reports `TaskID`, `ParentInterrupt` (`ctx.Err()`),
`ParentCause` (`context.Cause(ctx)`, including a custom host cause), `Stage`
(`CancellationCredentials` or `CancellationRequest`), `Acknowledged`, and the exact
cleanup `Cause`. Credential failures report the credentials stage and no request;
request failures preserve their cause and `Acknowledged=false`. A successful HTTP
response sets `Acknowledged=true`, which is **not** evidence that remote execution
stopped or an authorization to repeat creation. The host must reconcile uncertainty.

The observer runs synchronously once at the end of the attempt, even after failure,
using the same cleanup context (which may already have expired). Keep it brief,
context-aware, safe for concurrent client calls and free of panics. Capture is by
function reference; its state/lifetime remains host-owned. This option does not
replace the primary execution error or emit a tool chunk. No implicit logging occurs.
Task IDs and error causes may contain sensitive/untrusted observations; apply host
redaction and access controls before persistence or display. Headers/credentials
are not separate diagnostic fields. A disabled observer intentionally leaves cleanup
failure unreported, with existing best-effort behavior preserved.

```go
func newRemoteClient(observe agents.CancellationObserver) (*agents.Client, error) {
    return agents.NewClient("https://agent.example.com",
        agents.WithCancellationObserver(observe),
    )
}
```

This remains an adapter for the pinned base envelopes plus the custom SSE/cancel
extension; it neither runs the normative POST-step loop nor schedules remote task
retries, durable continuation or cleanup jobs. Local HTTP/SSE tests do not prove
remote cancellation acknowledgement means stopped execution.
