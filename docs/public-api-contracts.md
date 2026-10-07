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
| MCP Connect / ListToolsPage / Discover / proxies / transport facets | Strict discovery snapshots and typed supported protocol; bounded queues/items/bytes | [MCP contract](../mcp/README.md); atomic full-discovery authority, request-owned cancellation/retirement, borrowed custom transport required context/completion semantics, explicit disposal |
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
