# Policy and budget gates

Registry authorization uses `Policy.Decide(context.Context, PolicyRequest)` and
`Decision`. Install it with `WithPolicy(stableID, policy)`. The host owns the ID;
change it when authority semantics change, so binding/checkpoint compatibility
reflects those changes. A policy runs before validators, handler execution and
execution-profile replay. Typed argument policy remains after argument binding.
Manifest requirement guards still require an enforcing requirements policy.

For an existing host authorization service, `NewAuthorizerPolicy(authorizer)`
captures an `Authorizer` with `Authorize(context.Context, PolicyRequest) error`.
Check its construction error, then install the returned policy with `WithPolicy`.
The [executable host-port example](../policy_gates_example_test.go) demonstrates
construction and an authorized call.
It does not look up authorization through RunEnv. Every callback error becomes a
nonretryable POLICY_DENIED with no argument repair fields; errors.Is/As retains
the original cause. Structured argument-repair decisions belong to Policy or
typed argument policy. This adapter does not authenticate caller identity.

Explicit nil, typed-nil or nil-function policies fail registry construction with
ErrPolicyConfiguration. That error remains sticky even if a later option supplies
a valid policy. View and restore reject typed-nil policies too. Omitting policy
is intentional absence of an optional gate, subject to manifest requirements.
A view's identity comes from the bound registry view, not caller ViewID metadata.

Composed policies receive independent snapshots of framework-owned input,
manifest, call-context and view containers. Referenced arbitrary BYOT identities
are host-owned; they must be immutable or synchronized. Captured callback ports
must remain valid and support concurrent calls for the registry lifetime. Toolsy
does not close those ports.

## Budget admission

`WithBudget()` requires a valid BudgetTracker at DepKeyBudget for every invocation.
Install it with `Put(env, DepKeyBudget, tracker)`, check its error, and supply that
RunEnv to the call.
Missing, wrong-type, nil and typed-nil dependencies return an INTERNAL ToolError
with ErrBudgetConfiguration in the cause chain before wrapped execution.
`WithOptionalBudget()` permits only an absent dependency; supplied invalid values
still fail. Optional mode is a host decision, not automatic fail-open recovery.

The dependency is captured under the store lock. Its Allow callback runs outside
that lock, permitting host dependency updates. A replacement applies to subsequent
lookups; it does not change the captured callback already running. Host owns the
tracker's concurrency, pricing, state and resource lifetime.

Allow receives a cloned manifest and raw input before typed binding. Admission
runs for each wrapped call, including one that later replays a cached result.
It is not a physical-dispatch accounting hook. Charge actual effects using host
execution observations; idempotence alone does not justify charging or replay.

A callback failure returns INTERNAL retaining its cause. Denial yields one
BUDGET_EXCEEDED error chunk and skips the wrapped tool; inspect chunks as well as
the returned error. A yield failure retains the consumer cause. Context cancellation
is checked before lookup and after Allow, and wins over an Allow result. The
callback must cooperate with context; these checks cannot interrupt arbitrary
host code. Put WithBudget inside AsAsyncTool if admission should occur in its
background execution; wrapping AsAsyncTool checks only initial acceptance. See
the async middleware contract in async.go.
