# human toolkit

**Description:** Conversation tools requesting human review or clarification. Each tool yields a typed control pause signal and returns `toolsy.ErrPause`. Their free-text payload is not an authenticated approval grant and does not permit a dangerous action.

## Tools

- `request_approval` — conversational request for human review; not a bound execution approval
- `ask_human_clarification` — clarification question for the user

## Control flow

For action authorization, configure `OperationProfile` on the actual action executor and use its `PendingApprovalError` challenge. The host authenticates the approver, persists the bound `ApprovalGrant` in the same atomic operation store, and resumes the original logical operation through current policy. Never convert `request_approval` action/reason or arbitrary human/model text directly into a grant. These conversation tools remain independent of the protected action contract; there is no fallback from a missing/expired grant to their payload.

- **Pause-first behaviour:** both tools emit `Chunk{Event: toolsy.EventControl, Control: *toolsy.PauseSignal}` with JSON payload in `PauseSignal.Reason`, then return `toolsy.ErrPause`.
- Orchestrator is responsible for checkpointing state, pausing the run, and resuming later with external input.
- Manifest uses `CompletionPolicy: silent_yield`.

## Example orchestrator handling

```go
err := reg.Execute(ctx, call, func(c toolsy.Chunk) error {
    if pause, ok := c.Control.(*toolsy.PauseSignal); ok {
        checkpoint(pause.Reason)
    }
    return nil
})
if toolsy.IsControlError(err) {
    return pauseRun(err)
}
```

## Payload shapes

Approval:

```json
{"kind":"approval","action":"delete","reason":"user asked"}
```

Clarification:

```json
{"kind":"clarification","question":"Which button?"}
```

## Limits and trust boundary

The default maximum pause payload is 16 KiB of complete encoded JSON in
`PauseSignal.Reason`, including escaped strings and field names. Hosts override
it with `WithMaxPayloadBytes`; zero or negative explicit values are construction
errors. Oversize action/reason/question returns a validation error before a
control chunk is yielded. Input text is never truncated or promoted to authority.
There is no collection pagination or external provider in this toolkit.

## Compose conversation with a bound action

A conversation pause may trigger the host UI, but the host must obtain the
challenge from the actual action's `OperationProfile`. Display that challenge's
`DisplayJSON`, authenticate the approver and persist a grant bound to its exact
`Binding`. Resume the original operation through the protected executor:

```go
// pending came from errors.As(actionExecutionError, &pending), not pause.Reason.
err := store.PutGrant(ctx, toolsy.ApprovalGrant{
    ID: hostGrantID, Issuer: hostIssuer, Binding: pending.Challenge.Binding,
    IssuedAt: now, ExpiresAt: now.Add(time.Minute),
})
if err != nil { return err }
// The host PrepareOperation callback selects hostGrantID for this operation.
return actionRegistry.Execute(ctx, originalActionCall, yield)
```

No grant means no dispatch, even after `request_approval` paused successfully.
Current action policy is evaluated again on resume and replay. The executable
[composition fixture](bound_approval_test.go) covers pause, no-grant rejection,
host-issued grant, exactly one action dispatch, replay and policy revocation.
The complete trusted-local-host example is
[approval_journal](../../examples/approval_journal/main.go).
