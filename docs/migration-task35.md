# Prepared execution and explicit streams

## Result cache

`WithIdempotency`, `IdempotencyStore` and `MemoryIdempotencyStore` have been removed. They cached result bytes outside typed binding/policy and could disclose data across scope or broaden an internal result's audience.

Create `NewResultCache(store, eligibility, partition, codec, maxBytes)` and install it with `NewRegistryBuilder(WithExecutionProfile(cache))`. For direct built-in Tool calls use `NewRunEnv(nil, WithRunExecutionProfile(cache))`. Keep policy in `NewTypedTool` or `NewPolicyTool`; both run binding and current typed authorization before a replay.

The partition callback receives a `PreparedCall` with canonical input and current context/view. Supply an authenticated domain partition and freshness of dependencies affecting the result. Empty partitions fail closed. Canonical arguments, attachments and manifest/view identity are also included in the key. Do not use model text or transport call ID as authentication or operation identity.

Store values are complete encoded outcomes. Use `JSONResultCodec[YourResult, YourEffect]` for declared JSON result/effect types and JSON metadata, or implement `ResultCodec` for other host-owned types. Unsupported values and corrupt records fail explicitly. Existing byte-only cache entries are incompatible: use a separate storage namespace and expire old entries.

Dynamic JSON numbers in result/effect/interface fields and metadata now replay as `json.Number`, not float64; use its explicit conversion methods and handle conversion errors. Concrete declared numeric fields retain their declared type. Use a custom codec if consumers require reconstruction of arbitrary concrete Go types inside interfaces. Do not convert large dynamic numbers to float64 just to match an old assertion.

Replay preserves MIME, audience, delivery class, metadata, typed result, effects and control declarations. `ReplaySourceMetadata` is set on the replay envelope. Host reducers must treat those declarations as replayed, rather than applying effects a second time. Correlation belongs to the current call. Persistence/delivery/codec failures after dispatch never authorize automatic retries.

Remove `ReplaySourceMetadata` from policy `EnvelopeMetadata` configuration: the key is library-owned and constructors reject any configured value, including false or nil. Keep host labels under separate keys. Nested metadata transforms cannot erase replay provenance or broaden a stored private audience.

The cache does not atomically reserve external operations. `WithIdempotent` remains a declaration about the tool, not proof that its dependencies implement downstream idempotency. Operation journaling and unknown-outcome recovery are separate contracts.

## Tool implementations and wrappers

Built-in builders now call `ExecutePrepared` after binding and typed policy. Custom protected tool implementations must do the same and implement `PreparedExecutionTool`. Unsupported tool chains are rejected by a registry with a profile before execution. Instrumentation wrappers must preserve the prepared capability. The handler continuation can dispatch at most once per attempt and receives an independent canonical input copy.

`NewPolicyTool` runs its binder once and delegates to the underlying tool's final prepared boundary. Its validator and policy authorize the final handler argument snapshot, not the intermediate binder output, even without a profile. Inner binders and policies run on replay too; the profile observes the actual handler arguments. If the final argument type differs, the wrapper validates/decodes final canonical JSON into its declared type. Keep any required opaque proof in the final typed value. Current delivery class and envelope metadata apply to stored replay; differing stored/current audiences intersect at internal-only delivery, never broaden stored privacy. A base tool without a prepared capability is rejected at construction. Session RunPolicy/budget and registry authorizer/policy continue to run on every call. Use the actual Session/view executor for adapters.

## Streams

Every `NewStreamTool` now requires an explicit option:

- `WithIndependentStream()` for progress or independent results. There is no implicit terminal promise, schema aggregation or MIME inference.
- `WithTerminalStream(maxBytes)` plus `WithOutputSchema(schema)` for exactly one final result. Zero limit selects a bounded default; negative limits fail construction. Terminal schema compiles at construction and forbids external schema loading.

Progress is delivered while the tool runs. The terminal candidate is schema-validated and buffered until the producer completes successfully. Invalid, duplicate or missing terminals, output-limit failures and aborts are distinct `StreamContractError` kinds. Failure after yielding a candidate never delivers final success. Pause/control outcomes remain control, not successful completion. Stream contract errors are not retryable action errors.

Existing callers have been migrated to explicit independent semantics. To consume a validated export without an agent, see `examples/stream_terminal`. A valid JSON input prefix still does not authorize dispatch: provider adapters must wait for their explicit end marker and validate the full call before invoking the executor.

## Bound approvals and operations

Select `NewOperationProfile` instead of result caching for effectful logical operations. The profile uses the same post-binding/post-authorization boundary. Supply an authenticated host preparation callback, trusted issuer, clock, lease, complete result codec and atomic `OperationStore`.

The preparation callback supplies stable namespace/scope/subject and logical operation ID. Repeated delivery keeps that ID; a new intentional action gets a new ID even for identical arguments. Generate a fresh attempt ID for each authorized execution attempt. The canonical digest must bind all prepared arguments, attachments and secret references affecting the action; do not hash a redacted UI string. Build a bounded redacted JSON action description from that same snapshot.

The library additionally binds prepared JSON and attachment MIME/bytes to the opaque host digest. Never recycle any earlier attempt ID within the operation: the journal rejects it after recovery too. Resolve is a trusted host action; its proof identifier/time are persisted as provenance. `OperationStoreError` identifies infrastructure failure without exposing diagnostic paths/secret values in its public error text.

`WithRequiresConfirmation` is a declaration, not a grant. Without an approved grant the profile returns a pending control outcome and `PendingApprovalError` containing the bound challenge. The host authenticates the approver, then writes `ApprovalGrant` through the issuer port and resumes the same logical operation. Do not accept model-generated grant IDs or decisions as authenticated approval. Repeated calls still execute current registry/session/typed policy before disclosure or dispatch.

For direct typed tools, bind authenticated host values with `WithRunCallContext` in addition to `WithRunExecutionProfile`. Built-in calls own their preparation state while sharing host dependencies/session. Do not pass a protected handler's inherited environment into another direct Tool.Execute: it fails closed before child binding/handler because its manifest/grant belongs to the parent action. Use a fresh host environment for an independent direct call, or the supplied scoped executor for nested calls. Direct execution is not a replacement for Session RunPolicy/budget. The human toolkit's free-text review/clarification pauses are conversation outcomes only and cannot substitute for a bound grant.

Use the host-selected `adapters/execution/filejournal` for local crash recovery; `MemoryOperationStore` is process-local only. Protect the journal directory and manage confidentiality/retention. Grant reservation and claim must share the selected store transaction. Consumed grants cannot be replaced to blindly repeat an uncertain action. Recovery with the original grant requires explicit AllowRecovery, current expiry/authorization and trusted resolution.

`examples/approval_journal` is a runnable ordinary CLI host with its own subject/scope types, a restricted Session and one receipt-writing reference tool. Its integration test covers pending without effect, explicit local approval, reopening/replay without another effect, changed args conflict and a separately identified new intent. The trusted CLI approval flag is not remote authentication. For a nested call, pass the supplied Session/view executor and the explicitly delegated host context; do not recover a root registry or dispatch raw handlers. The nested Session acceptance fixture verifies RunPolicy/view selection, shared budget, pending/resume/replay and revoked ACL.

Lease expiry, cancellation and a lost response do not prove the effect failed. `OperationOutcomeError` means the dispatched action has an unknown outcome; do not retry automatically. The host reconciles through `Resolve` using a verified external result or explicitly declared downstream idempotency with the original key. Proof identifiers are provenance, not authority. A completed outcome survives delivery failure; replay is marked with `ReplaySourceMetadata: ReplaySourceOperation`, and reducers must not reapply effects.

Obtain binding/attempt from `OperationOutcomeError` or `OperationStateError`; use host-only `Inspect` to read it without dispatch or mutation. `ReconcileOperation` invokes a trusted host callback and fences its decision to that inspected operation/attempt. Callback failure is `ReconciliationError` and does not authorize retry. Resolve never runs the handler or delivers result to a model: resume/replay through the current authorized executor. When a retry relies on downstream idempotency, explicitly verify the downstream contract and forward the original key to the handler through trusted host context/dependencies; the library does not create a remote exactly-once guarantee.

## Remaining implementation

Approval/operation runtime, local durable adapter, terminal stream validation and their integration/fault-injection suites are present. All original MCP migration scenarios are reconciled. The final full workspace test/race, lint and clear-break preflight gates pass. The custom-pending consumer-abort cleanup finding is independently closed: per-invocation cancellation releases custom Await without closing the transport or duplicating cancellation notifications. Both independent final reviews pass:66/67 implemented plus one explicitly accepted historical process exception;67/67 accepted. Optional programmatic/workspace/MCP/catalog profiles retain their activation conditions and all54 deferred requirements in the matrix; none are claimed delivered. No release or issue closure has been performed.

Task41 supersedes the original cache eligibility/marker contract: supply an explicit per-attempt CacheEligibility predicate; WithIdempotent alone is insufficient. ReplaySourceMetadata holds ReplaySourceCache or ReplaySourceOperation, not a boolean. See [task41 migration](migration-task41.md).
