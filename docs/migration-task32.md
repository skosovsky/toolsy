# Migration notes for task32

Task32 is a breaking API change. It moves args binding, session rebinding, snapshot null policy, delivery classification, and generic tool policy wrappers into `toolsy`.

## Typed args binding

`TypedToolSpec.Handler` now receives `ValidatedArgs[TArgs]`.

Use `args.Value` for the typed handler value. Use `args.Raw` when downstream execution needs the canonical sanitized JSON. Use `args.Metadata` for host-owned bind metadata.

When an `ArgsBinder` is used to guard a generic tool, it must return canonical `Raw`. Empty `Raw` fails closed so the original unsafe input is not forwarded after policy approval.

```go
tool, err := toolsy.NewTypedTool(toolsy.TypedToolSpec[
    Subject,
    Scope,
    SearchArgs,
    SearchResult,
    struct{},
]{
    Name: "search",
    Description: "Search",
    ArgsBinder: func(ctx context.Context, req toolsy.ArgsBindRequest) (toolsy.ValidatedArgs[SearchArgs], error) {
        return toolsy.ValidatedArgs[SearchArgs]{
            Value: SearchArgs{Query: "canonical"},
            Raw:   []byte(`{"query":"canonical"}`),
        }, nil
    },
    Handler: func(
        ctx context.Context,
        call toolsy.TypedCallContext[Subject, Scope],
        env *toolsy.RunEnv,
        args toolsy.ValidatedArgs[SearchArgs],
    ) (toolsy.ToolResult[SearchResult, struct{}], error) {
        return toolsy.NewToolResult[SearchResult, struct{}](SearchResult{}), nil
    },
})
```

Remove call-context or session-state side channels that only existed to pass sanitized args from validation to the handler.

## Session binding and checkpointing

Sessions now expose their execution binding:

```go
checkpoint, err := sess.ExportCheckpoint()
restored, err := view.NewSessionFromCheckpoint(checkpoint, opts...)
err = view.RebindSession(sess)
```

The binding validates root policy digest, view identity, manifest digest, visible tool set, and state schema digest. Do not keep separate `session registry` or `view id` fields in host state just to decide whether a session is still attached to the right capability boundary.

Snapshots exported by `ExportSnapshot` are stamped with the session binding. Importing that snapshot into a session bound to another registry/view/schema fails before state hydration. Use `ExportCheckpoint`/`NewSessionFromCheckpoint` when persisting resumable sessions.

Root registry policies must provide a stable ID through `WithPolicy` or `WithRequirementsPolicy`:

```go
reg, err := toolsy.NewRegistryBuilder(
    toolsy.WithPolicy("root-policy", policy),
).Add(tools...).Build()
```

Policy-bound views must provide a stable `PolicyID`:

```go
view, err := reg.View(toolsy.RegistryViewSpec{
    ToolNames: []string{"search"},
    PolicyID: "search-policy",
    Policy: policy,
})
restored, err := reg.RestoreView(snapshot, policy, "search-policy")
```

## Snapshot import null policy

Registered state slots are non-null by default on import.

```go
codecs := toolsy.NewStateCodecRegistry()
_ = toolsy.RegisterJSONCodec[Prefs](codecs, "prefs", toolsy.WithStateSlotRequired())
sess, _ := toolsy.NewSession(reg,
    toolsy.WithStateCodecRegistry(codecs),
    toolsy.WithStrictStateCodecs(true),
)
```

Use `WithStateSlotNullable(true)` only when explicit JSON `null` is part of the state schema. Unknown `null` keys in strict mode fail with `CodeStateCodecMissing`.

Use `WithStateSlotSchemaID("...")` when a custom codec's decode semantics change without a Go type change. The session binding includes codec/value type and schema ID.

## Tool delivery envelope

`Chunk` and `ToolOutcome` expose `ToolEnvelope`, which classifies structured results and errors without parsing JSON shape:

```go
outcome, err := sess.RunCall(ctx, call)
if outcome.Envelope.Kind == toolsy.ToolEnvelopeKindError {
    _ = outcome.Envelope.Error.Code
}
```

Do not inspect strings or arbitrary JSON objects to guess whether a payload is a tool result or tool error.

## Policy-aware generic tools

Use `NewPolicyToolFromSpec` to build a policy-aware generic tool in one step:

```go
tool, err := toolsy.NewPolicyToolFromSpec(toolsy.ToolPolicyConstructorSpec[Subject, Scope, Args]{
    Name: "search",
    Description: "Search",
    RawJSONSchema: []byte(`{"type":"object","properties":{"q":{"type":"string"}}}`),
    Handler: rawHandler,
    ArgsBinder: bindArgs,
    Policy: typedPolicy,
})
```

`ArgsBinder` is required for policy-aware generic tools. It must return canonical `Raw` bytes; the wrapped raw handler receives those bytes, not the original `ToolInput.ArgsJSON`.

Use `NewPolicyTool` when the base generic tool already exists and must be hardened:

```go
tool, err := toolsy.NewPolicyTool(toolsy.ToolPolicySpec[Subject, Scope, Args]{
    Tool: baseTool,
    Requirements: toolsy.ToolRequirements{
        Permissions: []toolsy.Permission{"read"},
    },
    ArgsBinder: func(ctx context.Context, req toolsy.ArgsBindRequest) (toolsy.ValidatedArgs[Args], error) {
        return toolsy.ValidatedArgs[Args]{
            Value: Args{},
            Raw:   []byte(`{}`),
        }, nil
    },
    Policy: func(ctx context.Context, req toolsy.TypedPolicyRequest[Subject, Scope, Args]) toolsy.Decision {
        return toolsy.AllowDecision()
    },
})
```

Remove local wrappers whose only job was to sanitize raw args, enforce typed policy, or inject requirements before delegating to a generic tool.

`NewTool` and `WithValidator` are low-level primitives. They are reject-only and do not replace `ArgsBinder` on production agent execution paths.
