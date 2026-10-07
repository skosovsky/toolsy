// Package integration demonstrates concrete consumer adapters in an optional module.
package integration

import (
	"encoding/json"
	"errors"

	"github.com/skosovsky/guardy"
	"github.com/skosovsky/prompty"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/examples/host_dispatch/recipe"
)

// Prompt advertises the exact source schema from the execution view.
func Prompt(manifests toolsy.ManifestSet) (*prompty.PromptExecution, error) {
	exec := prompty.SimplePrompt("Use the permitted tools.")
	scope := prompty.ToolScope{}
	for _, name := range manifests.Names() {
		m, _ := manifests.Manifest(name)
		raw, err := json.Marshal(m.Parameters)
		if err != nil {
			return nil, err
		}
		def := prompty.ToolDefinition{Name: m.Name, Description: m.Description, Parameters: prompty.JSONDocument(raw)}
		if dialect, ok := m.Parameters["$schema"].(string); ok {
			def.Dialect = dialect
		}
		exec.Tools = append(exec.Tools, def)
		scope.Allowed = append(scope.Allowed, prompty.ToolManifestFromDefinition(def))
	}
	return exec.WithToolScope(scope)
}

// Requests consumes native call parts, using host-issued identities and authority.
// Fragmented calls require upstream finalization; text parts do not dispatch tools.
func Requests[S, C any, A comparable](
	response *prompty.Response,
	host func(prompty.ToolCallPart) (recipe.Request[S, C, A], error),
) ([]recipe.Request[S, C, A], error) {
	if response == nil || host == nil {
		return nil, errors.New("integration: response and trusted host mapper required")
	}
	var requests []recipe.Request[S, C, A]
	for _, part := range response.Content {
		call, ok := part.(prompty.ToolCallPart)
		if !ok {
			continue
		}
		if call.ArgsChunk != "" {
			return nil, errors.New("integration: fragmented calls must be finalized")
		}
		r, err := host(call)
		if err != nil {
			return nil, err
		}
		r.CallID, r.Tool, r.Args = call.ID, call.Name, json.RawMessage(call.Args)
		requests = append(requests, r)
	}
	return requests, nil
}

// ModelPart only accepts the recipe's audience-filtered projection.
func ModelPart(result *recipe.ModelResult) (prompty.ToolResultPart, error) {
	if result == nil {
		return prompty.ToolResultPart{}, errors.New("integration: no model delivery authorized")
	}
	if !json.Valid(result.Value) {
		return prompty.ToolResultPart{}, errors.New("integration: invalid projected JSON")
	}
	return prompty.ToolResultPart{
		ToolCallID: result.CallID,
		Name:       result.Tool,
		IsError:    result.IsError,
		Content:    []prompty.ContentPart{prompty.TextPart{Text: string(result.Value)}},
	}, nil
}

// GuardDecision preserves fault/deny/correction independently of business outcomes.
// No mapping grants authority, invokes a tool or schedules a retry.
func GuardDecision(decision guardy.Decision) string {
	switch {
	case decision.IsSystemFault():
		return "fault"
	case decision.IsTerminal():
		return "deny"
	case decision.IsRetryable():
		return "correction"
	default:
		return "continue"
	}
}
