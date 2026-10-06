package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

const mcpHeaderAnnotation = "x-mcp-header"

// ToolAnnotations carries untrusted source hints. ListTools preserves these
// values for host display and diagnostics; they never grant execution authority.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
	Extra           Meta   `json:"-"`
}

// ToolExecutionProperties is the host's explicit classification of a remote
// tool. It is not an authorization grant or cache freshness decision. ResultCache
// additionally requires per-attempt host eligibility and a trusted partition.
type ToolExecutionProperties struct {
	ReadOnly             bool
	Dangerous            bool
	Idempotent           bool
	RequiresConfirmation bool
}

// ToolPolicyMapper runs during Discover with the host's discovery context and
// an owned descriptor snapshot. Capture host-owned authority or use context
// values supplied by the host; remote annotations and metadata are untrusted.
// Invocation authorization remains the current Registry/typed policy's job.
type ToolPolicyMapper func(context.Context, MCPTool) (ToolExecutionProperties, error)

func WithToolPolicyMapper(mapper ToolPolicyMapper) ClientOption {
	return func(options *ClientOptions) { options.ToolPolicyMapper = mapper }
}

func (c *Client) toolPolicyOptions(ctx context.Context, descriptor MCPTool) ([]toolsy.ToolOption, error) {
	properties := ToolExecutionProperties{
		ReadOnly:             false,
		Dangerous:            true,
		Idempotent:           false,
		RequiresConfirmation: false,
	}
	if c.opts.ToolPolicyMapper != nil {
		snapshot, err := cloneToolDescriptor(descriptor)
		if err != nil {
			return nil, &InvalidPayloadError{Subject: "tool policy descriptor", Err: err}
		}
		properties, err = c.opts.ToolPolicyMapper(ctx, snapshot)
		if err != nil {
			return nil, err
		}
	}
	var options []toolsy.ToolOption
	if properties.Dangerous {
		options = append(options, toolsy.WithDangerous())
	}
	if properties.ReadOnly {
		options = append(options, toolsy.WithReadOnly())
	}
	if properties.Idempotent {
		options = append(options, toolsy.WithIdempotent())
	}
	if properties.RequiresConfirmation {
		options = append(options, toolsy.WithRequiresConfirmation())
	}
	return options, nil
}

func cloneToolDescriptor(descriptor MCPTool) (MCPTool, error) {
	raw, err := json.Marshal(descriptor)
	if err != nil {
		return MCPTool{}, err
	}
	var snapshot MCPTool
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return MCPTool{}, err
	}
	return snapshot, nil
}

// generationTool checks adapter authority before the underlying prepared
// boundary and before each delivery, including cached replay. It leaves host
// identity types and the proxy's schema binding unchanged.
type generationTool struct {
	toolsy.Tool

	client     *Client
	generation uint64
}

func (t *generationTool) SupportsPreparedExecution() bool { return true }

func (t *generationTool) Execute(
	ctx context.Context,
	env *toolsy.RunEnv,
	input toolsy.ToolInput,
	yield func(toolsy.Chunk) error,
) error {
	if current := t.client.toolGeneration.Load(); current != t.generation {
		return staleError(InvalidationTools, t.generation, current)
	}
	return t.Tool.Execute(ctx, env, input, func(chunk toolsy.Chunk) error {
		if current := t.client.toolGeneration.Load(); current != t.generation {
			return staleError(InvalidationTools, t.generation, current)
		}
		return yield(chunk)
	})
}

// compileHTTPToolHeaderBindings accepts annotations only on direct
// inputSchema.properties entries. Nested and indirect annotations invalidate
// the descriptor instead of creating ambiguous extraction semantics.
//
//nolint:gocognit // Header annotation validation deliberately evaluates all static-path constraints together.
//nolint:gocognit // Static-schema rejection and top-level binding compilation form one security boundary.
func compileHTTPToolHeaderBindings(raw json.RawMessage) ([]HTTPToolHeaderBinding, error) {
	decoded, err := jsonschemax.Decode(raw)
	if err != nil {
		return nil, err
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return nil, errors.New("input schema must be an object")
	}
	if _, annotated := root[mcpHeaderAnnotation]; annotated {
		return nil, errors.New("x-mcp-header is only valid on a property")
	}
	for keyword, value := range root {
		if keyword != "properties" && containsHTTPHeaderAnnotation(value) {
			return nil, fmt.Errorf("x-mcp-header under %s is not a top-level property", keyword)
		}
	}
	propertiesValue, hasProperties := root["properties"]
	if !hasProperties {
		return nil, nil
	}
	properties, ok := propertiesValue.(map[string]any)
	if !ok {
		return nil, errors.New("schema properties must be an object")
	}
	bindings := make([]HTTPToolHeaderBinding, 0)
	seenHeaders := make(map[string]struct{})
	propertyNames := make([]string, 0, len(properties))
	for propertyName := range properties {
		propertyNames = append(propertyNames, propertyName)
	}
	sort.Strings(propertyNames)
	for _, propertyName := range propertyNames {
		propertyValue := properties[propertyName]
		property, ok := propertyValue.(map[string]any)
		if !ok {
			continue
		}
		for keyword, nested := range property {
			if keyword != mcpHeaderAnnotation && containsHTTPHeaderAnnotation(nested) {
				return nil, fmt.Errorf("property %q contains nested x-mcp-header", propertyName)
			}
		}
		if suffixValue, annotated := property[mcpHeaderAnnotation]; annotated {
			suffix, ok := suffixValue.(string)
			if !ok || !validHTTPHeaderSuffix(suffix) {
				return nil, fmt.Errorf("property %q has invalid x-mcp-header suffix", propertyName)
			}
			for _, keyword := range []string{
				schemaRefKeyword, "items", "allOf", "anyOf", "oneOf", "not", "if", "then", "else",
			} {
				if _, indirect := property[keyword]; indirect {
					return nil, fmt.Errorf("property %q uses x-mcp-header with %s", propertyName, keyword)
				}
			}
			valueType, _ := property["type"].(string)
			switch valueType {
			case schemaTypeString, schemaTypeInteger, schemaTypeBoolean:
			default:
				return nil, fmt.Errorf(
					"property %q has unsupported x-mcp-header type %q",
					propertyName,
					valueType,
				)
			}
			canonical := strings.ToLower(suffix)
			if _, duplicate := seenHeaders[canonical]; duplicate {
				return nil, fmt.Errorf("duplicate x-mcp-header suffix %q", suffix)
			}
			seenHeaders[canonical] = struct{}{}
			bindings = append(bindings, HTTPToolHeaderBinding{
				Header: suffix,
				Path:   []string{propertyName},
				Type:   valueType,
			})
		}
	}
	return bindings, nil
}

func containsHTTPHeaderAnnotation(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if _, exists := typed[mcpHeaderAnnotation]; exists {
			return true
		}
		for _, nested := range typed {
			if containsHTTPHeaderAnnotation(nested) {
				return true
			}
		}
	case []any:
		if slices.ContainsFunc(typed, containsHTTPHeaderAnnotation) {
			return true
		}
	}
	return false
}

func validHTTPHeaderSuffix(value string) bool {
	if value == "" {
		return false
	}
	const tcharPunctuation = "!#$%&'*+-.^_`|~"
	for index := range len(value) {
		char := value[index]
		if char >= '0' && char <= '9' || char >= 'A' && char <= 'Z' ||
			char >= 'a' && char <= 'z' || strings.ContainsRune(tcharPunctuation, rune(char)) {
			continue
		}
		return false
	}
	return true
}

// cloneHTTPToolHeaderBindings ensures neither tool descriptors nor transports
// share caller-owned path backing arrays.
func cloneHTTPToolHeaderBindings(bindings []HTTPToolHeaderBinding) []HTTPToolHeaderBinding {
	cloned := make([]HTTPToolHeaderBinding, len(bindings))
	for index, binding := range bindings {
		cloned[index] = binding
		cloned[index].Path = append([]string(nil), binding.Path...)
	}
	return cloned
}

func rawJSONIsNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte(jsonNull))
}
