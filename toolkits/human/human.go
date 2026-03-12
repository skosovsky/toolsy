package human

import (
	"context"
	"fmt"

	"github.com/skosovsky/toolsy"
)

// EscalationHandler is implemented by the orchestrator developer
// to bridge agent questions to any UI (console, Telegram, React, etc.).
//
// Both methods block the agent goroutine until the human responds.
// Implementations MUST listen to ctx.Done() and return ctx.Err()
// if the context is cancelled (e.g. session closed, timeout expired).
// The orchestrator should use context.WithTimeout to bound wait time.
type EscalationHandler interface {
	ApproveAction(ctx context.Context, action, reason string) (bool, error)
	ProvideClarification(ctx context.Context, question string) (string, error)
}

// AsTools returns two tools (request_approval, ask_human_clarification) that
// delegate to the given EscalationHandler. Options customize names and descriptions.
func AsTools(handler EscalationHandler, opts ...Option) ([]toolsy.Tool, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)

	approvalTool, err := toolsy.NewTool[approvalArgs, approvalResult](
		o.approvalName,
		o.approvalDesc,
		func(ctx context.Context, args approvalArgs) (approvalResult, error) {
			ok, err := handler.ApproveAction(ctx, args.Action, args.Reason)
			if err != nil {
				return approvalResult{}, fmt.Errorf("toolkit/human: approve action: %w", err)
			}
			if ok {
				return approvalResult{Decision: "APPROVED"}, nil
			}
			return approvalResult{Decision: "REJECTED"}, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/human: build approval tool: %w", err)
	}

	clarificationTool, err := toolsy.NewTool[clarificationArgs, clarificationResult](
		o.clarificationName,
		o.clarificationDesc,
		func(ctx context.Context, args clarificationArgs) (clarificationResult, error) {
			answer, err := handler.ProvideClarification(ctx, args.Question)
			if err != nil {
				return clarificationResult{}, fmt.Errorf("toolkit/human: provide clarification: %w", err)
			}
			return clarificationResult{Answer: answer}, nil
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

type approvalResult struct {
	Decision string `json:"decision"`
}

type clarificationArgs struct {
	Question string `json:"question"`
}

type clarificationResult struct {
	Answer string `json:"answer"`
}
