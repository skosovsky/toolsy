# Public execution entry-point contracts

This map is the current reference for execution boundaries and their public
entry-point families. Helper options configure these same contracts; they do not
create a separate execution or authority path. Tool manifests expose input/output
schemas and audience/control/effect hints. A hint never grants business authority.
Host-provided implementations must satisfy their declared ports, own credentials,
lifetime and synchronization, and cooperate with context/owned IO interruption.

| Entry-point family | Input and authoritative output | Authority, failure, cancellation and lifecycle reference |
| --- | --- | --- |
| NewTool / NewTypedTool | BYO args/results/effects; reflected or explicit executable schemas; typed original value is distinct from final wire bytes | [Result](result-contract.md), [prepared execution](execution-contract.md); pre-dispatch input repair differs from post-handler failure; schema compiled at construction |
| NewProxyTool / NewDynamicToolFromSpec | Explicit local schema; raw exact JSON or lossless decoded json.Number values | [Prepared/input contract](execution-contract.md), [result representation](result-contract.md); bounded parser, no external schema resolution, no hidden repair retry |
| NewStreamTool / generated stream factories | Explicit independent-results or terminal-result contract; progress is not business result | [Structured stream](execution-contract.md#structured-stream-result), [generator](generator-contract.md#streaming-and-host-async-composition); consumer/producer failure prevents terminal success; cooperative producer cancellation |
| AsAsyncTool | Host background timeout/collection cap/callback; accepted scheduling differs from terminal completion | [README async](../README.md), [generator async](generator-contract.md#streaming-and-host-async-composition); host owns completion/error observation and registry Shutdown; parent cancellation detached by async contract |
| RegistryBuilder.Build / Registry.Execute / views / Session.RunCall | Manifest catalog, input JSON plus trusted typed call context, ToolOutcome/chunks | [Prepared execution](execution-contract.md), [policy gates](policy-gates.md), [result/control](control-contract.md); current policy before dispatch/replay, no scheduler or implicit retry |
| NewPolicyTool / WithPolicy / requirements / budget gates | Final bound argument snapshot and explicit host Policy/Decision/ports | [Gate contract](policy-gates.md); required nil dependency rejects; callback effects are host owned; current authority on replay |
| NewResultCache / NewOperationProfile / ReconcileOperation | Prepared intent, explicit eligibility/partition, codec, approval/claim/fence and trusted reconciliation | [Execution/cache/journal](execution-contract.md); persistence/replay source not new effects; unknown outcome never grants redispatch; original recovery grant invariant retained |
| NewSession / Rebind / ExportSnapshot / ImportSnapshot / codecs | Atomic immutable invocation configuration; state+binding snapshot, not workflow continuation; BYOT values and borrowed ports | [Session/DI reference](../README.md#session-state-and-runenv-di), [prepared snapshot boundary](execution-contract.md); host values need synchronization; codecs frozen/snapshot callbacks outside state lock; external StateStore not included |
| Put / Require / Lookup / state mutations | Typed dependency or session state boundary; required writes return errors, optional reads explicit | [Session/DI](../README.md#session-state-and-runenv-di), [task41 migration](migration-task41.md); no nil-env/session silent required mutation and no global value deep-copy promise |
| Control constructors / YieldControl / historycodec | Neutral typed Pause/Yield/Halt/host event; explicit audience/delivery/replay transcript | [Control](control-contract.md), [historycodec](../historycodec/README.md); host interprets event, payload has no executable authority; transcript is not a typed cache codec |
| MCP SDK import / export | Owned SDK snapshots; bounded adapter admission; scoped prepared execution | [MCP contract](../mcp/README.md); SDK protocol ownership, documented dependency limitations, generation revocation and borrowed session disposal |
| Remote agents bridge | Supported task/terminal reference and custom SSE/cancel extension | [Remote bridge](remote-bridge-contract.md), [agents](../agents/README.md); accepted task not completion; diagnostics not automatic scheduler/cancel confirmation |
| OpenAPI / GraphQL / gRPC adapters | Bounded supported source schema/discovery → tool manifests; host outputs/credentials/connection | [Schema adapters](../contracts/README.md), module READMEs; unsupported shapes fail closed, causal transport errors, owned HTTP pools vs borrowed gRPC connection |
| Toolkit AsTools / cleanup factories / configured ports | Each toolkit's concrete input/result DTO and final wire/source/item bounds | [Eleven toolkit contracts](../toolkits/README.md) and linked module READMEs; host ports/provider effects owned by host, zero/negative/nil constructor semantics explicit, cleanup owns idle pools only |
| exectool.New / Sandbox.Run / backend constructors | Language/code/env/files → exact RunResult; effect/cleanup failure carries RunOutcomeError | [Exec](../exectool/README.md), [sandbox result](sandbox-result-contract.md), [deadlines](sandbox-deadlines.md); selected capability policy, no generic isolation or hard foreign callback preemption |
| toolsy-gen / release CLI | Bounded manifests → generated files; tracked isolated checkout → exact release artifacts/ref train | [Generator](generator-contract.md), [current installation](README.md#installation-and-module-alignment), [release proof](task41-documentation-checklist.md#7-release-artifacts-and-checkout); generation recovery diagnostics vs completed commit cleanup; publish is explicit host action |

For concrete module paths/install alignment see the [current index](README.md).
A public custom Tool/port can advertise a different schema, but the host must
validate its construction, output and prepared capability before protected use.
This table does not turn arbitrary custom implementations into trusted built-ins.
Schema/source limits and cooperative callback ownership remain part of each
linked module's contract, rather than an unenforceable universal guarantee.

## Input admission before whole-batch dispatch

`AdmitInput(ctx, tool, trustedCallContext, input)` returns a defensive `ToolInput`
with canonical JSON from the same input stage as dispatch. It supports
`NewTypedTool` (default extractor or custom `ArgsBinder`, then `ArgValidator`)
and `NewDynamicToolFromSpec` (decode, schema, object check, then `ValidateArgs`).
Canonical JSON encodes the validated value, including sanitizer changes; it
preserves typed integers and dynamic `json.Number` precision.

Known native wrappers forward internally: logging, recovery, budget, truncation,
error formatting, async, and manifest overrides. Their execution behavior is not
run by admission. An override still uses its underlying constructor's binder and
schema, just as dispatch does. Chains are bounded to 64 inspected layers; nil,
cycles, excessive chains and unsupported tools return
`ErrInputAdmissionUnsupported`. `NewTool`, `NewStreamTool`, raw/proxy constructors
and `NewPolicyTool` currently have no separate admission stage and return that
error. Custom opaque tools and wrappers must implement `InputAdmitter` explicitly.
`ChainUnwrapper` alone is insufficient: a wrapper that changes input in `Execute`
must apply the same transformation in its admission port or reject admission.

The host can check every slot before starting any handler. It retains the original
inputs for dispatch; the returned canonical snapshot is informational, and feeding
it back to a non-idempotent binder could apply its transformation twice:

```go
// slots contain the resolved tool, its ToolCall, and trusted host context.
for _, slot := range slots {
    if _, err := toolsy.AdmitInput(ctx, slot.Tool, slot.Call.CallContext, slot.Call.Input); err != nil {
        return err
    }
}
for _, slot := range slots {
    if err := registry.Execute(ctx, slot.Call, yield); err != nil {
        return err
    }
}
```

See the executable package-external `ExampleAdmitInput` for an invalid second slot
that prevents the first handler, and a valid batch that runs only in phase two.
This API adds no batch executor and promises no batch transaction or atomicity.
Admission never invokes handlers, operation `Claim`, execution profiles, controls,
budget tracking, typed policy, scope authorization or registry hooks. Success is
not a grant, `PreparedCall`, cached binding, or reusable dispatch continuation.
Normal dispatch repeats binding, validation and current authorization using its
actual arguments and trusted context, even when the `CallID` is unchanged.

Binders, schema/custom validators and custom admission ports may perform I/O or
have effects: the SDK cannot prove callback purity. Hosts must keep these callbacks
repeatable and free of business effects, and retain synchronization responsibility
for host-owned mutable state. Input and context metadata snapshots protect ordinary
JSON/typed mutable values, not arbitrary resources or closures. Canceled contexts
are rejected before callbacks and after successful callbacks; callback validation
errors retain the dispatch input-stage error mapping. Callback panics propagate to
the caller, including through `WithRecovery`; Registry panic recovery belongs to
execution and is not invoked here.

Consumer migration should remove duplicate binder attachment and manual wrapper
walking only for these supported constructor/native wrapper paths after updating
to the verified release recorded in the input-admission task. Custom wrappers need
the explicit port first; raw passthrough is not a validation success. Core adds no
Guardy/Prompty imports and does not own Kosmify's batch orchestration.
