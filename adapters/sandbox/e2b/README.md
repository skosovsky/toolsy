# E2B Sandbox Adapter

`e2b` models the public E2B workflow as a small Go interface:

1. create a sandbox session
2. write files into `/workspace`
3. run a language-specific command with `RunRequest.Env`
4. kill the sandbox on completion or timeout

The package ships with built-in runtime mappings for `python`, `bash`, `js`,
and `go`, and is designed to sit behind a thin transport-specific Go client.

Custom `Runtime.Command` values intentionally support only a narrow subset:
the script path must appear exactly once as a top-level shell argument.
Wrapper forms such as `sh -c 'python /workspace/main.py'` and nested shell
snippets are rejected at construction time.

Remote sandbox teardown uses a bounded cleanup timeout so stalled control-plane
calls cannot block timeout returns forever.

## Execution contract

Every runtime, including built-ins and already-normalized entrypoints, passes the
same constructor validation. Commands are one POSIX-style command with exactly
one top-level `/workspace/<ScriptName>` argument following an executable. Missing,
mismatched, repeated, or embedded script references are rejected. Shell operators,
expansions, multiline commands, command-mode wrappers (`sh -c`, `bash -ec`, etc.)
and mixed quoting of the script token are unsupported. Literal single/double
quoting and escaped spaces are supported. Entrypoint normalization rewrites that
one argument and materializes code at the matching canonical path.

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
