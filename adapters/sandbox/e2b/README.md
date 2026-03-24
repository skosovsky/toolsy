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
