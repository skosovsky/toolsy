# Migration guide: task28 (Engine hardening)

Task28 closes remaining architectural leaks in the v1.0 tool engine.

## Strict state codecs

Opt in with `WithStrictStateCodecs(true)` on `NewSession`. Every **non-nil** session state key must have a registered codec via `WithStateCodecRegistry`.

Clear paths do **not** require codecs:

- Empty snapshot `{}` clears all in-memory state.
- JSON `null` for a key removes that key (explicit clear).

Export skips nil state values (keys removed from the wire without error).

```go
codecs := toolsy.NewStateCodecRegistry()
_ = toolsy.RegisterJSONCodec[Prefs](codecs, "prefs")

sess, err := toolsy.NewSession(reg,
    toolsy.WithStateCodecRegistry(codecs),
    toolsy.WithStrictStateCodecs(true),
)
```

Import/export of data without a codec returns `*ToolError` with `CodeStateCodecMissing` (`Retryable: false`).

## Error chunk normalization

Tools must emit error result chunks with `MimeType: application/vnd.toolsy.tool-error+json` (use `toolsy.NewErrorChunkFromErr`).

Legacy `MimeTypeText` + `IsError: true` chunks are **normalized** before delivery into `CodeInternal` structured wire (`MimeTypeToolErrorJSON`).

**RunCall routing:**

| Error source                                                                              | `RunCall` result                                             |
| ----------------------------------------------------------------------------------------- | ------------------------------------------------------------ |
| Business failures (`CodeValidationFailed`, `CodeBudgetExceeded`, handler `*ToolError`, …) | `(outcome, nil)` with `outcome.ExecutionError != nil`        |
| Normalized malformed chunks (`CodeInternal` from legacy text wire)                        | `(zero, infra *ToolError)` — infrastructure, not LLM-fixable |
| Registry/session infra (`CodeToolNotFound`, shutdown, max steps, …)                       | `(partial outcome, infra error)`                             |

Do not use `strings.Contains` on chunk text; use `AsToolError` on `outcome.ExecutionError` or the infra `err`.

## Snapshot hydration errors

`ImportSnapshot`, `ExportSnapshot`, and `NewSessionSnapshotFromJSON` failures (unsupported version, corrupt JSON, codec decode/type errors) return `*ToolError` with **`CodeInternal`** and **`Retryable: false`**. These are infrastructure failures — not LLM-correctable validation errors.

## Examples

Host-facing loops should use `Session.RunCall` + `DecodeOutcomeAs`:

- `examples/run_call/main.go`
- `examples/session_snapshot/main.go`
- `examples/resiliency/main.go`
- `examples/calculator/main.go`

Low-level `Registry.Execute` + manual chunk assembly remains for streaming adapters only.

See also [adr-task28-hardening.md](adr/adr-task28-hardening.md).
