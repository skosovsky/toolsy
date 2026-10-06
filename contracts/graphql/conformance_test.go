package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/jsonschemax"
	"github.com/skosovsky/toolsy/textprocessor"
)

const fixtureSDL = `enum Color { RED BLUE } input Filter { color: Color! count: Int! = 1 tags: [String!] } type Item { name: String! color: Color } type Query { scalar(id: ID! = 1, score: Float, count: Int): Int! color: Color item(filter: Filter!): Item! }`

func fixtureTypes() []introSchemaType {
	scalar := func(name string) introSchemaType { return introSchemaType{Name: name, Kind: "SCALAR"} }
	ref := func(kind, name string) graphQLTypeRef { return graphQLTypeRef{Kind: kind, Name: name} }
	nonnull := func(r graphQLTypeRef) graphQLTypeRef { return graphQLTypeRef{Kind: "NON_NULL", OfType: &r} }
	defaultCount := "1"
	return []introSchemaType{
		scalar("String"),
		scalar("ID"),
		scalar("Int"),
		scalar("Float"),
		{Name: "Color", Kind: "ENUM", EnumValues: []introTypeName{{Name: "RED"}, {Name: "BLUE"}}},
		{
			Name: "Filter",
			Kind: "INPUT_OBJECT",
			InputFields: []ArgSpec{
				{Name: "color", Type: nonnull(ref("ENUM", "Color"))},
				{Name: "count", Type: nonnull(ref("SCALAR", "Int")), DefaultValue: &defaultCount},
				{Name: "tags", Type: graphQLTypeRef{Kind: "LIST", OfType: new(nonnull(ref("SCALAR", "String")))}},
			},
		},
		{
			Name: "Item",
			Kind: "OBJECT",
			Fields: []introField{
				{Name: "name", Type: nonnull(ref("SCALAR", "String"))},
				{Name: "color", Type: ref("ENUM", "Color")},
			},
		},
		{
			Name: "Query",
			Kind: "OBJECT",
			Fields: []introField{
				{Name: "color", Type: ref("ENUM", "Color")},
				{
					Name: "scalar",
					Type: nonnull(ref("SCALAR", "Int")),
					Args: []ArgSpec{
						{Name: "id", Type: nonnull(ref("SCALAR", "ID")), DefaultValue: &defaultCount},
						{Name: "score", Type: ref("SCALAR", "Float")},
						{Name: "count", Type: ref("SCALAR", "Int")},
					},
				},
				{
					Name: "item",
					Type: nonnull(ref("OBJECT", "Item")),
					Args: []ArgSpec{{Name: "filter", Type: nonnull(ref("INPUT_OBJECT", "Filter"))}},
				},
			},
		},
	}
}

//nolint:gocognit // One integrated request contract checks discovery, variables, results and negative inputs.
func TestProtocolConformance(t *testing.T) {
	// Arrange: real schema/query and variable coercion validation on every request.
	schema, err := gqlparser.LoadSchema(&ast.Source{Input: fixtureSDL})
	if err != nil {
		t.Fatal(err)
	}
	var receivedID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		if e := decoder.Decode(&request); e != nil {
			t.Error(e)
			http.Error(w, "bad JSON", http.StatusBadRequest)
			return
		}
		doc, errs := gqlparser.LoadQueryWithRules(schema, request.Query, nil)
		if len(errs) > 0 {
			t.Error(errs)
			http.Error(w, "bad GraphQL", http.StatusBadRequest)
			return
		}
		if request.Query == introspectionQuery {
			_ = json.NewEncoder(w).
				Encode(introResponse{Data: &introData{Schema: introSchema{QueryType: &introTypeName{Name: "Query"}, Types: fixtureTypes()}}})
			return
		}
		if _, e := validator.VariableValues(
			schema,
			doc.Operations[0],
			normalizeIntegralNumbers(request.Variables).(map[string]any),
		); e != nil {
			t.Error(e)
			http.Error(w, "bad variables", http.StatusBadRequest)
			return
		}
		field := doc.Operations[0].SelectionSet[0].(*ast.Field)
		if field.Name == "color" {
			_, _ = w.Write([]byte(`{"data":{"color":"BLUE"}}`))
			return
		}
		if field.Name == "scalar" {
			if id, ok := request.Variables["id"].(json.Number); ok {
				receivedID = id.String()
			}
			_, _ = w.Write([]byte(`{"data":{"scalar":7}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"item":{"name":"useful","color":"RED"}}}`))
	}))
	defer server.Close()
	// Act: static scalar/object queries and input schemas originate from the same contract.
	tools, err := Introspect(
		context.Background(),
		server.URL,
		Options{
			AllowPrivateIPs: true,
			Selections:      map[string][]Selection{"query.item": {{Name: "name"}, {Name: "color"}}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	// Assert: useful bounded JSON data, GraphQL defaults, list coercion and exact integer IDs.
	cases := map[string][]string{
		"graphql_query_color":  {`{}`},
		"graphql_query_scalar": {`{}`, `{"count":1.0}`, `{"count":1e0}`, `{"id":9007199254740993}`},
		"graphql_query_item": {
			`{"filter":{"color":"RED","tags":"single"}}`,
			`{"filter":{"color":"RED","count":1.0}}`,
			`{"filter":{"color":"RED","count":1e0}}`,
		},
	}
	for _, tool := range tools {
		for _, input := range cases[tool.Manifest().Name] {
			count := 0
			err = tool.Execute(
				context.Background(),
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(input)},
				func(c toolsy.Chunk) error {
					count++
					if c.MimeType != toolsy.MimeTypeJSON || !json.Valid(c.Data) {
						t.Fatal("invalid result")
					}
					return nil
				},
			)
			if err != nil || count != 1 {
				t.Fatalf("%s: %v chunks=%d", tool.Manifest().Name, err, count)
			}
		}
	}
	if len(tools) != 3 || receivedID != "9007199254740993" {
		t.Fatalf("tools=%d ID=%s", len(tools), receivedID)
	}
	item := tools[1]
	for _, invalid := range []string{`{"filter":{"color":"GREEN"}}`, `{"filter":{"color":"RED","count":null}}`, `{"filter":{"color":"RED","unknown":1}}`, `{"filter":{"color":"RED","count":2147483648}}`, `{"filter":{"color":"RED","count":1.5}}`} {
		err = item.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(invalid)},
			func(toolsy.Chunk) error { t.Fatal("invalid input yielded"); return nil },
		)
		if err == nil {
			t.Fatalf("accepted %s", invalid)
		}
	}
	err = tools[2].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"score":1e400}`)},
		func(toolsy.Chunk) error { t.Fatal("nonfinite Float accepted"); return nil },
	)
	if err == nil {
		t.Fatal("Float exceeds finite double")
	}
}
func TestUnsupportedAndBoundedProjection(t *testing.T) {
	types, err := buildTypeMap(fixtureTypes())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ref       graphQLTypeRef
		selection []Selection
	}{
		{graphQLTypeRef{Kind: "OBJECT", Name: "Item"}, nil},
		{graphQLTypeRef{Kind: "OBJECT", Name: "Item"}, []Selection{{Name: "name { injected }"}}},
		{graphQLTypeRef{Kind: "SCALAR", Name: "Int"}, []Selection{{Name: "name"}}},
		{graphQLTypeRef{Kind: "LIST"}, nil},
		{graphQLTypeRef{Kind: "SCALAR", Name: "Custom"}, nil},
	} {
		if _, _, e := buildOutputContract(&tc.ref, tc.selection, types); e == nil {
			t.Fatal("unsupported output accepted")
		}
	}
	values := make([]introTypeName, 2000)
	for i := range values {
		values[i] = introTypeName{Name: "V" + strconv.Itoa(i)}
	}
	types["LargeEnum"] = introSchemaType{Name: "LargeEnum", Kind: "ENUM", EnumValues: values}
	args := []ArgSpec{}
	for i := range 3 {
		args = append(
			args,
			ArgSpec{Name: "arg" + strconv.Itoa(i), Type: graphQLTypeRef{Kind: "ENUM", Name: "LargeEnum"}},
		)
	}
	if _, err = argsToJSONSchema(args, types); err == nil {
		t.Fatal("repeated enum expansion exceeds budget")
	}
	types["One"] = introSchemaType{Name: "One", Kind: "INPUT_OBJECT", IsOneOf: true}
	if _, err = argsToJSONSchema(
		[]ArgSpec{{Name: "one", Type: graphQLTypeRef{Name: "One", Kind: "INPUT_OBJECT"}}},
		types,
	); err == nil {
		t.Fatal("oneOf input accepted")
	}
	recursive := graphQLTypeRef{Kind: "INPUT_OBJECT", Name: "Loop"}
	types["Loop"] = introSchemaType{
		Name:        "Loop",
		Kind:        "INPUT_OBJECT",
		InputFields: []ArgSpec{{Name: "again", Type: recursive}},
	}
	if _, err = argsToJSONSchema([]ArgSpec{{Name: "loop", Type: recursive}}, types); err == nil {
		t.Fatal("recursive input accepted")
	}
	ref := graphQLTypeRef{Kind: "SCALAR", Name: "Int"}
	for range 100 {
		ref = graphQLTypeRef{Kind: "LIST", OfType: new(ref)}
	}
	if _, err = argsToJSONSchema([]ArgSpec{{Name: "deep", Type: ref}}, types); err == nil {
		t.Fatal("unbounded reference accepted")
	}
	for _, literal := range []string{`1) { x } query($x: String = 2`, `$var`, `"unterminated`} {
		if _, err = defaultLiteral(literal); err == nil {
			t.Fatalf("accepted default %q", literal)
		}
	}
}
func TestExecutionFailsClosed(t *testing.T) {
	for _, body := range []string{`{"data":{"demo":"too large"}}`, `not JSON`, `{"errors"
 "math/big":[{"message":"denied"}],"data":{"demo":1}}`, `{"data":{}}`} {
		server := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }),
		)
		limit := 512
		if body == `{"data":{"demo":"too large"}}` {
			limit = 5
		}
		yielded := false
		err := executeGraphQL(
			context.Background(),
			toolsy.NewRunEnv(nil),
			"demo",
			server.URL,
			"query { demo }",
			"demo",
			nil,
			&Options{AllowPrivateIPs: true, MaxResponseBytes: limit},
			func(toolsy.Chunk) error { yielded = true; return nil },
		)
		server.Close()
		if err == nil || yielded {
			t.Fatalf("invalid response accepted: %s", body)
		}
		if limit == 5 && !errors.Is(err, textprocessor.ErrReadLimitExceeded) {
			t.Fatalf("missing limit error: %v", err)
		}
	}
}

func TestListNullabilityContract(t *testing.T) {
	// Arrange: list item nullability must not weaken non-null list presence.
	types, err := buildTypeMap(fixtureTypes())
	if err != nil {
		t.Fatal(err)
	}
	list := graphQLTypeRef{Kind: "LIST", OfType: new(graphQLTypeRef{Kind: "SCALAR", Name: "Int"})}
	for _, nonNull := range []bool{false, true} {
		typ := list
		if nonNull {
			typ = graphQLTypeRef{Kind: graphqlTypeKindNonNull, OfType: new(list)}
		}
		raw, e := argsToJSONSchema([]ArgSpec{{Name: "values", Type: typ}}, types)
		if e != nil {
			t.Fatal(e)
		}
		value, e := jsonschemax.Decode(raw)
		if e != nil {
			t.Fatal(e)
		}
		schema, e := jsonschemax.Compile(value)
		if e != nil {
			t.Fatal(e)
		}
		// Act and assert against the shared executable validator.
		for _, tc := range []struct {
			input string
			valid bool
		}{{`{"values":null}`, !nonNull}, {`{"values":[null]}`, true}, {`{"values":1}`, true}, {`{"values":[1]}`, true}} {
			input, e := jsonschemax.Decode([]byte(tc.input))
			if e != nil {
				t.Fatal(e)
			}
			if (schema.Validate(input) == nil) != tc.valid {
				t.Fatalf("nonnull=%v input=%s", nonNull, tc.input)
			}
		}
	}
}

// normalizeIntegralNumbers adapts gqlparser's lexical [json.Number.Int64] parsing to
// GraphQL §3.5 transport semantics: 1.0 and 1e0 are integral JSON numbers. Exact
// rational arithmetic keeps IDs beyond 2^53 intact. Production preserves raw JSON.
func normalizeIntegralNumbers(value any) any {
	switch typed := value.(type) {
	case json.Number:
		rational, ok := new(big.Rat).SetString(typed.String())
		if ok && rational.IsInt() {
			return json.Number(rational.Num().String())
		}
		return typed
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized[key] = normalizeIntegralNumbers(item)
		}
		return normalized
	case []any:
		normalized := make([]any, len(typed))
		for i, item := range typed {
			normalized[i] = normalizeIntegralNumbers(item)
		}
		return normalized
	default:
		return value
	}
}

func TestIntegralJSONSourceCoercion(t *testing.T) {
	// Arrange: gqlparser validates the source Int argument after exact transport normalization.
	schema, err := gqlparser.LoadSchema(&ast.Source{Input: fixtureSDL})
	if err != nil {
		t.Fatal(err)
	}
	query, errs := gqlparser.LoadQueryWithRules(schema, `query($count:Int) { scalar(count:$count) }`, nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for _, tc := range []struct {
		literal string
		valid   bool
	}{{"1.0", true}, {"1e0", true}, {"1.5", false}} {
		variables := map[string]any{"count": json.Number(tc.literal)}
		// Act.
		_, err = validator.VariableValues(
			schema,
			query.Operations[0],
			normalizeIntegralNumbers(variables).(map[string]any),
		)
		// Assert.
		if (err == nil) != tc.valid {
			t.Fatalf("source Int accepts %s = %v: %v", tc.literal, err == nil, err)
		}
	}
}
