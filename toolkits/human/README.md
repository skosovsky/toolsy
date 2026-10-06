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
