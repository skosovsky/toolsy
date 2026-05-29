package mcp

import "github.com/skosovsky/toolsy"

// ToolAnnotations carries MCP tool hints mapped into toolsy manifest policy fields.
// OpenWorldHint is parsed for forward compatibility but intentionally not mapped to manifest fields.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

func mcpToolPolicyOptions(annotations *ToolAnnotations) []toolsy.ToolOption {
	if annotations == nil {
		return nil
	}
	var opts []toolsy.ToolOption
	if annotations.ReadOnlyHint != nil && *annotations.ReadOnlyHint {
		opts = append(opts, toolsy.WithReadOnly())
	}
	if annotations.DestructiveHint != nil && *annotations.DestructiveHint {
		opts = append(opts, toolsy.WithDangerous())
	}
	if annotations.IdempotentHint != nil && *annotations.IdempotentHint {
		opts = append(opts, toolsy.WithIdempotent())
	}
	return opts
}
