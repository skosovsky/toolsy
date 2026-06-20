package toolsy

import (
	"encoding/json"
	"strconv"
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
	parts, err := NormalizeToolCallParts(parts)
	if err != nil {
		return nil, err
	}
	if err := rejectMultipleToolCalls(parts); err != nil {
		return nil, err
	}
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

// ParseExactlyOne extracts and decodes exactly one tool call for toolName.
func ParseExactlyOne[T any](parts []ContentPart, toolName string) (*T, error) {
	raw, err := StandardCallParser{}.ExtractExactlyOne(parts, toolName)
	if err != nil {
		return nil, err
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, NewJSONParseError(err)
	}
	return &out, nil
}

// NormalizeToolCallParts fills continuation chunks with a known ToolName from the same ToolCallID.
// Conflicting tool names for the same call id are rejected.
func NormalizeToolCallParts(parts []ContentPart) ([]ContentPart, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	namesByID := make(map[string]string)
	for _, p := range parts {
		if p.Type != ContentTypeToolCall {
			continue
		}
		id := strings.TrimSpace(p.ToolCallID)
		name := strings.TrimSpace(p.ToolName)
		if id == "" || name == "" {
			continue
		}
		if existing, exists := namesByID[id]; exists && existing != name {
			return nil, NewSchemaError("conflicting tool names for tool call " + id)
		}
		if _, exists := namesByID[id]; !exists {
			namesByID[id] = name
		}
	}
	if err := rejectDuplicateCompleteToolCalls(parts); err != nil {
		return nil, err
	}
	out := make([]ContentPart, len(parts))
	copy(out, parts)
	for i := range out {
		if out[i].Type != ContentTypeToolCall || strings.TrimSpace(out[i].ToolName) != "" {
			continue
		}
		if name := namesByID[strings.TrimSpace(out[i].ToolCallID)]; name != "" {
			out[i].ToolName = name
		}
	}
	return out, nil
}

func rejectDuplicateCompleteToolCalls(parts []ContentPart) error {
	type callState struct {
		args     string
		complete bool
	}
	stateByID := make(map[string]callState)
	for _, p := range parts {
		if p.Type != ContentTypeToolCall {
			continue
		}
		id := strings.TrimSpace(p.ToolCallID)
		if id == "" {
			continue
		}
		args := strings.TrimSpace(p.Args + p.ArgsChunk)
		if args == "" {
			continue
		}
		state := stateByID[id]
		if state.complete {
			return NewSchemaError("duplicate complete tool call " + id)
		}
		state.args += args
		if json.Valid([]byte(strings.TrimSpace(state.args))) {
			state.complete = true
		}
		stateByID[id] = state
	}
	return nil
}

func rejectMultipleToolCalls(parts []ContentPart) error {
	groups := make(map[string]struct{})
	noID := 0
	for i, p := range parts {
		if p.Type != ContentTypeToolCall {
			continue
		}
		id := strings.TrimSpace(p.ToolCallID)
		if id == "" {
			noID++
			groups["no-id-"+strconv.Itoa(i)] = struct{}{}
			continue
		}
		groups[id] = struct{}{}
	}
	if len(groups) > 1 || noID > 1 {
		return NewSchemaError("multiple tool calls found")
	}
	return nil
}

func validatedToolCallArgs(toolName, args string) ([]byte, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil, NewSchemaError("tool call arguments are empty for tool " + toolName)
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return nil, NewJSONParseError(err)
	}
	return []byte(args), nil
}
