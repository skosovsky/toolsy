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
	var opts []toolsy.ToolOption
	// MCP defaults destructiveHint to true. Preserve fail-closed host policy unless
	// the server explicitly opts out or declares the tool read-only.
	readOnly := annotations != nil && annotations.ReadOnlyHint != nil && *annotations.ReadOnlyHint
	explicitlyDestructive := annotations != nil && annotations.DestructiveHint != nil &&
		*annotations.DestructiveHint
	destructive := explicitlyDestructive ||
		(!readOnly && (annotations == nil || annotations.DestructiveHint == nil))
	if destructive {
		opts = append(opts, toolsy.WithDangerous())
	}
	if annotations == nil {
		return opts
	}
	if annotations.ReadOnlyHint != nil && *annotations.ReadOnlyHint {
		opts = append(opts, toolsy.WithReadOnly())
	}
	if annotations.IdempotentHint != nil && *annotations.IdempotentHint {
		opts = append(opts, toolsy.WithIdempotent())
	}
	return opts
}
