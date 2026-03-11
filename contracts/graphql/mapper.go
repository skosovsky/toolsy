package graphql

import (
	"encoding/json"
	"strings"
)

// graphQLTypeRef matches GraphQL introspection __Type for type references (supports arbitrary depth via OfType).
type graphQLTypeRef struct {
	Name   string          `json:"name"`
	Kind   string          `json:"kind"`
	OfType *graphQLTypeRef `json:"ofType,omitempty"`
}

// ArgSpec is one argument from GraphQL introspection (field args).
type ArgSpec struct {
	Name string         `json:"name"`
	Type graphQLTypeRef `json:"type"`
}

func argsToJSONSchema(args []ArgSpec) ([]byte, error) {
	props := make(map[string]any)
	var required []string
	for _, a := range args {
		if a.Name == "" {
			continue
		}
		props[a.Name] = graphQLTypeToJSONSchemaInner(&a.Type)
		if a.Type.Kind == "NON_NULL" {
			required = append(required, a.Name)
		}
	}
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return json.Marshal(out)
}

func graphQLTypeToJSONSchemaInner(t *graphQLTypeRef) map[string]any {
	if t == nil {
		return map[string]any{"type": "string"}
	}
	if t.Kind == "NON_NULL" && t.OfType != nil {
		return graphQLTypeToJSONSchemaInner(t.OfType)
	}
	if t.Kind == "LIST" && t.OfType != nil {
		return map[string]any{"type": "array", "items": graphQLTypeToJSONSchemaInner(t.OfType)}
	}
	// Scalar or named type
	switch strings.ToLower(t.Name) {
	case "int", "integer":
		return map[string]any{"type": "integer"}
	case "float":
		return map[string]any{"type": "number"}
	case "boolean", "bool":
		return map[string]any{"type": "boolean"}
	case "id", "string":
		return map[string]any{"type": "string"}
	default:
		return map[string]any{"type": "string"}
	}
}

// buildStaticQuery generates a static query/mutation string with variable declarations; args go in variables only.
func buildStaticQuery(opType, fieldName string, args []ArgSpec) string {
	var decls []string
	var uses []string
	for _, a := range args {
		if a.Name == "" {
			continue
		}
		decls = append(decls, "$"+a.Name+": "+graphQLTypeString(&a.Type))
		uses = append(uses, a.Name+": $"+a.Name)
	}
	var b strings.Builder
	b.WriteString(opType)
	if len(decls) > 0 {
		b.WriteString("(")
		b.WriteString(strings.Join(decls, ", "))
		b.WriteString(")")
	}
	b.WriteString(" { ")
	b.WriteString(fieldName)
	if len(uses) > 0 {
		b.WriteString("(")
		b.WriteString(strings.Join(uses, ", "))
		b.WriteString(")")
	}
	b.WriteString(" { __typename } }")
	return b.String()
}

func graphQLTypeString(t *graphQLTypeRef) string {
	if t == nil {
		return "String"
	}
	if t.Kind == "NON_NULL" && t.OfType != nil {
		return graphQLTypeString(t.OfType) + "!"
	}
	if t.Kind == "LIST" && t.OfType != nil {
		return "[" + graphQLTypeString(t.OfType) + "]"
	}
	return t.Name
}
