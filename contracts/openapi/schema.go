package openapi

import (
	"reflect"
	"sort"
	"strings"
)

const locationPath = "path"
const locationQuery = "query"
const defaultKeyword = "default"
const maxProjectionDepth = 64
const maxProjectionNodes = 4096

// UnsupportedError identifies a source contract outside the adapter's declared subset.
type UnsupportedError struct{ Reason string }

func (e *UnsupportedError) Error() string { return "openapi: unsupported contract: " + e.Reason }
func unsupported(reason string) error     { return &UnsupportedError{Reason: reason} }
func object(value any) map[string]any     { result, _ := value.(map[string]any); return result }
func stringValue(value any) string        { result, _ := value.(string); return result }
func boolValue(value any) bool            { result, _ := value.(bool); return result }
func keys(value map[string]any) []string {
	result := make([]string, 0, len(value))
	for key := range value {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func checkReferences(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		if ref, ok := typed["$ref"]; ok && !strings.HasPrefix(stringValue(ref), "#/components/") {
			return unsupported("external or non-component reference " + stringValue(ref))
		}
		for _, key := range keys(typed) {
			if err := checkReferences(typed[key]); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := checkReferences(child); err != nil {
				return err
			}
		}
	}
	return nil
}

type projector struct {
	source map[string]any
	nodes  int
	active map[string]bool
}

func (p *projector) resolve(value any, category string) (map[string]any, error) {
	current := object(value)
	if current == nil {
		return nil, unsupported("expected object in " + category)
	}
	visited := make(map[string]bool)
	for {
		ref, ok := current["$ref"]
		if !ok {
			return current, nil
		}
		pointer := stringValue(ref)
		prefix := "#/components/" + category + "/"
		if !strings.HasPrefix(pointer, prefix) || len(current) != 1 {
			return nil, unsupported("reference shape " + pointer)
		}
		if visited[pointer] || len(visited) >= maxProjectionDepth {
			return nil, unsupported("reference cycle or depth limit " + pointer)
		}
		visited[pointer] = true
		name := strings.TrimPrefix(pointer, prefix)
		if strings.Contains(name, "/") {
			return nil, unsupported("nested reference " + pointer)
		}
		name = strings.ReplaceAll(strings.ReplaceAll(name, "~1", "/"), "~0", "~")
		current = object(object(object(p.source["components"])[category])[name])
		if current == nil {
			return nil, unsupported("unresolved reference " + pointer)
		}
	}
}

func (p *projector) schema(value any, depth int) (map[string]any, error) {
	p.nodes++
	if depth > maxProjectionDepth || p.nodes > maxProjectionNodes {
		return nil, unsupported("schema projection complexity limit")
	}
	raw := object(value)
	if raw == nil {
		return nil, unsupported("schema must be an object")
	}
	if reference, ok := raw["$ref"]; ok {
		ref := stringValue(reference)
		if p.active[ref] {
			return nil, unsupported("recursive schema " + ref)
		}
		resolved, err := p.resolve(raw, "schemas")
		if err != nil {
			return nil, err
		}
		p.active[ref] = true
		result, err := p.schema(resolved, depth+1)
		delete(p.active, ref)
		return result, err
	}
	result := make(map[string]any)
	for _, key := range keys(raw) {
		mapped, keep, err := p.keyword(key, raw[key], depth)
		if err != nil {
			return nil, err
		}
		if keep {
			result[key] = mapped
		}
	}

	if err := convertOpenAPIBoundsAndNull(raw, result); err != nil {
		return nil, err
	}

	return result, nil
}

type parameter struct {
	name, location string
	explode        bool
	array          bool
}
type operationContract struct {
	input, output map[string]any
	parameters    []parameter
	body          bool
	responseJSON  map[string]bool
}

func closedObject(properties map[string]any, required []string) map[string]any {
	result := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		result["required"] = required
	}
	return result
}

//nolint:gocognit,funlen // Construction checks parameter identity and transport semantics before publishing a single contract.
func buildOperationContract(source, item, operation map[string]any, method, path string) (*operationContract, error) {
	projection := &projector{source: source, nodes: 0, active: make(map[string]bool)}
	merged := make(map[string]map[string]any)
	for _, owner := range []map[string]any{item, operation} {
		list, _ := owner["parameters"].([]any)
		for _, value := range list {
			param, err := projection.resolve(value, "parameters")
			if err != nil {
				return nil, err
			}
			identity := stringValue(param["in"]) + "\x00" + stringValue(param["name"])
			merged[identity] = param
		}
	}
	contract := new(operationContract)
	groups := map[string]map[string]any{locationPath: {}, locationQuery: {}}
	required := map[string][]string{locationPath: {}, locationQuery: {}}
	pathNames := make(map[string]bool)
	for _, identity := range keysOfParameters(merged) {
		raw := merged[identity]
		param, schema, err := projection.parameter(raw)
		if err != nil {
			return nil, err
		}
		location, name := param.location, param.name

		if location == locationPath {
			pathNames[name] = true
		}
		groups[location][name] = schema
		if boolValue(raw["required"]) {
			required[location] = append(required[location], name)
		}
		contract.parameters = append(
			contract.parameters,
			param,
		)
	}
	for _, name := range pathParamNamesFromTemplate(path) {
		if !pathNames[name] {
			return nil, unsupported("path placeholder without parameter " + name)
		}
		delete(pathNames, name)
	}
	if len(pathNames) > 0 {
		return nil, unsupported("path parameter without placeholder")
	}
	properties := make(map[string]any)
	var outerRequired []string
	for _, location := range []string{locationPath, locationQuery} {
		if len(groups[location]) > 0 {
			properties[location] = closedObject(groups[location], required[location])
			if len(required[location]) > 0 {
				outerRequired = append(outerRequired, location)
			}
		}
	}
	if body, exists := operation["requestBody"]; exists {
		schema, isRequired, err := projection.bodySchema(body, method)
		if err != nil {
			return nil, err
		}
		properties["body"] = schema
		contract.body = true
		if isRequired {
			outerRequired = append(outerRequired, "body")
		}
	}

	contract.input = closedObject(properties, outerRequired)
	output, err := projectOutput(projection, operation, contract)
	if err != nil {
		return nil, err
	}
	contract.output = output
	return contract, nil
}
func keysOfParameters(parameters map[string]map[string]any) []string {
	result := make([]string, 0, len(parameters))
	for key := range parameters {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func projectOutput(
	projection *projector,
	operation map[string]any,
	contract *operationContract,
) (map[string]any, error) {
	responses := object(operation["responses"])
	var output map[string]any
	var untyped bool
	contract.responseJSON = make(map[string]bool)
	for _, status := range keys(responses) {
		if status != defaultKeyword && (len(status) != 3 || status[0] != '2') {
			continue
		}
		response, err := projection.resolve(responses[status], "responses")
		if err != nil {
			return nil, err
		}
		content := object(response["content"])
		if len(content) == 0 {
			untyped = true
			contract.responseJSON[status] = false
			continue
		}
		if len(content) != 1 || content["application/json"] == nil {
			return nil, unsupported("successful response must declare only application/json")
		}
		schema, err := projection.schema(object(content["application/json"])["schema"], 0)
		if err != nil {
			return nil, err
		}
		if output != nil && !reflect.DeepEqual(output, schema) {
			return nil, unsupported("heterogeneous successful response schemas")
		}
		output = schema
		contract.responseJSON[status] = true
	}
	if len(contract.responseJSON) == 0 {
		return nil, unsupported("operation must declare a successful or default response")
	}
	if untyped && output != nil {
		return nil, unsupported("mixed empty and JSON successful response contracts")
	}
	return output, nil
}
func pathParamNamesFromTemplate(path string) []string {
	var result []string
	for {
		start := strings.IndexByte(path, '{')
		if start < 0 {
			return result
		}
		end := strings.IndexByte(path[start:], '}')
		if end < 0 {
			return result
		}
		result = append(result, path[start+1:start+end])
		path = path[start+end+1:]
	}
}

func convertOpenAPIBoundsAndNull(raw, result map[string]any) error {
	for _, bound := range []struct{ exclusive, ordinary string }{{"exclusiveMinimum", "minimum"}, {"exclusiveMaximum", "maximum"}} {
		if flag, exists := raw[bound.exclusive]; exists {
			exclusive, ok := flag.(bool)
			if !ok {
				return unsupported("OpenAPI 3.0 exclusive bound must be boolean")
			}
			if exclusive {
				number, exists := result[bound.ordinary]
				if !exists {
					return unsupported("exclusive bound without limit")
				}
				delete(result, bound.ordinary)
				result[bound.exclusive] = number
			}
		}
	}
	if boolValue(raw["nullable"]) {
		kind := stringValue(raw["type"])
		if kind == "" {
			return unsupported("nullable without explicit type")
		}
		result["type"] = []any{kind, "null"}
	}
	return nil
}

func (p *projector) parameter(raw map[string]any) (parameter, map[string]any, error) {
	location, name := stringValue(raw["in"]), stringValue(raw["name"])
	if location != locationPath && location != locationQuery {
		return parameter{}, nil, unsupported("parameter location " + location)
	}
	if name == "" || raw["content"] != nil || boolValue(raw["allowReserved"]) || boolValue(raw["allowEmptyValue"]) {
		return parameter{}, nil, unsupported("parameter content/allowReserved/allowEmptyValue")
	}
	style := stringValue(raw["style"])
	defaultStyle := "form"
	if location == locationPath {
		defaultStyle = "simple"
	}
	if style != "" && style != defaultStyle {
		return parameter{}, nil, unsupported("parameter style " + style)
	}
	schema, err := p.schema(raw["schema"], 0)
	if err != nil {
		return parameter{}, nil, err
	}
	kind := stringValue(schema["type"])
	array := kind == "array"
	if array && location == locationQuery {
		kind = stringValue(object(schema["items"])["type"])
	}
	if (kind != "string" && kind != "integer" && kind != "number" && kind != "boolean") ||
		(array && location == locationPath) {
		return parameter{}, nil, unsupported("parameter must be scalar or query scalar array")
	}
	explode := location == locationQuery
	if flag, ok := raw["explode"].(bool); ok {
		explode = flag
	}
	if location == locationPath && !boolValue(raw["required"]) {
		return parameter{}, nil, unsupported("path parameter must be required")
	}
	return parameter{name: name, location: location, explode: explode, array: array}, schema, nil
}

func (p *projector) accountLiteral(value any, depth int) error {
	p.nodes++
	if depth > maxProjectionDepth || p.nodes > maxProjectionNodes {
		return unsupported("schema literal projection complexity limit")
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys(typed) {
			if err := p.accountLiteral(typed[key], depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, entry := range typed {
			if err := p.accountLiteral(entry, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

//nolint:gocognit // The bounded keyword switch explicitly separates preserved, converted and unsupported semantics.
func (p *projector) keyword(key string, value any, depth int) (any, bool, error) {
	switch key {
	case "type":
		if _, ok := value.(string); !ok {
			return nil, false, unsupported("OpenAPI 3.0 type must be a string")
		}
		return value, true, nil
	case "enum",
		"minimum",
		"maximum",
		"multipleOf",
		"minLength",
		"maxLength",
		"pattern",
		"minItems",
		"maxItems",
		"uniqueItems",
		"minProperties",
		"maxProperties",
		"required",
		"title",
		"description",
		defaultKeyword:
		if err := p.accountLiteral(value, depth+1); err != nil {
			return nil, false, err
		}
		return value, true, nil
	case "properties":
		properties := object(value)
		if properties == nil {
			return nil, false, unsupported("properties must be an object")
		}
		mapped := make(map[string]any)
		for _, name := range keys(properties) {
			child, err := p.schema(properties[name], depth+1)
			if err != nil {
				return nil, false, err
			}
			mapped[name] = child
		}
		return mapped, true, nil
	case "items", "not":
		child, err := p.schema(value, depth+1)
		return child, true, err
	case "additionalProperties":
		if flag, ok := value.(bool); ok {
			return flag, true, nil
		}
		child, err := p.schema(value, depth+1)
		return child, true, err
	case "allOf", "anyOf", "oneOf":
		variants, ok := value.([]any)
		if !ok {
			return nil, false, unsupported("composition must be an array")
		}
		mapped := make([]any, 0, len(variants))
		for _, variant := range variants {
			child, err := p.schema(variant, depth+1)
			if err != nil {
				return nil, false, err
			}
			mapped = append(mapped, child)
		}
		return mapped, true, nil
	case "exclusiveMinimum", "exclusiveMaximum", "nullable", "example", "deprecated":
		return nil, false, nil
	case "readOnly", "writeOnly":
		if boolValue(value) {
			return nil, false, unsupported("directional keyword " + key)
		}
		return nil, false, nil
	default:
		return nil, false, unsupported("schema keyword " + key)
	}
}

func (p *projector) bodySchema(body any, method string) (map[string]any, bool, error) {
	if method != "POST" && method != "PUT" && method != "PATCH" {
		return nil, false, unsupported("request body only supported for POST/PUT/PATCH")
	}
	raw, err := p.resolve(body, "requestBodies")
	if err != nil {
		return nil, false, err
	}
	content := object(raw["content"])
	if len(content) != 1 || content["application/json"] == nil {
		return nil, false, unsupported("request body must declare only application/json")
	}
	schema, err := p.schema(object(content["application/json"])["schema"], 0)
	return schema, boolValue(raw["required"]), err
}
