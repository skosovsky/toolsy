# human toolkit

**Description:** Conversation tools requesting human review or clarification. Each tool yields a typed control pause signal and returns `toolsy.ErrPause`. Their free-text payload is not an authenticated approval grant and does not permit a dangerous action.

## Tools

- `request_human_review` — conversational request for human review; not a bound execution approval
- `ask_human_clarification` — clarification question for the user

## Control flow

For action authorization, configure `OperationProfile` on the actual action executor and use its `PendingApprovalError` challenge. The host authenticates the approver, persists the bound `ApprovalGrant` in the same atomic operation store, and resumes the original logical operation through current policy. Never convert `request_human_review` action/reason or arbitrary human/model text directly into a grant. These conversation tools remain independent of the protected action contract; there is no fallback from a missing/expired grant to their payload.

- **Pause-first behaviour:** both tools emit `Chunk{Event: toolsy.EventControl, Control: *toolsy.PauseSignal}` with JSON payload in `PauseSignal.Reason`, then return `toolsy.ErrPause`.
- Host decides whether to persist state, pause its continuation and resume later; core does not persist a workflow or cancel other calls.
- Manifest uses the host routing hint `CompletionPolicy: silent_yield`.

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

Human review intent:

```json
{"kind":"human_review","action":"delete","reason":"user asked"}
```

Clarification:

```json
{"kind":"clarification","question":"Which button?"}
```

## Limits and trust boundary

The default maximum pause payload is 16 KiB of complete encoded JSON in
`PauseSignal.Reason`, including escaped strings and field names. Hosts override
it with `WithMaxPayloadBytes`; zero or negative explicit values are construction
errors. A limit above core `MaxControlBytes` (64 KiB) is also rejected at
construction. Oversize action/reason/question returns a validation error before a
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

No grant means no dispatch, even after `request_human_review` paused successfully.
Current action policy is evaluated again on resume and replay. The executable
[composition fixture](bound_approval_test.go) covers pause, no-grant rejection,
host-issued grant, exactly one action dispatch, replay and policy revocation.
The complete trusted-local-host example is
[approval_journal](../../examples/approval_journal/main.go).

## Review naming and authority

Configure conversational naming with `WithReviewName` and
`WithReviewDescription`. The old approval-named options/default tool and payload
kind have been removed; see [migration](../../docs/migration-task41.md#d03--human-review-is-an-intent).
Action/reason text is untrusted conversation data. It has no authenticated subject,
canonical operation binding, policy fingerprint, issuer, expiry or consume-once
semantics. A reply such as "approved" does not fill these missing properties.
Use the actual protected operation's challenge, authenticated host UI/issuer and
current action policy. Do not use this conversation payload as DisplayJSON or grant
Binding for an unrelated action.

The executable [ExampleAsTools](example_test.go) shows the new data-only intent.
Run `go test -run ExampleAsTools` in this module. Review and clarification have
unchanged callback failure/cancellation behavior and do not issue a grant.


Nil options reject construction. Host ports and callbacks are borrowed; the host
owns their lifetime and synchronization. See the [shared constructor and ownership
contract](../README.md#constructor-configuration-and-ownership) for option snapshots
and the distinction between configuration containers and mutable host ports.
