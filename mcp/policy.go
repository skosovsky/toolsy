package mcp

import (
	"bytes"
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

// ToolAnnotations carries MCP tool hints mapped into toolsy manifest policy fields.
// OpenWorldHint is parsed for forward compatibility but intentionally not mapped to manifest fields.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
	Extra           Meta   `json:"-"`
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
