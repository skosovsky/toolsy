package openapi

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// operationParamSets returns path, query, and body parameter name sets for an operation.
// pathNames: from pathTemplate placeholders {name}. queryNames: from parameters with In=="query".
// bodyNames: top-level keys from requestBody application/json schema (resolved via Schema.Value); nil if no body.
func operationParamSets(
	op *openapi3.Operation,
	pathItem *openapi3.PathItem,
	pathTemplate string,
) ([]string, []string, []string) {
	pathNames := pathParamNamesFromTemplate(pathTemplate)
	querySet := make(map[string]bool)
	mergeQueryParamsFromRefs(op.Parameters, querySet)
	mergeQueryParamsFromRefs(pathItem.Parameters, querySet)
	var queryNames []string
	for k := range querySet {
		queryNames = append(queryNames, k)
	}
	bodyNames := bodyParamNamesFromRequestBody(op)
	return pathNames, queryNames, bodyNames
}

func mergeQueryParamsFromRefs(refs []*openapi3.ParameterRef, set map[string]bool) {
	for _, pRef := range refs {
		if pRef == nil || pRef.Value == nil {
			continue
		}
		p := pRef.Value
		if p.In == "query" && p.Name != "" {
			set[p.Name] = true
		}
	}
}

func bodyParamNamesFromRequestBody(op *openapi3.Operation) []string {
	if op.RequestBody == nil || op.RequestBody.Value == nil || op.RequestBody.Value.Content == nil {
		return nil
	}
	mt, ok := op.RequestBody.Value.Content["application/json"]
	if !ok || mt == nil || mt.Schema == nil || mt.Schema.Value == nil {
		return nil
	}
	resolved := mt.Schema.Value
	if resolved.Properties == nil {
		return nil
	}
	var bodyNames []string
	for k := range resolved.Properties {
		bodyNames = append(bodyNames, k)
	}
	return bodyNames
}

func pathParamNamesFromTemplate(pathTemplate string) []string {
	var names []string
	for {
		i := strings.Index(pathTemplate, "{")
		if i < 0 {
			break
		}
		j := strings.Index(pathTemplate[i:], "}")
		if j < 0 {
			break
		}
		names = append(names, pathTemplate[i+1:i+j])
		pathTemplate = pathTemplate[i+j+1:]
	}
	return names
}

// operationToJSONSchema builds a single JSON Schema object from operation parameters and requestBody.
func operationToJSONSchema(op *openapi3.Operation, pathItem *openapi3.PathItem) ([]byte, error) {
	props := make(map[string]any)
	required := []string{}
	required = appendOperationParametersToProps(op, props, required)
	required = appendPathItemParametersToProps(pathItem, props, required)
	required = mergeRequestBodyJSONPropertiesIntoProps(op, props, required)

	out := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return json.Marshal(out)
}

func appendOperationParametersToProps(op *openapi3.Operation, props map[string]any, required []string) []string {
	for _, pRef := range op.Parameters {
		if pRef == nil || pRef.Value == nil {
			continue
		}
		p := pRef.Value
		name := p.Name
		if name == "" {
			continue
		}
		if p.Required {
			required = append(required, name)
		}
		schema := p.Schema
		if schema != nil && schema.Value != nil {
			props[name] = schemaRefToJSONSchemaMap(schema)
		} else {
			props[name] = map[string]any{"type": "string"}
		}
	}
	return required
}

func appendPathItemParametersToProps(pathItem *openapi3.PathItem, props map[string]any, required []string) []string {
	for _, pRef := range pathItem.Parameters {
		if pRef == nil || pRef.Value == nil {
			continue
		}
		p := pRef.Value
		name := p.Name
		if name == "" {
			continue
		}
		if _, exists := props[name]; exists {
			continue
		}
		if p.Required {
			required = append(required, name)
		}
		schema := p.Schema
		if schema != nil && schema.Value != nil {
			props[name] = schemaRefToJSONSchemaMap(schema)
		} else {
			props[name] = map[string]any{"type": "string"}
		}
	}
	return required
}

func mergeRequestBodyJSONPropertiesIntoProps(op *openapi3.Operation, props map[string]any, required []string) []string {
	if op.RequestBody == nil || op.RequestBody.Value == nil {
		return required
	}
	content := op.RequestBody.Value.Content
	if content == nil {
		return required
	}
	mt, ok := content["application/json"]
	if !ok || mt == nil || mt.Schema == nil || mt.Schema.Value == nil {
		return required
	}
	bodySchema := schemaToJSONSchemaMap(mt.Schema.Value)
	bodyProps, ok := bodySchema["properties"].(map[string]any)
	if !ok {
		return required
	}
	for k, v := range bodyProps {
		if _, exists := props[k]; exists {
			continue
		}
		props[k] = v
		if req, ok := bodySchema["required"].([]string); ok {
			if slices.Contains(req, k) {
				required = append(required, k)
			}
		}
	}
	return required
}

func schemaRefToJSONSchemaMap(s *openapi3.SchemaRef) map[string]any {
	if s == nil || s.Value == nil {
		return map[string]any{"type": "string"}
	}
	return schemaToJSONSchemaMap(s.Value)
}

func schemaToJSONSchemaMap(s *openapi3.Schema) map[string]any {
	if s == nil {
		return map[string]any{"type": "string"}
	}
	out := make(map[string]any)
	if s.Type != nil && len(*s.Type) > 0 {
		types := *s.Type
		if len(types) == 1 {
			out["type"] = types[0]
		} else {
			out["type"] = types
		}
	}
	if s.Format != "" {
		out["format"] = s.Format
	}
	if s.Description != "" {
		out["description"] = s.Description
	}
	if s.Properties != nil {
		props := make(map[string]any)
		for k, v := range s.Properties {
			props[k] = schemaRefToJSONSchemaMap(v)
		}
		out["properties"] = props
	}
	if len(s.Required) > 0 {
		out["required"] = s.Required
	}
	if s.Items != nil {
		out["items"] = schemaRefToJSONSchemaMap(s.Items)
	}
	if s.Enum != nil {
		out["enum"] = s.Enum
	}
	return out
}
