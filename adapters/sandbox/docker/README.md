# Docker sandbox

This backend runs one request in an ephemeral **local Linux Docker** container.
It requires a Unix daemon connection and a host workspace path visible at the
same path to that daemon. `WithWorkspaceRoot` selects a shared directory (for
example `/private/tmp` on Docker Desktop). Remote daemons are outside this
backend's filesystem contract. Injected clients must preserve that contract.
Images must already exist; image acquisition and image trust belong to the host.

## Mandatory execution profile

`DefaultPolicy()` selects 256 MiB memory without swap, one CPU (100000 microseconds
per 100ms), 64 PIDs, 32 MiB `/tmp`, 32 MiB shared memory, 64 MiB input bytes (including filenames), at most 256 input files, and
8 MiB output bytes **per stream**. `WithPolicy` overrides positive bounds;
unrestricted zero bounds are rejected. At each run the daemon must report
Linux, seccomp, memory, swap, CPU-period/quota and PID enforcement support or the run refuses.

| Capability | Enforced mechanism |
| --- | --- |
| Network | `NetworkMode=none`; guest has loopback only |
| Memory/CPU/PIDs | Mandatory Linux cgroup bounds, no swap |
| Privileges | UID/GID 65534, all capabilities dropped, no-new-privileges; Docker default seccomp |
| Root/workspace | Readonly root filesystem and readonly local bind of prepared input |
| Writable scratch | `/tmp` tmpfs with size/noexec/nosuid/nodev, separately bounded `/dev/shm` |
| Output | Live stdout/stderr follow with independent caps; malformed/truncated framing rejects; overflow cancels/kills execution |
| Time | Caller cancellation/deadline plus a host-selected log stream deadline (default five seconds), including execution |
| Cleanup | Fresh five-second contexts for kill/removal; removal failures are typed secondary cleanup errors |

Python, Bash and Node default images execute with this profile. Their entrypoint
is explicitly replaced by the configured runtime command. Workspace directories
are traversable and files readable by UID 65534; guest writes belong in `/tmp`.
The adapter does not offer root privileges, configurable outbound networking,
persistent workspaces, a scheduler or hard host disk quota. Docker virtual device
mounts retain their normal semantics; `/tmp` is not the only writable virtual
mount. Daemon log storage is unrotated to avoid silent transcript loss; live output
caps terminate offending guests, but this is **not a hard daemon log disk quota**.
The host owns daemon provisioning, image safety and disk capacity.

## Result and cleanup contract

A nonzero guest exit with complete logs is an ordinary `RunResult`. Setup, wait,
log transport, framing, log deadline and output collection failures return errors;
an incomplete transcript is never reported as successful execution. Cancellation
closes the owned log body and terminates the container. Cleanup is independent
of the caller's cancelled context. `exectool.CleanupError` is joined to the primary
error, preserving its identity. If execution completed but removal failed, the
completed `RunResult` is retained together with the cleanup error; cleanup is
never advertised as confirmed when the daemon refused removal.

## Verification

`go test -race ./...` covers unit/client-mock contracts, strict log framing,
unsupported guarantees, output failures, owned reader cancellation and cleanup.
`TOOLSY_DOCKER_LIVE=1 go test -race -run TestDockerLiveProfile -v .` runs the three
default images plus Linux cgroup, UID/capability, readonly filesystem, disabled
network, bounded scratch and output checks against an actual local daemon.
The live restrictions test currently targets Linux cgroup v2. Skipped live tests
are not evidence of isolation; reports must identify whether they ran.

WithClient accepts the exported focused Client interface. It contains only daemon
capability discovery and owned container lifecycle/log methods; the Docker SDK client
satisfies it directly. Custom ports must report capabilities truthfully, respect
contexts and return owned log readers whose Close unblocks reads. A ContainerWait
response is terminal status, not an acknowledgement. The local workspace must be
visible at the same path to the daemon. Policy fields validated by New are the sole
runtime bounds; there are no zero-value output/log-timeout fallback defaults.
