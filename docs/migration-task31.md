# Migration Task31: Typed execution boundary

Task31 moves production host integration from string-keyed runtime glue to typed execution contracts.

## What changed

- `ToolCall` now carries `CallContext` for typed subject/scope and request-local values.
- `RunEnv` still carries DI and session access, but subject/scope should not be smuggled through `Put`, `Require`, `Lookup`, `SetState`, or `GetState`.
- `WithPolicy` runs before raw validators, typed validators, and handlers. Denial returns `CodePolicyDenied`.
- `NewRequirementsPolicy` and `WithRequirementsPolicy` enforce `ToolManifest.Requirements` against typed subject/scope on the registry/view/session execution path; any non-empty requirements fail closed when no requirements policy is attached.
- `WithAuthorizer` and `WithAuthorization` receive `AuthorizationRequest`, not separate manifest/input arguments.
- `Registry.View` creates a first-class capability view with tool names, required tool names, manifest digest, durable snapshot identity, optional policy, and shared lifecycle.
- `Registry.RestoreView` validates snapshot identity and manifest digest against the current registry before recreating a view.
- `NewTypedTool` now takes `TypedToolSpec[TSubject, TScope, TArgs, TResult, TEffect]`.
- Typed tool handlers receive `TypedCallContext[TSubject, TScope]` and return `ToolResult[TResult, TEffect]`.
- `ToolOutcome` now reports `Status`, `TypedResult`, `EmptyResult`, `Noop`, and `Effects`.
- `StandardCallParser` normalizes continuation chunks with missing tool names when a call id can be resolved.
- `ParseExactlyOne[T]` extracts and decodes a single expected tool call.

## New typed tool shape

```go
tool, err := toolsy.NewTypedTool(toolsy.TypedToolSpec[
    UserSubject,
    WorkspaceScope,
    SearchArgs,
    SearchResult,
    SearchEffect,
]{
    Name: "search",
    Description: "Search documents",
    Policy: func(ctx context.Context, req toolsy.TypedPolicyRequest[UserSubject, WorkspaceScope, SearchArgs]) toolsy.Decision {
        return toolsy.AllowDecision()
    },
    Handler: func(
        ctx context.Context,
        call toolsy.TypedCallContext[UserSubject, WorkspaceScope],
        env *toolsy.RunEnv,
        args SearchArgs,
    ) (toolsy.ToolResult[SearchResult, SearchEffect], error) {
        out := toolsy.NewToolResult[SearchResult, SearchEffect](SearchResult{})
        out.Effects = []SearchEffect{{}}
        return out, nil
    },
})
```

For tools that do not use subject or scope, use `toolsy.NoSubject` and `toolsy.NoScope`.

## Migration checklist

- Replace string-keyed subject/scope lookups with `ToolCall.CallContext`.
- Replace manual permission branching with `WithRequirementsPolicy` or `RegistryViewSpec.Policy: NewRequirementsPolicy(...)`.
- Replace ad-hoc scoped registry structs with `Registry.View` and `RegistryViewSnapshot`.
- Replace raw `ArgsJSON` wrapper stacks with `NewTypedTool` validators and typed policy.
- Replace byte-only outcome classification with `ToolOutcome.Status`, `DecodeOutcomeAs`, `DecodeOutcomeEffectsAs`, and `Controls`.
- Remove service-side tool-call continuation normalization when call ids carry enough information for `StandardCallParser`.

## Error model

Policy denial is not a schema error and not an internal error. It returns `ToolError.Code == CodePolicyDenied`.

Capability denial through a registry view returns `ToolError.Code == CodeCapabilityDenied`.

Business failures still live in `ToolOutcome.ExecutionError` for `Session.RunCall`. Infrastructure and pre-tool boundary failures return as `error` and mark `ToolOutcome.Status == OutcomeInfrastructureError`; this includes policy and capability denial.
