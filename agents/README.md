# agents

Agent Protocol bridge for [toolsy](https://github.com/skosovsky/toolsy). This module turns a remote sub-agent (exposing the [Agent Protocol](https://agentprotocol.ai) REST API and SSE) into a `toolsy.Tool`, so the orchestrator can delegate work without knowing the protocol.

**Principle: Agent-as-a-Tool.** The toolsy core does not depend on agents or the Agent Protocol. This package is a facade: it speaks REST/SSE with the remote agent and implements the toolsy `Tool` interface via `toolsy.NewProxyTool`.

## Features

- **REST client:** `CreateTask`, `CancelTask` with custom HTTP client and runtime auth supplied through `toolsy.RunContext.Credentials`.
- **SSE streaming:** `StreamSteps` consumes `GET /ap/v1/agent/tasks/{id}/steps?stream=true`, parses steps, and supports **Last-Event-ID** auto-reconnect with a 1s backoff on disconnect.
- **Delegation:** `AsTool` (sync delegation with progress streaming) and `AsBackgroundTool` (fire-and-forget, returns `task_id` for status checks).

### Stream completion

If the sub-agent server closes the SSE stream without sending a step with `is_last: true`, the tool finishes without a final result chunk (`EventResult`). That behavior indicates a contract violation on the sub-agent server side (Agent Protocol requires the last step to be marked); it is not a bug in toolsy. When the orchestrator sees a tool that simply ends with no result, the issue is with the remote server, not with this client.

## Example

```go
package main

import (
	"context"
	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/agents"
)

type staticCredentials struct{}

func (staticCredentials) GetAuth(context.Context, string) (string, error) {
	return "Bearer your-token", nil
}

func main() {
	reg := toolsy.NewRegistry()

	client := agents.NewClient("https://api.example.com/agent")

	schema := []byte(`{
		"type": "object",
		"properties": {
			"repository": {"type": "string", "description": "Repository URL"},
			"bug_description": {"type": "string"}
		},
		"required": ["repository", "bug_description"]
	}`)

	tool, _ := agents.AsTool(
		"delegate_to_coder",
		"Delegates to the coder agent to fix bugs.",
		schema,
		client,
	)
	reg.Register(tool)

	_ = reg.Execute(context.Background(), toolsy.ToolCall{
		ID:       "1",
		ToolName: "delegate_to_coder",
		Args:     []byte(`{"repository":"https://example.com/repo","bug_description":"fix the failing test"}`),
		Run:      toolsy.RunContext{Credentials: staticCredentials{}},
	}, func(toolsy.Chunk) error { return nil })
}
```

## AsBackgroundTool

`AsBackgroundTool` creates a tool that starts a task and returns immediately with `task_id` (as JSON `{"task_id":"..."}`). The orchestrator can use another tool or API to poll task status by `task_id`.

When a `CredentialsProvider` is present, the bridge resolves auth separately for `agents.create_task`, `agents.stream_steps`, and `agents.cancel_task`.

## Requirements

- Go 1.26+
- Agent Protocol server at `baseURL` (paths: `/ap/v1/agent/tasks`, `/ap/v1/agent/tasks/{id}/cancel`, `/ap/v1/agent/tasks/{id}/steps?stream=true`).
