# Host dispatch contract

This recipe is an ordinary host application. Toolsy owns tool preparation and
execution, not model iteration, scheduling, authentication or recovery policy.
The executable reference is `examples/host_dispatch/recipe`; concrete consumer
adapters live in the separate `examples/host_dispatch_integration` module.

## Inputs and ownership

| Input | Owner | Rule |
|---|---|---|
| CallID | Provider | Nonempty original correlation; unique in an admitted batch |
| OperationID | Host | Stable for redelivery of one intent; distinct for new intents, even with identical arguments |
| AttemptID | Host | Nonempty fresh fencing attempt, independent of CallID |
| ActivityIdentity | Host | BYOT comparable type, nonzero; stable within a continuation |
| Subject / Scope | Host | BYOT authenticated values, never extracted from model text |
| GrantID | Trusted issuer | Bound challenge only; never sourced from provider content |
| Tool / raw args / attachments | Provider + host | Bounded by execution policies; exact JSON transferred without floating point conversion |
| Policy / dependency fingerprint | Host | Current trusted identity, recomputed on resume |

The host snapshots request bytes before invocation. The supplied view creates the
Session and supplies the exact source manifests. A capability is visibility, not
an authorization grant. Source schemas remain admission contracts; provider
lowering must not replace them. Raw preflight must not execute the binder or reject
aliases accepted by the tool's canonical binder. Final schema and policy checks
belong to the prepared execution path. A binder is not an effectful activity.

## Admission and transitions

`Dispatch` accepts sequential batches only. It checks all identities, membership
and valid raw JSON before the first handler. Duplicate call, operation, attempt
or activity identities within a batch are rejected. Redelivery uses a separate
batch. A second concurrent call on the same dispatcher is rejected; independent
hosts rely on the journal's atomic claim, not that local barrier. The host must
not concurrently schedule a second effect outside this dispatcher.

| Priority | Observed state | Host decision | Next dispatch / model result |
|---|---|---|---|
| 1 | Unknown outcome or in-progress journal record | uncertain | Neither; inspect/reconcile using trusted evidence |
| 2 | PendingApproval | approval | Neither; persist challenge and authenticate approver |
| 3 | Halt / Pause / Yield / HostEvent | halt / pause / yield / host_event | Neither; route to trusted host |
| 4 | Infrastructure failure | fault | Neither; diagnostics remain host-only |
| 5 | Policy/capability deny | deny | Stop; fixed error projection only for model audience |
| 6 | Correctable schema/args error | correction | Stop; explicit future correction, no automatic retry |
| 7 | Business error | business_error | Stop; fixed error projection only for model audience |
| 8 | Completion halt / silent_yield | halt / yield | Stop; no automatic model turn |
| 9 | Successful continue | continue | Next sequential call permitted; audience/MIME checked first |

Unrecognized control, status, completion, replay provenance, audience or MIME
fails closed. Empty/noop success is represented explicitly. Host outcome and
infrastructure error remain separate, preserving partial results and progress.
The example's model projection deliberately omits progress, effects, controls,
typed objects and diagnostics. JSON results are not double encoded; text is JSON
quoted; binary requires a separate host renderer and is unsupported here.
Internal/user-only payloads have no model projection, including business errors.

Fresh built-in producers reject the reserved replay metadata key before result
persistence/delivery. Custom tools/profiles must implement that ownership contract;
arbitrary tool metadata is not authentication. Only host-trusted prepared tools
and execution profiles belong in this full-lifecycle profile.

Effects are delivered to a trusted host reducer for fresh outcomes only; trusted
`result_cache` and `completed_operation` provenance suppress repeated effects.
This demo does not promise exactly-once reducer application across crashes. A
production host needs an atomic reducer/outbox boundary for that guarantee.
Reducer failure stops scheduling and does not authorize a repeat tool dispatch.

## Approval and recovery

The operation profile receives the prepared canonical arguments and authenticated
context. Its binding covers subject/scope, tool, manifest/view/policy fingerprints,
args and attachment bytes plus host dependency identity. Description derives from
the same snapshot. Editing any bound value invalidates the grant. Current policy
is reevaluated before dispatch and before completed replay. Two authorized resume
attempts may cause at most one handler invocation.

After delivery loss: `Inspect` confirms completed state without executing or
revealing a model payload; replay is obtained through the current Session with a
new provider CallID and the same OperationID. Unknown/in-progress never causes
blind dispatch. Trusted reconciliation requires external evidence; neither a
lease expiry nor a model assertion proves absence of an effect. Local journal
reopen tests demonstrate local durability, not distributed exactly-once effects.

## Supported profiles

| Profile | Supported | Limits |
|---|---|---|
| OneShot single call | Yes | Full outcome returned; caller owns any continuation; no automatic recovery |
| Sequential dispatch with prepared typed tools | Yes | One scheduler; explicit controls and completion |
| Concurrent batch on one dispatcher | No | Admission refusal before dispatch |
| Binary model delivery / unknown protocol fields | No | Explicit host delivery failure |
| Raw convenience ToolInvoker for full lifecycle | No | Does not carry native identity/full outcomes; simple JSON tools only |
| Model-issued approval / implicit retry | No | Host-only authority |

## Evidence index

AC1–AC7: recipe contract/operation tests, including durable reopen and fault
injection. AC8: runnable provider-neutral main and real consumer semantic fixtures.
AC9: module-wide race/lint and both dependency modes. Exact test names and results
are recorded in `docs/reviews/task36/verification.md` after execution.
