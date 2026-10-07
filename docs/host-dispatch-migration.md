# Migrating host glue to full dispatch

Public function signatures remain unchanged. Fresh built-in producers now reject
`toolsy.replay_source` in envelope metadata: remove that key from handler results.
Only execution profiles may attach replay provenance. `Chunk` now has a private
invocation-local replay snapshot; replace positional `Chunk{...}` literals with
keyed fields such as `Chunk{Event: toolsy.EventResult, Data: payload}`. The private
snapshot is not serialized and is not an approval credential. Forward genuine
child replay chunks unchanged; modifying their payload/effects/envelope invalidates
that provenance.

Do not manufacture replay metadata to
suppress effects. This is a behavior break; a rejected result after an external
effect remains uncertain under the operation profile and needs reconciliation.

Change the code that translates native provider
calls into tool execution and the code that schedules the next effect.

Before, a convenience port often does this:

```go
outcome, err := session.RunCall(ctx, toolsy.ToolCall{
    ToolName: name, Input: toolsy.ToolInput{ArgsJSON: rawArgs},
})
return outcome.Result, err
```

This loses provider identity and business errors returned with nil infrastructure
error. Returning a pause error does not stop a caller's independent batch loop.

After, copy the original native call ID and use a trusted host request:

```go
req := recipe.Request[Subject, Scope, Activity]{
    CallID: providerCall.ID, Tool: providerCall.Name, Args: json.RawMessage(providerCall.Args),
    OperationID: storedIntent.ID, AttemptID: freshAttemptID,
    ActivityIdentity: storedActivity, Subject: authenticatedSubject, Scope: currentScope,
    GrantID: authenticatedGrantID, PolicyFingerprint: currentPolicyFingerprint,
    DependencyFingerprint: currentDependencyFingerprint,
}
results, admissionErr := dispatcher.Dispatch(ctx, []recipe.Request[Subject, Scope, Activity]{req}, false)
```

Use manifests from `dispatcher.Manifests()` for prompt tool declarations. Serialize
the source schema, preserving dialect and exact-number semantics. Do not infer it
again or turn visibility/capabilities into authorization. Install `recipe.Profile`
in the registry before creating the restricted view/Session; see the runnable
example for actual setup, not just the abbreviated request above.

Handle admission failure before any effects. Each Result retains Outcome and Err;
process its Decision before scheduling another tool or asking the model again.
Only continue permits the next sequential invocation. Approval/pause, uncertain,
yield/halt and fault return control to the host. Deny, correction and business
failure are separate outcomes with no implicit retry. Serialize only Result.Model;
a nil projection is intentionally not model-deliverable. Progress, controls,
effects, raw errors and diagnostic causes belong to trusted host consumers.

New intents receive fresh OperationIDs even if name+args are identical. Redelivery
retains the stored OperationID and ActivityIdentity, with a new CallID and attempt.
Approval must come from an authenticated issuer bound to the prepared challenge.
Edited arguments or changed subject/scope, tool/schema/view/policy or dependency
identity require new authorization; an old grant is not reusable authority.

After a delivery failure, Inspect is a read-only host diagnostic. If completed,
obtain replay through the currently authorized Session. Never serialize the raw
journal record to the model. Unknown/in-progress requires trusted reconciliation
against external evidence, not a blind handler retry or replacement grant.
Suppress reducer effects on trusted replay provenance. Add an atomic reducer/outbox
boundary if effects must survive host crashes without duplicate application.

The optional integration module shows actual native call mapping, scoped schema,
guard decisions and durable activity recovery. Keep caller-specific dependencies
there or in your own application; they do not become core dependencies. Existing
one-shot helpers remain usable for their simple JSON contract; switch full lifecycle
callers to one host execution owner.
