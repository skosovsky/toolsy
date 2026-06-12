# toolsy examples

Run from the repository root (module `github.com/skosovsky/toolsy`):

```bash
go run ./examples/run_call
go run ./examples/session_snapshot
go run ./examples/resiliency
go run ./examples/calculator
```

- **`run_call`** — `RunCall`, `ToolOutcome`, `DecodeOutcomeAs`, validation via `ExecutionError`
- **`session_snapshot`** — `ExportSnapshot` → JSON → `ImportSnapshot` with `StateCodecRegistry` and strict codecs
- **`resiliency`** — host retry loop with `Session.RunCall` and `NewTypedTool`
- **`calculator`** — minimal `RunCall` + `DecodeOutcomeAs` with two tools

Other examples (`streaming`, `full_agent`) demonstrate chunk-level streaming APIs and legacy patterns.
