# Starlark Sandbox Adapter

`starlark` runs code in-process using [go.starlark.net/starlark](https://pkg.go.dev/go.starlark.net/starlark).
It exposes two bindings to scripts:

- `env`: immutable dictionary of environment variables.
- `fs.read(path)`: reads a UTF-8 file from `RunRequest.Files`.

Supported language is `starlark`.
