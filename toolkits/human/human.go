package human

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skosovsky/toolsy"
)

// AsTools returns two suspend-first tools (request_approval, ask_human_clarification).
// The orchestrator is expected to checkpoint execution when toolsy.ErrSuspend is returned.
func AsTools(opts ...Option) ([]toolsy.Tool, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)

	approvalTool, err := toolsy.NewStreamTool[approvalArgs](
		o.approvalName,
		o.approvalDesc,
		func(_ context.Context, _ toolsy.RunContext, args approvalArgs, yield func(toolsy.Chunk) error) error {
			payload, marshalErr := json.Marshal(map[string]string{
				"kind":   "approval",
				"action": args.Action,
				"reason": args.Reason,
			})
			if marshalErr != nil {
				return fmt.Errorf("toolkit/human: marshal approval payload: %w", marshalErr)
			}
			if yieldErr := yield(toolsy.Chunk{
				Event:    toolsy.EventSuspend,
				Data:     payload,
				MimeType: toolsy.MimeTypeJSON,
			}); yieldErr != nil {
				return yieldErr
			}
			return toolsy.ErrSuspend
		},
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/human: build approval tool: %w", err)
	}

	clarificationTool, err := toolsy.NewStreamTool[clarificationArgs](
		o.clarificationName,
		o.clarificationDesc,
		func(_ context.Context, _ toolsy.RunContext, args clarificationArgs, yield func(toolsy.Chunk) error) error {
			payload, marshalErr := json.Marshal(map[string]string{
				"kind":     "clarification",
				"question": args.Question,
			})
			if marshalErr != nil {
				return fmt.Errorf("toolkit/human: marshal clarification payload: %w", marshalErr)
			}
			if yieldErr := yield(toolsy.Chunk{
				Event:    toolsy.EventSuspend,
				Data:     payload,
				MimeType: toolsy.MimeTypeJSON,
			}); yieldErr != nil {
				return yieldErr
			}
			return toolsy.ErrSuspend
		},
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/human: build clarification tool: %w", err)
	}

	return []toolsy.Tool{approvalTool, clarificationTool}, nil
}

type approvalArgs struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

type clarificationArgs struct {
	Question string `json:"question"`
}
