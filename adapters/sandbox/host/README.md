# Host Sandbox Adapter

**DANGER: NO ISOLATION. USE ONLY WITH HUMAN-IN-THE-LOOP.**

`host` runs configured host binaries inside a temporary workspace. It is useful for
CLI helpers and local development flows where the operator explicitly accepts that
code executes on the current machine.

On Unix, an owned /bin/sh supervisor holds the process-group identity until
guest completion. All completion paths kill remaining group members before
reaping that leader; no destructive signal is sent after reaping it. On non-Unix
platforms, cleanup is best-effort because there is no portable process-tree
termination primitive in the standard library.

## Installation

```bash
go get github.com/skosovsky/toolsy/adapters/sandbox/host
```

## Example

```go
sb, err := host.New(
    host.WithRuntime("python", host.Runtime{
        Command:    "python3",
        ScriptName: "main.py",
    }),
)
if err != nil {
    panic(err)
}
```

## Execution contract

This backend executes **trusted code without isolation**. It does not restrict
filesystem access, network access, memory, CPU, process creation, or access to
other host resources. Workspace path validation prevents request file collisions;
it does not confine guest code. Human approval is an application policy, not a
security boundary enforced by this adapter.

The default guest environment starts empty (the Go process launcher can add
`PWD`). `WithEnvironment(map[string]string)` supplies an explicit base;
`RunRequest.Env` overrides it. `WithInheritedEnvironment()` explicitly snapshots
the parent environment at construction, including any secrets. Runtime executable
lookup uses the host process PATH; select an absolute executable path when its
identity matters. Inheritance is a clear break from the previous default.

The host config fixes the executable, initial arguments, and entrypoint filename.
Request code and validated files are materialized exactly as supplied. Unix launches the runtime directly through Go exec. A separate /bin/sh anchor
holds the owned group identity without executing guest code. It requires /bin/sh. Stdout and stderr each have a 256 KiB cap. Nonzero guest
exit is a result, while start/collection failures are errors. Cancellation wins
over output overflow. Unix cleanup kills the launched process group on normal
exit, nonzero exit, cancellation and collection failure;
processes that deliberately leave it are outside this guarantee. Non-Unix process
termination is best effort. Unix owned output pipes have an explicit five-second collection budget; non-Unix
process I/O uses five-second WaitDelay.

Workspace cleanup uses a fresh five-second context independent of the caller and
checks its deadline between filesystem operations. Individual filesystem syscalls
cannot be interrupted; this is a cooperative cleanup budget, not a hard bound on
a stalled filesystem. It never follows symlinks during normal traversal. As a
trusted host backend it does not defend against malicious concurrent filesystem
mutation. Cleanup failures return an inspectable `*exectool.CleanupError` and
preserve any primary error and completed result. A failed cleanup is never reported
as confirmed removal. No background cleanup goroutine is left behind.

The runtime retains Go exec start errors and exit-code semantics. Guest descendants
do not inherit the private anchor hold descriptor. After SIGKILL and supervisor collection, the
adapter observes group disappearance with a fresh five-second budget; zombies or
PID/group reuse may cause conservative cleanup failure, never a destructive signal
to a reaped/reused group. Cleanup failure retains the workspace for reconciliation
and supplies its path as CleanupError.ResourceID. Escaped groups remain outside
this non-isolated backend guarantee. A trusted guest can interfere with supervision;
this is resource ownership, not a malicious-process containment mechanism.

If a group signal fails, direct owned guest/anchor termination is attempted and the
failure remains a cleanup diagnostic. Kernel process termination and filesystem
syscalls cannot provide a hard deadline for an uninterruptible OS state. This adapter
does not claim containment of malicious credential changes or signal interference.

All concurrent stop requests share their first outcome. After the direct guest is
reaped, further group sweeps run only while the anchor remains unreaped, covering a
fork concurrent with the initial signal. During output collection these sweeps are
bounded by the collection budget. A transient EPERM after successful signaling is
not removal confirmation; post-anchor observation must still reach ESRCH. No
process-group signal is sent after anchor reap.
