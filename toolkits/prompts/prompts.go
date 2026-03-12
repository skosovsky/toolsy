package prompts

import (
	"context"
	"fmt"
	"strings"

	"github.com/skosovsky/toolsy"
)

// Provider is the interface the toolkit expects. Implement it with any backend
// (prompty, Git, file store); the toolkit only needs roleID + variables -> rendered text.
type Provider interface {
	Get(ctx context.Context, roleID string, variables map[string]any) (string, error)
}

type getArgs struct {
	RoleID    string         `json:"role_id"`
	Variables map[string]any `json:"variables,omitempty"`
}

type getResult struct {
	Instructions string `json:"instructions"`
}

// AsTool builds a single toolsy.Tool that calls p.Get with the parsed role_id and variables,
// and returns the rendered instructions.
func AsTool(p Provider, opts ...Option) (toolsy.Tool, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	o.applyDefaults()

	handler := func(ctx context.Context, args getArgs) (getResult, error) {
		text, err := p.Get(ctx, args.RoleID, args.Variables)
		if err != nil {
			return getResult{}, fmt.Errorf("toolkit/prompts: get failed: %w", err)
		}
		if o.maxBytes > 0 && len(text) > o.maxBytes {
			trunc := text[:o.maxBytes]
			trunc = strings.ToValidUTF8(trunc, "")
			text = trunc + "\n[Truncated]"
		}
		return getResult{Instructions: text}, nil
	}

	return toolsy.NewTool(o.name, o.description, handler)
}
