# Exec Tool

`github.com/skosovsky/toolsy/exectool` provides a single generic tool,
`exec_code`, backed by a pluggable sandbox adapter.

The LLM-facing schema includes:

- `language`
- `code`
- optional `env`
- optional UTF-8 text `files`

Time limits come only from the [`context.Context`] passed into `Sandbox.Run`
(e.g. `context.WithTimeout`, or a wrapper like `routery.Timeout` around the
tool). Sandboxes do not apply a separate duration from `RunRequest`.

## Manifest policy

`exec_code` is marked `Dangerous` by default in `ToolManifest`. For
human-in-the-loop sandboxes (for example `adapters/sandbox/host`), add
confirmation via `WithToolOptions`:

```go
tool, err := exectool.New(
    sb,
    exectool.WithToolOptions(
        toolsy.WithRequiresConfirmation(),
    ),
)
```

Policy flags are manifest fields (`ReadOnly`, `Dangerous`, `RequiresConfirmation`, …), not `Metadata` keys.

## Example

```go
sb, err := starlarksandbox.New(starlarksandbox.DefaultConfig())
if err != nil {
    panic(err)
}

tool, err := exectool.New(
    sb,
    exectool.WithAllowedLanguages("starlark"),
)
if err != nil {
    panic(err)
}
```

Low-level adapters exchange `exectool.RunRequest` and `exectool.RunResult`,
which makes it possible to swap `starlark`, `host`, `wazero`, `docker`, or
`e2b` sandboxes without changing agent business logic.

Backend quotas and isolation differ: see the [capability matrix](../adapters/sandbox/README.md).
Execution, collection and cleanup follow the [result contract](../docs/sandbox-result-contract.md).
The host owns approval, durable operation recording and reconciliation of unknown effects;
`journal_integration_test.go` exercises a collection failure after an external effect with the file journal.
