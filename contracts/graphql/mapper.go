package graphql

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

const jsonSchemaTypeKey = "type"
const graphqlTypeKindNonNull = "NON_NULL"
const jsonSchemaString = "string"
const additionalPropertiesKey = "additionalProperties"
const jsonSchemaObject = "object"
const propertiesKey = "properties"
const requiredKey = "required"
const maxDefaultBytes = 65536
const maxContractDepth = 16
const maxContractNodes = 4096

var graphqlName = regexp.MustCompile(`^[_A-Za-z][_0-9A-Za-z]*$`)

type graphQLTypeRef struct {
	Name   string          `json:"name"`
	Kind   string          `json:"kind"`
	OfType *graphQLTypeRef `json:"ofType,omitempty"`
}

// ArgSpec describes one introspected argument or input field.
type ArgSpec struct {
	Name         string         `json:"name"`
	Type         graphQLTypeRef `json:"type"`
	DefaultValue *string        `json:"defaultValue"`
}

type projection struct {
	types  map[string]introSchemaType
	nodes  int
	active map[string]bool
}

func (p *projection) enter(depth int) error {
	p.nodes++
	if depth > maxContractDepth || p.nodes > maxContractNodes {
		return errors.New("graphql: contract traversal limit exceeded")
	}
	return nil
}
func nullable(schema map[string]any) map[string]any {
	return map[string]any{"anyOf": []any{schema, map[string]any{jsonSchemaTypeKey: "null"}}}
}

func (p *projection) project(
	t *graphQLTypeRef,
	selection []Selection,
	output bool,
	depth int,
) (map[string]any, string, error) {
	if err := p.enter(depth); err != nil {
		return nil, "", err
	}
	if t == nil {
		return nil, "", errors.New("graphql: incomplete type reference")
	}
	if t.Kind == graphqlTypeKindNonNull {
		if t.OfType == nil || t.OfType.Kind == graphqlTypeKindNonNull {
			return nil, "", errors.New("graphql: invalid non-null reference")
		}
		return p.nonNull(t.OfType, selection, output, depth+1)
	}
	s, q, e := p.nonNull(t, selection, output, depth+1)
	if e != nil {
		return nil, "", e
	}
	return nullable(s), q, nil
}

func (p *projection) nonNull(
	t *graphQLTypeRef,
	selection []Selection,
	output bool,
	depth int,
) (map[string]any, string, error) {
	if err := p.enter(depth); err != nil {
		return nil, "", err
	}
	if t.Kind == "LIST" {
		item, q, e := p.project(t.OfType, selection, output, depth+1)
		schema := map[string]any{jsonSchemaTypeKey: "array", "items": item}
		if !output {
			singleton := map[string]any{
				"allOf": []any{item, map[string]any{"not": map[string]any{jsonSchemaTypeKey: "null"}}},
			}
			schema = map[string]any{"anyOf": []any{schema, singleton}}
		}
		return schema, q, e
	}
	if t.OfType != nil {
		return nil, "", errors.New("graphql: invalid named type wrapper")
	}
	if !graphqlName.MatchString(t.Name) {
		return nil, "", fmt.Errorf("graphql: invalid named reference %q", t.Name)
	}
	typ, ok := p.types[t.Name]
	if !ok || typ.Kind != t.Kind {
		return nil, "", fmt.Errorf("graphql: unknown/mismatched type %s", t.Name)
	}
	if t.Kind == "SCALAR" || t.Kind == "ENUM" {
		if len(selection) > 0 {
			return nil, "", fmt.Errorf("graphql: leaf type %s has selection", t.Name)
		}
		return p.leafSchema(typ, output)
	}
	if !output && t.Kind == "INPUT_OBJECT" {
		return p.inputObject(typ, depth)
	}
	if !output {
		return nil, "", fmt.Errorf("graphql: unsupported input %s", t.Name)
	}
	return p.outputObject(typ, selection, depth)
}
func (p *projection) leafSchema(typ introSchemaType, output bool) (map[string]any, string, error) {
	switch typ.Kind {
	case "ENUM":
		p.nodes += len(typ.EnumValues)
		if p.nodes > maxContractNodes {
			return nil, "", errors.New("graphql: enum projection size limit exceeded")
		}
		values := make([]string, 0, len(typ.EnumValues))
		for _, v := range typ.EnumValues {
			if !graphqlName.MatchString(v.Name) {
				return nil, "", errors.New("graphql: invalid enum value")
			}
			values = append(values, v.Name)
		}
		if len(values) == 0 {
			return nil, "", errors.New("graphql: empty enum")
		}
		return map[string]any{jsonSchemaTypeKey: jsonSchemaString, "enum": values}, "", nil
	default:
		switch typ.Name {
		case "String":
			return map[string]any{jsonSchemaTypeKey: jsonSchemaString}, "", nil
		case "ID":
			if output {
				return map[string]any{jsonSchemaTypeKey: jsonSchemaString}, "", nil
			}
			return map[string]any{jsonSchemaTypeKey: []string{jsonSchemaString, "integer"}}, "", nil
		case "Int":
			return map[string]any{
				jsonSchemaTypeKey: "integer",
				"minimum":         math.MinInt32,
				"maximum":         math.MaxInt32,
			}, "", nil
		case "Float":
			return map[string]any{
				jsonSchemaTypeKey: "number",
				"minimum":         -math.MaxFloat64,
				"maximum":         math.MaxFloat64,
			}, "", nil
		case "Boolean":
			return map[string]any{jsonSchemaTypeKey: "boolean"}, "", nil
		default:
			return nil, "", fmt.Errorf("graphql: unsupported custom scalar %s", typ.Name)
		}
	}
}
func (p *projection) inputObject(typ introSchemaType, depth int) (map[string]any, string, error) {
	if typ.IsOneOf {
		return nil, "", errors.New("graphql: unsupported oneOf input object")
	}
	props := map[string]any{}
	required := []string{}
	if p.active[typ.Name] {
		return nil, "", fmt.Errorf("graphql: unsupported recursive input %s", typ.Name)
	}
	p.active[typ.Name] = true
	defer delete(p.active, typ.Name)
	for _, f := range typ.InputFields {
		if f.DefaultValue != nil {
			if _, err := defaultLiteral(*f.DefaultValue); err != nil {
				return nil, "", err
			}
		}
		if !graphqlName.MatchString(f.Name) {
			return nil, "", errors.New("graphql: invalid input field")
		}
		if _, exists := props[f.Name]; exists {
			return nil, "", errors.New("graphql: duplicate input field")
		}
		s, _, e := p.project(&f.Type, nil, false, depth+1)
		if e != nil {
			return nil, "", e
		}
		props[f.Name] = s
		if f.Type.Kind == graphqlTypeKindNonNull && f.DefaultValue == nil {
			required = append(required, f.Name)
		}
	}
	return map[string]any{
		jsonSchemaTypeKey:       jsonSchemaObject,
		propertiesKey:           props,
		requiredKey:             required,
		additionalPropertiesKey: false,
	}, "", nil
}

func (p *projection) outputObject(
	typ introSchemaType,
	selection []Selection,
	depth int,
) (map[string]any, string, error) {
	props := map[string]any{}
	required := []string{}
	if typ.Kind != "OBJECT" {
		return nil, "", fmt.Errorf("graphql: unsupported %s %s", typ.Kind, typ.Name)
	}
	if len(selection) == 0 {
		return nil, "", fmt.Errorf("graphql: object %s requires host selection", typ.Name)
	}
	queries := []string{}
	for _, sel := range selection {
		if !graphqlName.MatchString(sel.Name) {
			return nil, "", errors.New("graphql: invalid selection name")
		}
		if _, exists := props[sel.Name]; exists {
			return nil, "", errors.New("graphql: duplicate selection")
		}
		field, err := selectedField(typ, sel.Name)
		if err != nil {
			return nil, "", err
		}
		s, q, e := p.project(&field.Type, sel.Fields, true, depth+1)
		if e != nil {
			return nil, "", e
		}
		props[sel.Name] = s
		required = append(required, sel.Name)
		queries = append(queries, sel.Name+q)
	}
	return map[string]any{
		jsonSchemaTypeKey:       jsonSchemaObject,
		propertiesKey:           props,
		requiredKey:             required,
		additionalPropertiesKey: false,
	}, " { " + strings.Join(
		queries,
		" ",
	) + " }", nil
}
func argsToJSONSchema(args []ArgSpec, types map[string]introSchemaType) ([]byte, error) {
	p := projection{types: types, nodes: 0, active: map[string]bool{}}
	props := map[string]any{}
	required := []string{}
	for _, a := range args {
		if !graphqlName.MatchString(a.Name) {
			return nil, errors.New("graphql: invalid argument name")
		}
		if _, ok := props[a.Name]; ok {
			return nil, errors.New("graphql: duplicate argument")
		}
		s, _, e := p.project(&a.Type, nil, false, 0)
		if e != nil {
			return nil, e
		}
		props[a.Name] = s
		if a.Type.Kind == graphqlTypeKindNonNull && a.DefaultValue == nil {
			required = append(required, a.Name)
		}
	}
	return json.Marshal(
		map[string]any{
			jsonSchemaTypeKey:       jsonSchemaObject,
			propertiesKey:           props,
			requiredKey:             required,
			additionalPropertiesKey: false,
		},
	)
}

func buildOutputContract(
	t *graphQLTypeRef,
	selections []Selection,
	types map[string]introSchemaType,
) (string, map[string]any, error) {
	p := projection{types: types, nodes: 0, active: map[string]bool{}}
	schema, query, err := p.project(t, selections, true, 0)
	return query, schema, err
}
func buildStaticQuery(kind, field string, args []ArgSpec, selection string) (string, error) {
	decls := []string{}
	uses := []string{}
	for _, a := range args {
		declaration := "$" + a.Name + ": " + graphQLTypeString(&a.Type)
		if a.DefaultValue != nil {
			literal, err := defaultLiteral(*a.DefaultValue)
			if err != nil {
				return "", err
			}
			declaration += " = " + literal
		}
		decls = append(decls, declaration)
		uses = append(uses, a.Name+": $"+a.Name)
	}
	query := kind
	if len(decls) > 0 {
		query += "(" + strings.Join(decls, ", ") + ")"
	}
	query += " { " + field
	if len(uses) > 0 {
		query += "(" + strings.Join(uses, ", ") + ")"
	}
	return query + selection + " }", nil
}
func graphQLTypeString(t *graphQLTypeRef) string {
	if t.Kind == graphqlTypeKindNonNull {
		return graphQLTypeString(t.OfType) + "!"
	}
	if t.Kind == "LIST" {
		return "[" + graphQLTypeString(t.OfType) + "]"
	}
	return t.Name
}

func defaultLiteral(source string) (string, error) {
	if len(source) > maxDefaultBytes {
		return "", errors.New("graphql: default literal limit exceeded")
	}
	doc, err := parser.ParseQueryWithTokenLimit(
		&ast.Source{Name: "default", Input: "query($x: String = " + source + ") { x }", BuiltIn: false},
		maxContractNodes,
	)
	if err != nil || len(doc.Operations) != 1 || len(doc.Fragments) != 0 {
		return "", errors.New("graphql: invalid default literal")
	}
	op := doc.Operations[0]
	if len(op.VariableDefinitions) != 1 || len(op.SelectionSet) != 1 || op.VariableDefinitions[0].DefaultValue == nil {
		return "", errors.New("graphql: invalid default literal")
	}
	field, ok := op.SelectionSet[0].(*ast.Field)
	if !ok || field.Name != "x" || len(field.Arguments) > 0 || len(field.SelectionSet) > 0 || len(op.Directives) > 0 {
		return "", errors.New("graphql: invalid default literal")
	}
	return op.VariableDefinitions[0].DefaultValue.String(), nil
}

func selectedField(typ introSchemaType, name string) (*introField, error) {
	for i := range typ.Fields {
		field := &typ.Fields[i]
		if field.Name != name {
			continue
		}
		for _, a := range field.Args {
			if a.Type.Kind == graphqlTypeKindNonNull && a.DefaultValue == nil {
				return nil, fmt.Errorf("graphql: selected field requires argument %s", a.Name)
			}
		}
		return field, nil
	}
	return nil, fmt.Errorf("graphql: unknown field %s.%s", typ.Name, name)
}
