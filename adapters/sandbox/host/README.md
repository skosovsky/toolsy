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
