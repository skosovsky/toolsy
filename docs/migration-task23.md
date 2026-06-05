# Migration guide: task23 (State Authority on Session)

## Breaking: `NewRunEnv(session *Session, opts ...)`

| Before                         | After                                                                                    |
| ------------------------------ | ---------------------------------------------------------------------------------------- |
| `toolsy.NewRunEnv(opts...)`    | `toolsy.NewRunEnv(nil, opts...)` for DI-only (middleware, tests without in-memory state) |
| Shared in-memory state via env | `sess, _ := toolsy.NewSession(reg, ...)` then `env := toolsy.NewRunEnv(sess, opts...)`   |

`SetState` / `GetState` on an env created with `session == nil` are no-ops / false.

## Host overlay (before tools run)

```go
toolsy.SetSessionState(sess, "routing", overlay)
env := toolsy.NewRunEnv(sess)
toolsy.Put(env, "db", db)
```

Handlers may still use `toolsy.SetState(env, key, val)` when `env` is bound to the same session.

## One RunEnv for DI (deps vs in-memory state)

- **In-memory state** is shared across any `NewRunEnv(sess)` bound to the same `*Session`.
- **Dependencies** (`Put` / `Require`) live in each env's private `deps` store. Every `NewRunEnv(sess)` creates a **new** deps map.
- Reuse the **same** `*RunEnv` pointer across tool calls when DI must persist (budget tracker, DB handle, etc.).
- Creating a fresh `NewRunEnv(sess)` per call is fine when only session state must carry over and you re-`Put` deps each time.

## Checkpoints: Export → JSON → Import

```go
dump := sess.Export()
raw, _ := json.Marshal(dump)
// persist raw ...

var wire map[string]any
_ = json.Unmarshal(raw, &wire)
newSess, _ := toolsy.NewSession(reg, toolsy.WithStateTypeRegistry(types))
_ = newSess.Import(wire)
env := toolsy.NewRunEnv(newSess)
// fresh Put() for DI — deps are never in Export()
```

**Do not mutate** the map returned by `Export()` in place; treat it as a snapshot.

`Export()` performs a shallow copy of values. Shared references remain shared until JSON serialization.

## StateTypeRegistry (typed keys after JSON)

After `json.Unmarshal` into `map[string]any`, struct values become `map[string]any`.
Register concrete types so `Import` can restore them:

```go
types := toolsy.NewStateTypeRegistry()
if err := types.Register("ctx", MyContext{}); err != nil {
    return err
}
sess, _ := toolsy.NewSession(reg, toolsy.WithStateTypeRegistry(types))
```

Unregistered keys are stored as-is (primitives or generic maps). After JSON without registry, struct keys remain `map[string]any`; use `StateTypeRegistry` or read generic maps on the host.

`Register` accepts value or pointer prototypes (`MyStruct{}` and `&MyStruct{}` register the same type).

`Import` returns a standard `error` for registered-key deserialization failures (not `*ToolError`); wrap or inspect the message for routing.

## Session.Execute vs Registry.Execute

- `Session.Execute` validates `call.Env` is bound to that session when `call.Env != nil` (`ValidateRunEnvSession`).
- `Session.Execute` with `call.Env == nil` skips validation; tools using `SetState` will not persist in-memory state — pass `NewRunEnv(sess)` for stateful agent loops.
- `Registry.Execute` does **not** validate env binding; call `ValidateRunEnvSession` yourself when needed.

## Host checklist (external repos, e.g. kosmify — not part of toolsy)


- [ ] Replace `NewRunEnv()` with `NewRunEnv(nil)` or `NewRunEnv(session)`.
- [ ] Move long-lived in-memory state to `Session`; wire one `RunEnv` per session (or per call on the same session).
- [ ] Add `StateTypeRegistry` for struct keys restored from DB JSON.
- [ ] Persist `session.Export()` only; re-`Put` dependencies after `Import`.
- [ ] Use `Session.Execute` for stateful agent tracks with env validation.

Async middleware behavior (background unwrap-wrap) is unchanged; see task24 / README async section.
