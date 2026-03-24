# Exec Tool

`github.com/skosovsky/toolsy/exectool` provides a single generic tool,
`exec_code`, backed by a pluggable sandbox adapter.

The LLM-facing schema includes:

- `language`
- `code`
- optional `env`
- optional UTF-8 text `files`

Timeouts are infrastructure-controlled and configured only in Go via
`exectool.WithTimeout(...)`.

## Example

```go
sb := starlarksandbox.New()

tool, err := exectool.New(
    sb,
    exectool.WithTimeout(2*time.Second),
    exectool.WithAllowedLanguages("starlark"),
)
if err != nil {
    panic(err)
}
```

Low-level adapters exchange `exectool.RunRequest` and `exectool.RunResult`,
which makes it possible to swap `starlark`, `host`, `wazero`, `docker`, or
`e2b` sandboxes without changing agent business logic.
