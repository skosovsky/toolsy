# Starlark Sandbox Adapter

`starlark` runs code in the host process using `go.starlark.net/starlark`.
Supported language is `starlark`, with the runtime's default syntax options
(no recursion or `while` statements). Only `env`, an immutable dictionary of
request environment variables, and `fs.read(path)`, an immutable in-memory
request file reader, are added to the standard language built-ins. Module
loading and access to the host filesystem/network are not exposed.

## Execution contract

Construct with `New(DefaultConfig())`, or provide `Config{MaxExecutionSteps: n}`
with a positive finite limit. Zero is rejected, including a zero-value Sandbox
at Run. The default budget is 1,000,000 abstract interpreter steps per request.
The meaning of a step belongs to the runtime and may change between versions.
Budget exhaustion returns an error matching both `ErrStepLimit` and
`exectool.ErrSandboxFailure`; it is not a guest exit. Syntax/evaluation failures
return a normal `RunResult` with exit code 1 and bounded stderr.

| Capability | Guarantee |
| --- | --- |
| Computation | Finite interpreter step budget; enforced on interpreted instructions |
| Cancellation | Caller cancellation interrupts interpreted instructions; deadline maps to `exectool.ErrTimeout` |
| Output | stdout/stderr capped at `sandboxfs.DefaultMaxSandboxOutputBytes`; overflow is an execution error and stdout overflow cancels further interpretation |
| Files | Canonicalized request paths only; reads capped at `sandboxfs.DefaultMaxSandboxFileReadBytes` |
| Environment | Explicit request map only; no parent process environment inheritance |
| Cleanup | Cancellation watcher is stopped and joined before Run returns |
| Hard memory/CPU isolation | **Not provided**; execution, parsing and built-ins share host resources |

Step counting and cancellation do not preempt parsing or a running Go built-in.
They are not a hard wall-clock bound and cannot prevent a large allocation
inside a single operation. For hostile code requiring enforceable memory or
process resource isolation, the host must select a backend providing those
capabilities. This adapter does not advertise a memory cap.
