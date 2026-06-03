package toolsy

import (
	"strings"
)

// Content type constants for [ContentPart.Type].
const (
	ContentTypeText     = "text"
	ContentTypeToolCall = "tool_call"
)

// ContentPart is a provider-agnostic fragment of an LLM response for tool-call extraction.
type ContentPart struct {
	Type       string
	Text       string
	ToolCallID string
	ToolName   string
	Args       string
	ArgsChunk  string
}

// CallParser extracts tool call arguments from LLM content parts.
type CallParser interface {
	ExtractExactlyOne(parts []ContentPart, toolName string) ([]byte, error)
}

// StandardCallParser is the default [CallParser] implementation.
type StandardCallParser struct{}

// ExtractExactlyOne returns merged JSON arguments for exactly one tool_call matching toolName.
// Parts with the same non-empty [ContentPart.ToolCallID] are merged (Args + ArgsChunk).
func (StandardCallParser) ExtractExactlyOne(parts []ContentPart, toolName string) ([]byte, error) {
	var matches []ContentPart
	for _, p := range parts {
		if p.Type != ContentTypeToolCall || p.ToolName != toolName {
			continue
		}
		matches = append(matches, p)
	}
	if len(matches) == 0 {
		return nil, NewSchemaError("no tool call found for tool " + toolName)
	}

	byID := make(map[string]string)
	var noID []ContentPart
	for _, p := range matches {
		if id := strings.TrimSpace(p.ToolCallID); id != "" {
			byID[id] = byID[id] + p.Args + p.ArgsChunk
			continue
		}
		noID = append(noID, p)
	}

	if len(byID) > 0 {
		if len(noID) > 0 || len(byID) > 1 {
			return nil, NewSchemaError("multiple tool calls found for tool " + toolName)
		}
		for _, args := range byID {
			return validatedToolCallArgs(toolName, args)
		}
	}

	switch len(noID) {
	case 1:
		args := strings.TrimSpace(noID[0].Args + noID[0].ArgsChunk)
		return validatedToolCallArgs(toolName, args)
	default:
		return nil, NewSchemaError("multiple tool calls found for tool " + toolName)
	}
}

func validatedToolCallArgs(toolName, args string) ([]byte, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil, NewSchemaError("tool call arguments are empty for tool " + toolName)
	}
	return []byte(args), nil
}
