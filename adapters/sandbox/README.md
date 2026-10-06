# Sandbox backends

All backends implement the [execution and cleanup contract](../../docs/sandbox-result-contract.md).
A backend receives a single `exectool.RunRequest`; the host chooses its trust boundary,
provides deadlines and owns approval, retries and reconciliation. Selecting a backend
never grants authority to execute an external effect.

| Backend | Enforced boundary | Limits and host responsibilities | Verification |
| --- | --- | --- | --- |
| [Docker](docker/README.md) | Local Linux daemon; mandatory cgroup limits, unprivileged user, network disabled, read-only root and request workspace, dropped capabilities, seccomp | Finite memory, CPU/PID, tmpfs and per-stream output limits. Host provides daemon-visible workspace and disk capacity; live output collection is not a hard log-disk quota | Unit/client mocks; optional `TestDockerLiveProfile` checks three default images, restrictions and output overflow only when explicitly enabled against a provisioned daemon |
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


## Evidence scope

The verification column lists available fixtures, not certification of every
backend in the current run. Ordinary `go test` skips Docker live checks unless
`TOOLSY_DOCKER_LIVE=1` is set; a PASS with that skip proves only unit/client
contracts. Live results must identify the daemon/platform/images and actual
executed checks. E2B has no live service verification in this repository; its
mock port tests do not establish remote isolation, teardown or SDK compliance.
Local host processes and real in-process interpreters exercise their documented
local behavior, without extending it to hostile process containment or a hard
Go process memory quota. See each backend README for the exact capabilities.

Filesystem pathname containment also does not imply storage isolation: mounts,
hardlink aliases and host directory mutation require host ownership policy.
Request materialization and cleanup assume an exclusively owned workspace and
parent. For general filesystem tools, read ranges and directory offsets are not
snapshots and writes are non-atomic in-place updates; see the
[filesystem host consistency requirements](../../toolkits/fstool/README.md#filesystem-boundary-and-host-consistency).
