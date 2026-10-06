# Control requests and host events

Core delivers typed, sealed control data; the host owns continuation and routing.
`YieldControl` validates and snapshots its signal, emits one `EventControl` chunk,
and returns the corresponding sentinel after successful delivery. Return that
error from the producer. A consumer error takes precedence; invalid controls
produce an INTERNAL `ResultContractError` with kind `control_contract` before
control delivery. This is post-handler output failure: effects may already exist,
and the error never authorizes argument repair, rollback or blind retry.

| Signal / sentinel | Host request | Core scope |
|---|---|---|
| `PauseSignal` / `ErrPause` | Pause host continuation; Reason explains why | No durable continuation, automatic resume, grant issuance or blocking of other calls. |
| `YieldSignal` / `ErrYield` | End host continuation quietly; Result is descriptive text | Not a terminal business result or proof of success. |
| `HaltSignal` / `ErrHalt` | Stop host continuation; Reason explains why | No cancellation of context, session, registry or other executions. |
| `HostEventSignal` / `ErrHostEvent` | Route Name plus optional JSON data | No built-in event semantics, UI action, executor, authorization or scheduler. |

`IsControlError` recognizes wrapped control sentinels, except diagnostic causes
inside known result/outcome reconciliation errors. Middleware preserves actual
control errors. `EventControl` delivery alone cannot preempt a producer that
ignores callback/control errors. Terminal-stream and cache/operation profiles
retain their existing sticky-stop contracts; ordinary callbacks remain cooperative.
Controls declared in `ToolResult.Controls` are attached to a terminal result and
collected in outcomes; they do not automatically return control errors or execute
a host action. `CompletionPolicy` is separate metadata asking the host how to route
a successful completion. Core does not enforce those routing hints.

## Delivery and persistence bounds

- `MaxControlBytes = 65536`: inclusive sum of text/name/payload field bytes for
  one signal, and aggregate sum for a terminal result's control list.
- `MaxControlSignals = 64`: inclusive terminal control count.
- `MaxHostEventNameBytes = 128`: inclusive ASCII name length. First byte must be
  alphanumeric; later bytes may also contain `.`, `_`, `-`, `/` or `:`.
- Text and payload must be valid UTF-8. Empty reason/result text is allowed.
- Host payload may be absent; a present payload must be strict JSON (duplicate
  keys, trailing documents and depth above 128 rejected by the shared decoder).
  These checks establish data shape, not a host command schema or permission.
- Singular `Chunk.Control` requires `EventControl`. That event cannot carry wire
  Data/MimeType/error data or a result Controls list. Result Controls are allowed
  only on `EventResult`; typed-nil, nil and unrecognized signals fail explicitly.

Preparation copies built-in signal structs and host payload bytes for delivery.
Consumers own their delivered copy and must not concurrently mutate it during
capture/encoding; no general concurrency safety for mutable Go aliases is claimed.
These finite limits validate delivery/persistence; they cannot prevent a handler
from first allocating a larger value, or enforce a whole-invocation control count
when an ordinary producer ignores control errors.

`JSONResultCodec` preserves neutral events as kind `host_event`, enforcing the
same control bounds on encode/decode. Legacy kind `ui` is rejected. Host migration
must deliberately map old records after review; no silent reinterpretation occurs.
Raw `historycodec` still rejects controls by its existing transcript contract.

## Host composition

See runnable [host event example](../examples/host_event/main.go): a named request
is routed by an explicit host allowlist, with host payload interpretation. An
actual shell performs authorization and invokes its UI API in that adapter. Core
provides no generic `any` callback/execution port and no mandatory shell dependency.
