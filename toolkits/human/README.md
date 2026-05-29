# human toolkit

**Description:** Lets the agent request human approval for dangerous actions and ask for clarification. Each tool yields a typed control pause signal and returns `toolsy.ErrPause`.

## Tools

- `request_approval` — human approval for a dangerous action
- `ask_human_clarification` — clarification question for the user

## Control flow

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
