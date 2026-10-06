# Sandbox backends

All backends implement the [execution and cleanup contract](../../docs/sandbox-result-contract.md).
A backend receives a single `exectool.RunRequest`; the host chooses its trust boundary,
provides deadlines and owns approval, retries and reconciliation. Selecting a backend
never grants authority to execute an external effect.

| Backend | Enforced boundary | Limits and host responsibilities | Verification |
| --- | --- | --- | --- |
| [Docker](docker/README.md) | Local Linux daemon; mandatory cgroup limits, unprivileged user, network disabled, read-only root and request workspace, dropped capabilities, seccomp | Finite memory, CPU/PID, tmpfs and per-stream output limits. Host provides daemon-visible workspace and disk capacity; live output collection is not a hard log-disk quota | Unit fixtures and live Docker runs for all three default images, isolation restrictions and output overflow |
| [Wazero](wazero/README.md) | WASI request filesystem confined through `os.Root`, read-only mount; cancellation enabled even with custom runtime config | Explicit linear-memory page cap. Caller supplies deadline. Compiler, tables and other host allocations have no process memory quota | Real WASI containment, memory growth and infinite-loop cancellation tests |
| [Starlark](starlark/README.md) | Request-only environment and in-memory files; no exposed host filesystem, network or module loader | Mandatory positive instruction-step budget and bounded output. Parsing and Go built-ins cannot be preempted; no hard memory isolation | Actual interpreter workloads for step budget and cancellation |
| [Host](host/README.md) | Trusted host execution | Empty environment by default; inheritance requires explicit option. Host rights, filesystem and network remain available; process-group cancellation has documented platform limits | Local process and cancellation tests |
| [E2B](e2b/README.md) | Bring your own client and remote service | Adapter validates runtime command, caps observations and reports cleanup failures. Client owns transport interruption and service isolation guarantees | Mock client conformance; no claim of live remote-service verification |

Generic readers only check context between reads. Their owner must close a blocked
transport or set deadlines. Cleanup uses independent bounded contexts where the
backend supports them; filesystem system calls remain cooperative. A cleanup
failure is observable and never proves that a resource was removed.

These profiles are distinct capabilities, not interchangeable isolation guarantees.
No agent scheduler, persistent sandbox manager or remote credential lifecycle is
provided by this directory.
