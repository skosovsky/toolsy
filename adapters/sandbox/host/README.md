# Host Sandbox Adapter

**DANGER: NO ISOLATION. USE ONLY WITH HUMAN-IN-THE-LOOP.**

`host` runs configured host binaries inside a temporary workspace. It is useful for
CLI helpers and local development flows where the operator explicitly accepts that
code executes on the current machine.

On Unix, timeout cleanup kills the whole spawned process group. On non-Unix
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
Request code and validated files are materialized exactly as supplied. No shell
command is constructed. Stdout and stderr each have a 256 KiB cap. Nonzero guest
exit is a result, while start/collection failures are errors. Cancellation wins
over output overflow. Unix cancellation kills the launched process group;
processes that deliberately leave it are outside this guarantee. Non-Unix process
termination is best effort. Process I/O waiting has a five-second `WaitDelay`.

Workspace cleanup uses a fresh five-second context independent of the caller and
checks its deadline between filesystem operations. Individual filesystem syscalls
cannot be interrupted; this is a cooperative cleanup budget, not a hard bound on
a stalled filesystem. It never follows symlinks during normal traversal. As a
trusted host backend it does not defend against malicious concurrent filesystem
mutation. Cleanup failures return an inspectable `*exectool.CleanupError` and
preserve any primary error and completed result. A failed cleanup is never reported
as confirmed removal. No background cleanup goroutine is left behind.
