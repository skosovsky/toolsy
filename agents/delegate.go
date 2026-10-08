package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

const (
	createTaskAuthToolName    = "agents.create_task"
	cancelTaskAuthToolName    = "agents.cancel_task"
	streamStepsAuthToolName   = "agents.stream_steps"
	defaultMaxStepOutputBytes = 256 * 1024
)

// cancelTaskTimeout bounds CancelTask after the stream context is canceled.
const cancelTaskTimeout = 5 * time.Second

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
	out := b.String()
	if defaultMaxStepOutputBytes > 0 && len(out) > defaultMaxStepOutputBytes {
		out = textprocessor.TruncateStringUTF8(out, defaultMaxStepOutputBytes, textprocessor.TruncationSuffix)
	}
	return out
}

func resolveAuthHeader(ctx context.Context, run *toolsy.RunEnv, toolName string) (string, error) {
	if run.Credentials == nil {
		return "", nil
	}
	return run.Credentials.GetAuth(ctx, toolName)
}

// AsTool adapts remote task creation and the toolsy-step-stream-v1 observation extension.
// inputSchema is the JSON Schema the host must satisfy; args are sent as task input.
//
//nolint:gocognit
func AsTool(name, description string, inputSchema []byte, client *Client) (toolsy.Tool, error) {
	if client == nil {
		return nil, errors.New("agents: client is nil")
	}
	if len(inputSchema) == 0 {
		return nil, errors.New("agents: inputSchema must not be empty")
	}
	return toolsy.NewProxyTool(name, description, inputSchema,
		func(ctx context.Context, run *toolsy.RunEnv, args []byte, yield func(toolsy.Chunk) error) error {
			createAuth, authErr := resolveAuthHeader(ctx, run, createTaskAuthToolName)
			if authErr != nil {
				return fmt.Errorf("agents: get create task auth: %w", authErr)
			}
			task, err := client.CreateTask(ctx, args, createAuth)
			if err != nil {
				return fmt.Errorf("agents: create task: %w", err)
			}
			defer client.cancelInterruptedTask(ctx, run, task.TaskID)
			streamAuth, authErr := resolveAuthHeader(ctx, run, streamStepsAuthToolName)
			if authErr != nil {
				return interruptionOutcome(
					&RemoteOutcomeError{
						Phase:   PhaseObserve,
						Outcome: OutcomeUnknown,
						TaskID:  task.TaskID,
						Step:    nil,
						Cause:   authErr,
					},
				)
			}
			var lastStep *Step
			for step, streamErr := range client.StreamSteps(ctx, task.TaskID, streamAuth) {
				if streamErr != nil {
					return mapStreamOutcome(ctx, task.TaskID, lastStep, streamErr, client.maxSSEStreamBytes())
				}
				lastStep = &step
				if step.IsLast {
					switch step.Status {
					case "failed":
						return &RemoteOutcomeError{
							Phase:   PhaseObserve,
							Outcome: OutcomeFailed,
							TaskID:  task.TaskID,
							Step:    &step,
							Cause:   nil,
						}
					case "cancelled":
						return &RemoteOutcomeError{Phase: PhaseObserve,
							Outcome: OutcomeCancelled,
							TaskID:  task.TaskID,
							Step:    &step,
							Cause:   nil,
						}
					}
					finalData := formatStepOutput(step.Output, step.Artifacts)
					var chunk toolsy.Chunk
					chunk.Event = toolsy.EventResult
					chunk.Data = []byte(finalData)
					chunk.MimeType = toolsy.MimeTypeText
					return yield(chunk)
				}
				var chunk toolsy.Chunk
				chunk.Event = toolsy.EventProgress
				chunk.Progress = &toolsy.ProgressInfo{ //nolint:exhaustruct_v5 // label/status only for sub-agent steps
					Label:  step.Name,
					Status: step.Status,
				}
				if yieldErr := yield(chunk); yieldErr != nil {
					return yieldErr
				}
			}
			if err := ctx.Err(); err != nil {
				return interruptionOutcome(
					&RemoteOutcomeError{
						Phase:   PhaseObserve,
						Outcome: OutcomeUnknown,
						TaskID:  task.TaskID,
						Step:    lastStep,
						Cause:   err,
					},
				)
			}
			return &RemoteOutcomeError{
				Phase:   PhaseObserve,
				Outcome: OutcomeIncomplete,
				TaskID:  task.TaskID,
				Step:    lastStep,
				Cause:   nil,
			}
		},
	)
}

// AsBackgroundTool returns an AcceptedTaskReference after creation acknowledgement, without claiming completion.
func AsBackgroundTool(name, desc string, schema []byte, client *Client) (toolsy.Tool, error) {
	if client == nil {
		return nil, errors.New("agents: client is nil")
	}
	if len(schema) == 0 {
		return nil, errors.New("agents: schema must not be empty")
	}
	return toolsy.NewProxyTool(name, desc, schema,
		func(ctx context.Context, run *toolsy.RunEnv, args []byte, yield func(toolsy.Chunk) error) error {
			createAuth, authErr := resolveAuthHeader(ctx, run, createTaskAuthToolName)
			if authErr != nil {
				return fmt.Errorf("agents: get create task auth: %w", authErr)
			}
			task, err := client.CreateTask(ctx, args, createAuth)
			if err != nil {
				return fmt.Errorf("agents: create task: %w", err)
			}
			out, _ := json.Marshal(AcceptedTaskReference{TaskID: task.TaskID, Accepted: true})
			var chunk toolsy.Chunk
			chunk.Event = toolsy.EventResult
			chunk.Data = out
			chunk.MimeType = toolsy.MimeTypeJSON
			return yield(chunk)
		},
		toolsy.WithOutputSchema(map[string]any{schemaTypeKey: "object", "properties": map[string]any{
			taskIDKey:  map[string]any{schemaTypeKey: "string", "minLength": 1},
			"accepted": map[string]any{schemaTypeKey: "boolean", "const": true},
		}, "required": []string{taskIDKey, "accepted"}, "additionalProperties": false}),
	)
}

func (c *Client) cancelInterruptedTask(ctx context.Context, run *toolsy.RunEnv, taskID string) {
	if ctx.Err() == nil {
		return
	}
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cancelTaskTimeout)
	defer cancel()
	diagnostic := CancellationDiagnostic{
		TaskID: taskID, ParentInterrupt: ctx.Err(), ParentCause: context.Cause(ctx),
		Stage: CancellationCredentials, Acknowledged: false, Cause: nil,
	}
	auth, err := resolveAuthHeader(cancelCtx, run, cancelTaskAuthToolName)
	if err == nil {
		diagnostic.Stage = CancellationRequest
		err = c.CancelTask(cancelCtx, taskID, auth)
		diagnostic.Acknowledged = err == nil
	}
	diagnostic.Cause = err
	if observer := c.opts.cancellationObserver; observer != nil {
		observer(cancelCtx, diagnostic)
	}
}

func mapStreamOutcome(ctx context.Context, taskID string, lastStep *Step, err error, maxBytes int) error {
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if remote, ok := errors.AsType[*RemoteOutcomeError](err); ok && remote.Step == nil {
		remote.Step = lastStep
	}
	if toolsy.IsContextInterrupt(err) {
		return interruptionOutcome(
			&RemoteOutcomeError{
				Phase:   PhaseObserve,
				Outcome: OutcomeUnknown,
				TaskID:  taskID,
				Step:    lastStep,
				Cause:   err,
			},
		)
	}
	if textprocessor.IsReadLimitExceeded(err) {
		mapped := toolsy.MapReadLimitErrorFor(err, maxBytes, "SSE step stream", "")
		return errors.Join(mapped, err)
	}
	return fmt.Errorf("agents: stream error: %w", err)
}
