package human

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/skosovsky/toolsy"
)

const payloadKindKey = "kind"

type approvalArgs struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

type clarificationArgs struct {
	Question string `json:"question"`
}

// AsTools returns two suspend-first tools (request_approval, ask_human_clarification).
// The orchestrator is expected to checkpoint execution when a control pause error is returned.
func AsTools(opts ...Option) ([]toolsy.Tool, error) {
	o := options{
		approvalName: "", approvalDesc: "", clarificationName: "", clarificationDesc: "",
		maxPayloadBytes: defaultMaxPayloadBytes,
	}
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.New("toolkit/human: nil option")
		}
		opt(&o)
	}
	applyDefaults(&o)
	if o.maxPayloadBytes <= 0 {
		return nil, errors.New("toolkit/human: payload limit must be positive")
	}

	approvalTool, err := toolsy.NewStreamTool[approvalArgs](
		o.approvalName,
		o.approvalDesc,
		func(_ context.Context, _ *toolsy.RunEnv, args approvalArgs, yield func(toolsy.Chunk) error) error {
			payload, marshalErr := encodePausePayload(map[string]string{
				payloadKindKey: "approval",
				"action":       args.Action,
				"reason":       args.Reason,
			}, o.maxPayloadBytes)
			if marshalErr != nil {
				return marshalErr
			}
			return toolsy.YieldControl(yield, &toolsy.PauseSignal{
				Reason: string(payload),
			})
		},
		toolsy.WithCompletionPolicy(toolsy.CompletionSilentYield),
		toolsy.WithIndependentStream(),
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/human: build approval tool: %w", err)
	}

	clarificationTool, err := toolsy.NewStreamTool[clarificationArgs](
		o.clarificationName,
		o.clarificationDesc,
		func(_ context.Context, _ *toolsy.RunEnv, args clarificationArgs, yield func(toolsy.Chunk) error) error {
			payload, marshalErr := encodePausePayload(map[string]string{
				payloadKindKey: "clarification",
				"question":     args.Question,
			}, o.maxPayloadBytes)
			if marshalErr != nil {
				return marshalErr
			}
			return toolsy.YieldControl(yield, &toolsy.PauseSignal{
				Reason: string(payload),
			})
		},
		toolsy.WithCompletionPolicy(toolsy.CompletionSilentYield),
		toolsy.WithIndependentStream(),
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/human: build clarification tool: %w", err)
	}

	return []toolsy.Tool{approvalTool, clarificationTool}, nil
}

func encodePausePayload(fields map[string]string, limit int) ([]byte, error) {
	remaining := limit
	for _, value := range fields {
		if len(value) > remaining {
			return nil, toolsy.NewValidationError(fmt.Sprintf("human pause payload exceeds %d bytes", limit))
		}
		remaining -= len(value)
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return nil, toolsy.NewInternalError(fmt.Errorf("human pause payload: %w", err))
	}
	if len(payload) > limit {
		return nil, toolsy.NewValidationError(fmt.Sprintf("human pause payload exceeds %d bytes", limit))
	}
	return payload, nil
}
