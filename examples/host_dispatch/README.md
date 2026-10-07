# Host dispatch recipe

Run `go run ./examples/host_dispatch` from the repository root. The local trusted
operator receives a bound approval challenge, approves that exact action, executes
an exact integer write and redelivers it with a new provider correlation. Only the
fresh execution prints a reducer effect. No provider credentials are required.

`recipe` is host application code: BYOT Request carries provider CallID separately
from OperationID, AttemptID and ActivityIdentity, plus trusted subject/scope and
current policy/dependency fingerprints. The caller supplies a restricted view;
that same view creates the Session and the advertised manifests. `Profile` must
be installed in the registry before constructing the view. Tools requiring
approval must not be invoked without that prepared execution profile. This demo
is not a remote authentication implementation: the local operator is fixed.

The host owns the sequential barrier, model delivery and reducer; Toolsy owns
binding, authorization and atomic operation claim. An outer continuation runtime
may own persistence and recovery; disable any competing dispatch/retry loop.
See [the contract](../../docs/host-dispatch-contract.md),
[migration](../../docs/host-dispatch-migration.md) and the
[optional integration module](../host_dispatch_integration).

`OneShot` returns the complete Result for a single call. It neither resumes nor
retries. A simple JSON-only convenience helper in a caller remains appropriate
only when identity/control/effects/approval/recovery are outside its contract.
Use the full host dispatcher for that lifecycle.

Only sequential batches are supported. Internal/user payloads and diagnostics
never enter ModelResult; progress remains host-only. Binary delivery requires a
separate renderer. A reducer must support a durable outbox/atomic state boundary
before a host can promise exactly-once reducer effects across crashes. The memory
store used here provides no process durability; reopen tests use the bounded local
file journal. Neither store provides distributed exactly-once external effects.
