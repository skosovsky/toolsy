# toolsy examples

Run from the repository root (module `github.com/skosovsky/toolsy`):

```bash
go run ./examples/run_call
go run ./examples/session_snapshot
```

- **`run_call`** — `RunCall`, `ToolOutcome`, `DecodeOutcomeAs`, validation via `ExecutionError`
- **`session_snapshot`** — `ExportSnapshot` → JSON → `ImportSnapshot` with `StateCodecRegistry`

Other examples (`calculator`, `streaming`, `full_agent`, `resiliency`) demonstrate chunk-level APIs and legacy patterns.
