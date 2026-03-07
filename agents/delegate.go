package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skosovsky/toolsy"
)

// formatStepOutput builds a single Markdown string from step text and artifacts.
// Artifacts with Data (base64) are rendered as ![FileName](data:MimeType;base64,Data) for multimodal models.
func formatStepOutput(text string, artifacts []Artifact) string {
	var b strings.Builder
	if text != "" {
		b.WriteString(text)
	}
	for _, a := range artifacts {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		mime := a.MimeType
		if mime == "" {
			mime = "application/octet-stream"
		}
		fileName := a.FileName
		if fileName == "" {
			fileName = "file"
		}
		if a.Data != "" {
			_, _ = fmt.Fprintf(&b, "![%s](data:%s;base64,%s)", fileName, mime, a.Data)
		} else {
			b.WriteString(fileName)
		}
	}
	return b.String()
}

// AsTool creates a toolsy.Tool that delegates to the Agent Protocol: CreateTask, stream steps, yield progress and final result.
// inputSchema is the JSON Schema the orchestrator must satisfy; args are sent as task input.
func AsTool(name, description string, inputSchema []byte, client *Client) (toolsy.Tool, error) {
	if client == nil {
		return nil, fmt.Errorf("agents: client is nil")
	}
	if len(inputSchema) == 0 {
		return nil, fmt.Errorf("agents: inputSchema must not be empty")
	}
	return toolsy.NewProxyTool(name, description, inputSchema,
		func(ctx context.Context, args []byte, yield func(toolsy.Chunk) error) error {
			task, err := client.CreateTask(ctx, args)
			if err != nil {
				return fmt.Errorf("agents: create task: %w", err)
			}
			defer func() {
				if ctx.Err() != nil {
					_ = client.CancelTask(context.Background(), task.TaskID)
				}
			}()
			for step, streamErr := range client.StreamSteps(ctx, task.TaskID) {
				if streamErr != nil {
					return fmt.Errorf("agents: stream error: %w", streamErr)
				}
				if step.IsLast {
					finalData := formatStepOutput(step.Output, step.Artifacts)
					return yield(toolsy.Chunk{
						Event: toolsy.EventResult,
						Data:  []byte(finalData),
					})
				}
				if yieldErr := yield(toolsy.Chunk{
					Event:    toolsy.EventProgress,
					Metadata: map[string]any{"sub_agent_step": step.Name, "status": step.Status},
				}); yieldErr != nil {
					return yieldErr
				}
			}
			// If the stream ends without a step with IsLast, we exit without a final chunk
			// (server-dependent behavior; orchestrator gets no final result in that case).
			return nil
		},
	)
}

// AsBackgroundTool creates a toolsy.Tool that starts a task and returns the task_id immediately without waiting for completion.
func AsBackgroundTool(name, desc string, schema []byte, client *Client) (toolsy.Tool, error) {
	if client == nil {
		return nil, fmt.Errorf("agents: client is nil")
	}
	if len(schema) == 0 {
		return nil, fmt.Errorf("agents: schema must not be empty")
	}
	return toolsy.NewProxyTool(name, desc, schema,
		func(ctx context.Context, args []byte, yield func(toolsy.Chunk) error) error {
			task, err := client.CreateTask(ctx, args)
			if err != nil {
				return fmt.Errorf("agents: create task: %w", err)
			}
			out, _ := json.Marshal(map[string]string{"task_id": task.TaskID})
			return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: out})
		},
	)
}
