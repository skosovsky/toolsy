# Sandbox deadlines and host capability policy

`exectool.New` publishes language/code/env/files, validates before dispatch and
passes the caller's context to `Sandbox.Run`. There is no timeout field in
RunRequest or the model-facing schema. The host selects the backend and policy;
changing backend changes capabilities, not business authorization. Manifest hints
including Dangerous/RequiresConfirmation do not authenticate an approval grant.

| Bound | Owner/phase | Behavior and limits |
| --- | --- | --- |
| Caller cancellation/deadline | Host context for Run/Execute | Parent cancellation remains observable; execution deadline maps to ErrTimeout on supported backends. Ports must cooperate with context and owned IO close; not hard syscall/foreign callback preemption |
| Docker Policy.LogTimeout | Host backend configuration; default5s, starts at log acquisition/follow after container start | Bounds the log phase while execution may still be running; effective context deadline is earlier of parent and log deadline. Expiry closes owned body, cancels wait/requests kill; no incomplete-output success. Without parent interruption it is ErrSandboxFailure collection error, not ErrTimeout/context.DeadlineExceeded |
| Docker resource/input/output bounds | Mandatory validated backend Policy | Memory/CPU/PIDs/tmpfs/input/files/per-stream output are different budgets, not elapsed seconds; cannot disable mandatory bounds with zero |
| Starlark MaxExecutionSteps | Positive finite interpreter budget | Limits interpreted instructions, not seconds or memory. Parsing and running Go built-ins cannot be preempted; caller cancellation is cooperative |
| Host output collection | Backend-owned5s Unix collection budget / non-Unix WaitDelay | Stops incomplete collection; owned group cleanup has platform/escaped-process limits, no hostile-code isolation |
| Wazero linear memory | Backend pages plus caller execution context | Per linear memory quota, no Go process/compiler/table allocation quota; local filesystem calls are cooperative |
| E2B transport | Borrowed client/service | Client must honor contexts and bounded writers; mocks do not certify remote interruption or isolation |
| Cleanup | Fresh contexts independent of cancelled caller (backend defaults5s where applicable) | Separate kill/remove/close phases can outlive execution deadline. Filesystem checks are between syscalls; a stalled syscall is not hard bounded. Failed removal/close is diagnostic, not confirmed cleanup |

A timeout does not prove absence of an external effect. Inspect the typed
RunOutcomeError/CleanupError and reconcile through trusted host evidence before
considering another dispatch. Parent interruption is classified before collection
failure; secondary cleanup deadlines never become execution timeout authority.
See [result/cleanup classification](sandbox-result-contract.md) and the
[backend capability matrix](../adapters/sandbox/README.md).

## Configure the backend before dispatch

The local [Starlark host recipe](../adapters/sandbox/starlark/examples/policy/main.go)
constructs a positive step budget and a caller context deadline, then creates the
generic exec tool. Run from root:

```sh
go -C adapters/sandbox/starlark run ./examples/policy
```

It executes only an effect-free print in-process. Tests exercise exact output,
pre-cancelled caller and step exhaustion, with no timing-based hard-preemption
claim. Resource policy is host configuration, not generated/model input.

For Docker, start with mandatory defaults and change selected positive fields:

```go
policy := docker.DefaultPolicy()
policy.LogTimeout = 20 * time.Second
sb, err := docker.New(docker.WithPolicy(policy))
if err != nil { return err }
ctx, cancel := context.WithTimeout(parent, 10*time.Second)
defer cancel()
result, err := sb.Run(ctx, request)
```

This is a construction snippet for a host with a provisioned local Linux daemon
and images; it does not run or certify Docker here. Parent deadline can arrive
before the20s log deadline; the log clock does not cover Info/materialization/
create/start. For a long allowed guest, choose both caller deadline and LogTimeout
consciously: a long caller deadline alone does not raise the default5s log bound.
Cleanup still uses independent contexts. Unit fixtures
TestActualLogDeadlineClosesBodyAndRemainsInfrastructure and
TestRunKillsContainerOnTimeout distinguish those failure phases. The latter
uses a phase-triggered deadline signal after resource acquisition; it does not
measure elapsed execution time. The separate LogTimeout fixture uses the actual
collection clock. Optional live
Docker checks require explicit opt-in and are reported separately.

Every backend's detailed policy remains authoritative:
[Docker](../adapters/sandbox/docker/README.md),
[Starlark](../adapters/sandbox/starlark/README.md),
[Host](../adapters/sandbox/host/README.md),
[Wazero](../adapters/sandbox/wazero/README.md),
[E2B](../adapters/sandbox/e2b/README.md).
