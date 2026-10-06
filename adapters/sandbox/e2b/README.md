# E2B Sandbox Adapter

`e2b` models the public E2B workflow as a small Go interface:

1. create a sandbox session
2. write files into `/workspace`
3. run a language-specific command with `RunRequest.Env`
4. kill the sandbox on completion or timeout

The package ships with built-in runtime mappings for `python`, `bash`, `js`,
and `go`, and is designed to sit behind a thin transport-specific Go client.

## Literal executable and argv

`Runtime.Command` is a literal executable (for example `python` or `/opt/my python`),
not a shell command line. `Runtime.Args` contains literal arguments, including
exactly one `/workspace/<ScriptName>` argument. `New` canonicalizes ScriptName and
rewrites only that exact argument; code is uploaded at the same canonical path.
The canonical script path must also occur exactly once after rewriting; supplying
both noncanonical and canonical script references is rejected. Other arguments,
including empty strings, spaces, quotes and shell metacharacters,
remain literal. Arguments/executable must be valid UTF-8 without NUL. Empty or
whitespace-only executable, missing/repeated/mismatched script arguments and invalid
relative script paths are rejected before provisioning.

```go
sb, err := e2b.New(client, e2b.WithRuntime("custom", e2b.Runtime{
    Command:    "python",
    Args:       []string{"-u", "/workspace/dir/../main script.py", "$literal"},
    ScriptName: "dir/../main script.py",
}))
```

This dispatches executable `python` and arguments `-u`,
`/workspace/main script.py`, `$literal` separately. There is no shell parser,
quoting pass, expansion or shell-operator interpretation in the adapter. Other
args that embed script-like text are not rewritten. Runtime arguments are copied
at option creation, construction and each dispatch, so a client's argument mutation
does not change subsequent calls. See the runnable [public example](example_test.go).

`Session.StartAndWait` accepts `(ctx, command, args, env, stdout, stderr)`.
The injected client must preserve literal argv semantics. If its transport/SDK
only accepts a command string, that client owns the **single serialization
boundary** and must independently escape each argument for the actual target
transport. Joining raw strings or parsing them again changes the contract. This
module supplies no universal shell serializer. Host-selected programs can still
interpret their own flags and execute code (including explicit shell interpreters);
argv is not an executable allowlist or isolation guarantee. Such semantics remain
trusted host runtime policy.

## Execution and output contract

`CommandResult` contains only `ExitCode`. Supplied stdout/stderr writers are the
only output source; clients must not return separately buffered output. There is
no fallback around bounded writers. Remote sandbox teardown uses a fresh bounded
five-second cleanup context.

The injected client owns remote transport and infrastructure. It must honor
contexts for provisioning, upload, execution and teardown, stream stdout/stderr
through the supplied bounded writers, and propagate writer/transport errors.
Adapter cancellation is therefore cooperative with that client. This package does
not assert remote filesystem, network, memory, CPU or privilege isolation without
an independently verified client/service capability. Request Env is passed to the
client; remote environment inheritance is that client's responsibility.

Nonzero guest exit with complete output is a result. Provisioning/upload/transport
failures and incomplete output are errors. Each output stream is capped at 256 KiB.
Teardown always receives a fresh five-second context independent of caller
cancellation. If Kill fails or times out, `*exectool.CleanupError` exposes the
backend, operation and cause, preserving the original result/error. Successful
cleanup means the client returned success; no additional remote destruction claim
is made. Unit mocks verify requests and error semantics, not live remote isolation.

## D27 migration

Replace `Runtime{Command: "python /workspace/main.py", ScriptName: "main.py"}` with
`Runtime{Command: "python", Args: []string{"/workspace/main.py"}, ScriptName: "main.py"}`.
For Go, use `Command: "go", Args: []string{"run", "/workspace/main.go"}`.
Remove pre-quoting/escaping from literal paths and args. Update client implementations
to accept the new `args []string` parameter and stream all output through supplied
writers; remove `CommandResult.Stdout`/`Stderr` initializers. Keep `ExitCode`.
See [task41 migration](../../../docs/migration-task41.md).
