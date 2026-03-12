# Toolsy: Human Toolkit (HITL escalation)

**Description:** Lets the agent request human approval for dangerous actions and ask for clarification. Both tools block the agent goroutine until the human responds via your `EscalationHandler` implementation.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/human
```

**Dependencies:** stdlib only; requires `github.com/skosovsky/toolsy` (core).

## Available tools

| Tool                     | Description                          | Input                                              |
|--------------------------|--------------------------------------|----------------------------------------------------|
| `request_approval`       | Request human approval for an action | `{"action": "string", "reason": "string"}`         |
| `ask_human_clarification`| Ask a human for clarification        | `{"question": "string"}`                           |

## Configuration and security

- **Tool names and descriptions:** Use `WithApprovalName`, `WithApprovalDescription`, `WithClarificationName`, `WithClarificationDescription` to customize.
- **Blocking behaviour:** Both tools block the calling goroutine until the handler returns. Implementations of `EscalationHandler` MUST listen to `ctx.Done()` and return `ctx.Err()` when the context is cancelled (e.g. session closed, timeout). The orchestrator should use `context.WithTimeout` to bound wait time.

## Quick start

```go
package main

import (
	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/human"
)

func main() {
	reg := toolsy.NewRegistry()

	handler := &myHandler{} // implements human.EscalationHandler
	tools, err := human.AsTools(handler)
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		reg.Register(tool)
	}

	// When executing agent tools, use context.WithTimeout so approval/clarification don't block forever.
}
```

## Example: implementing EscalationHandler

Implement `ApproveAction` and `ProvideClarification` to bridge to your UI (console, Telegram, React, etc.). Always respect context cancellation:

```go
type myHandler struct{}

func (h *myHandler) ApproveAction(ctx context.Context, action, reason string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}
	// Show "action" and "reason" to user, wait for Yes/No, return true/false
	return userSaidYes(), nil
}

func (h *myHandler) ProvideClarification(ctx context.Context, question string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	// Show "question" to user, wait for reply, return answer
	return userReply(), nil
}
```
