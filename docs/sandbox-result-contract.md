# Sandbox result and cleanup contract

`Sandbox` is a one-shot execution seam. Isolation, network, filesystem and resource guarantees belong to the selected backend and its configuration; this interface does not imply isolation. Host owns retry/reconciliation policy.

| Outcome | Result and error |
| --- | --- |
| Guest completes, including nonzero exit | Complete stdout/stderr, exit code and duration; nil error |
| Setup/transport/output collection fails | Error matching `ErrSandboxFailure`; incomplete output is not a successful result |
| Output exceeds a cap | Error matching `ErrSandboxFailure` and `textprocessor.ErrReadLimitExceeded` |
| Caller deadline expires | `ErrTimeout`; takes precedence over simultaneous output overflow |
| Caller cancellation | Caller context error; takes precedence over simultaneous output overflow |
| Cleanup fails after guest completion | Original guest result plus `CleanupError` |
| Cleanup fails after primary error | Primary error joined with `CleanupError`; both remain inspectable |

Known guest exits are normalized by each backend before calling `sandboxfs.FinalizeOrInterrupt`: pass nil execution error and the exit code. All nonnil execution errors mean incomplete execution or collection. `FinishRun` no longer accepts an `exitOK` flag that could hide a transport failure.

Cleanup diagnostics expose backend, operation and cause through `errors.As(*exectool.CleanupError)`, and match both `ErrSandboxCleanup` and `ErrSandboxFailure`. The cleanup cause is available on the typed diagnostic, but deliberately does not participate in errors.Is: a cleanup deadline must not become an execution timeout. Joined primary errors remain inspectable normally. Cleanup diagnostics never replace a primary timeout/cancellation, so consumers must classify interruption first. A cleanup error confirms failure or missing confirmation, not resource removal. Hosts must not blindly retry a side effect merely because cleanup failed. Backend resource handles stay backend-owned, rather than becoming a new core lifecycle framework.

Backends create a fresh bounded cleanup context independent of caller cancellation for context-aware close/remove operations. `sandboxfs.RemoveWorkspace` checks cancellation between paged directory reads and deletions and refuses nesting beyond 128 directories without following child symlinks. The owner must stop guest access before removal and retain exclusive control of the workspace path and its parent. Root symlinks/non-directories are rejected before opening; this does not claim protection against a concurrent external actor replacing the owned path. An OS filesystem syscall cannot offer a hard wall-clock guarantee: backends document this limitation, avoid abandoning a goroutine per operation, and report any observed cleanup failure. No helper claims an unenforceable universal hard timeout.

`textprocessor.ReaderWithContext` checks context before each read. It cannot interrupt an already blocked generic `io.Reader`, owns no reader lifetime and launches no goroutines. Transport owners close owned bodies/processes or set I/O deadlines to unblock reads. Cancellation between reads is cooperative; mid-read termination requires a transport capability.

This is a breaking change to internal helpers and cleanup visibility. No compatibility flag or permissive legacy path is retained.

`CleanupError.ResourceID` optionally retains a backend-owned opaque reconciliation
locator (the host backend uses its retained workspace path). `exectool` wraps sandbox
errors with `RunOutcomeError`, retaining exactly the returned RunResult for host
inspection through errors.As. It emits no success chunk on that path. Cleanup-only
failures can carry a completed guest exit/output; interrupted/setup failures may
carry zero or incomplete results. The typed error does not claim completeness or
authorize retry. Primary cancellation/timeout/output-limit classification survives.

Completed stdout/stderr are exact collected bytes; the shared finalizer has no
presentation trim flag. Starlark print's newline is retained on success and guest
failure. Consumers may explicitly trim for display. Starlark fs.read failures,
including its file cap, are guest evaluation errors; missing complete collected
stdout/stderr instead returns an infrastructure/output error.
