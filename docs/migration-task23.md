# Migration guide: task23 (State Authority)

Clear break: in-memory mutable state moved from `RunEnv` to `Session`. See also [migration-task22.md](migration-task22.md).

## Session owns state

| Before                                   | After                                                                                      |
| ---------------------------------------- | ------------------------------------------------------------------------------------------ |
| `SetState(env, k, v)` without shared env | `SetSessionState(session, k, v)` or `SetState(env, k, v)` with `env := NewRunEnv(session)` |
| State lost on each `NewRunEnv()`         | One `Session` for the track; reuse `NewRunEnv(session)` per call                           |
| No checkpoint API                        | `session.Export()` / `session.Import(data)`                                                |

## NewRunEnv signature

```go
// Before
env := toolsy.NewRunEnv(toolsy.WithStateStore(store))

// After
session, _ := toolsy.NewSession(reg, opts...)
env := toolsy.NewRunEnv(session, toolsy.WithStateStore(store))
```

`NewRunEnv(nil)` remains valid for DI-only tests (budget middleware, etc.); `SetState`/`GetState` are no-ops without a session.

## Typed restore after JSON

```go
types := toolsy.NewStateTypeRegistry()
types.Register("execution_context", domain.ExecutionContext{})

session, _ := toolsy.NewSession(reg, toolsy.WithStateTypeRegistry(types))
toolsy.SetSessionState(session, "execution_context", ctx)

raw, _ := json.Marshal(session.Export())
// ... persist ...

var payload map[string]any
json.Unmarshal(raw, &payload)

resumed, _ := toolsy.NewSession(reg, toolsy.WithStateTypeRegistry(types))
if err := resumed.Import(payload); err != nil { /* validation */ }
got, ok := toolsy.GetSessionState[domain.ExecutionContext](resumed, "execution_context")
```

`Export` / `Import` never include `Put` dependencies, attachments, or `StateStore` data.

## Shallow Export and dependencies

- `Export()` copies the state map with `maps.Copy`: keys are independent, but **values are shared by reference** until you `json.Marshal` the export. Do not mutate exported values in place before serializing.
- Each `NewRunEnv(session)` creates a **new** dependency map. Reuse the same `*RunEnv` pointer across calls if you want shared `Put` entries, or call `Put` again on each env.

## Session.Execute and Env binding

When `ToolCall.Env` is non-nil and bound to a session (`NewRunEnv(session)`), `Session.Execute` rejects calls where `env.session` is a different `*Session` than the executor (`CodeValidationFailed`).

## Session.Execute vs Registry.Execute

For tracks that use in-memory session state, prefer **`session.Execute`** so env/session binding is validated.

`Registry.Execute` does **not** check that `ToolCall.Env` matches a particular `Session`. If you call `reg.Execute` directly, ensure `ToolCall.Env` is `NewRunEnv(theSameSession)` yourself. Use `env.Session()` (or host bookkeeping) to verify binding when needed.

`Registry.Execute` with `call.Env == nil` still auto-creates `NewRunEnv(nil)` (DI-only; no session state).

## Import and Export edge cases

- `Import(nil)` replaces session state with an empty map (clears all keys).
- `Import` on a nil `*Session` returns `CodeValidationFailed` (not a silent no-op).
- `Export()` on a nil `*Session` returns `nil`; on a non-nil session with no keys, returns a non-nil empty map.

### Nil `*Session` receiver matrix

| API               | `s == nil`                   |
| ----------------- | ---------------------------- |
| `GetSessionState` | `(zero, false)` — no-op read |
| `SetSessionState` | no-op                        |
| `Export`          | `nil`                        |
| `Import`          | `CodeValidationFailed`       |

`Import` fails loudly on a nil session because it replaces all in-memory state; `Get`/`Set` on nil are safe no-ops for defensive host code.

## ValidateRunEnvSession (Registry.Execute)

`Session.Execute` validates `ToolCall.Env` against the executor session. Direct `Registry.Execute` does not. Before `reg.Execute` with session state, call:

```go
if err := toolsy.ValidateRunEnvSession(session, call.Env); err != nil { /* ... */ }
```

## Host pattern (e.g. kosmify)

toolsy does not define a global state registry. The host may:

```go
var GlobalStateRegistry = toolsy.NewStateTypeRegistry()

func init() {
    GlobalStateRegistry.Register(tool.StateKeyExecutionContext, domain.ExecutionContext{})
}

func resume(reg *toolsy.Registry, payload Checkpoint) {
    session, _ := toolsy.NewSession(reg,
        toolsy.WithStateTypeRegistry(GlobalStateRegistry),
    )
    _ = session.Import(payload.SessionData)
    env := toolsy.NewRunEnv(session)
    // Put deps for this process, then session.Execute(..., ToolCall{Env: env}, ...)
}
```

Full runnable sketch: `examples/session_checkpoint/main.go`.
