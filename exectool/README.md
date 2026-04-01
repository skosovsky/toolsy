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

## Example

```go
sb := starlarksandbox.New()

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
