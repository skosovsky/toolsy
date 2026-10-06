package human

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/skosovsky/toolsy"
)

const payloadKindKey = "kind"

type reviewArgs struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

type clarificationArgs struct {
	Question string `json:"question"`
}

// AsTools returns data-only request_human_review and ask_human_clarification tools.
// Neither authenticates a reviewer, binds an operation or issues an ApprovalGrant.
// Hosts own continuation, UI routing and authenticated action authorization.
func AsTools(opts ...Option) ([]toolsy.Tool, error) {
	o := options{
		reviewName: "", reviewDesc: "", clarificationName: "", clarificationDesc: "",
		maxPayloadBytes: defaultMaxPayloadBytes,
	}
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.New("toolkit/human: nil option")
		}
		opt(&o)
	}
	applyDefaults(&o)
	if o.maxPayloadBytes <= 0 || o.maxPayloadBytes > toolsy.MaxControlBytes {
		return nil, fmt.Errorf("toolkit/human: payload limit must be between 1 and %d bytes", toolsy.MaxControlBytes)
	}

	reviewTool, err := toolsy.NewStreamTool[reviewArgs](
		o.reviewName,
		o.reviewDesc,
		func(_ context.Context, _ *toolsy.RunEnv, args reviewArgs, yield func(toolsy.Chunk) error) error {
			payload, marshalErr := encodePausePayload(map[string]string{
				payloadKindKey: "human_review",
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
		return nil, fmt.Errorf("toolkit/human: build review tool: %w", err)
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

	return []toolsy.Tool{reviewTool, clarificationTool}, nil
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
