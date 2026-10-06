package openapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

func fixtureSpec(paths string, components string) string {
	return `{"openapi":"3.0.3","info":{"title":"fixture","version":"1"},"paths":` + paths + `,"components":` + components + `}`
}
func discoverFixture(t *testing.T, spec string, handler http.HandlerFunc) ([]toolsy.Tool, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/spec.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, spec)
			return
		}
		if handler != nil {
			handler(w, r)
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(server.Close)
	options := Options{HTTPClient: server.Client(), AllowPrivateIPs: true}
	return ParseURL(t.Context(), server.URL+"/spec.json", options)
}

//nolint:gocognit // Parity intentionally runs independent original and projected validators over the same fixtures.
func TestSourceProjectedSchemaParity(t *testing.T) {
	fixtures := []struct {
		name, schema string
		values       []string
	}{
		{
			"minimum",
			`{"type":"number","minimum":2,"maximum":8,"exclusiveMinimum":true,"multipleOf":2}`,
			[]string{`2`, `4`, `9`},
		},
		{
			"pattern",
			`{"type":"string","pattern":"^[a-z]+$","minLength":2,"maxLength":4}`,
			[]string{`"ab"`, `"a"`, `"ABC"`, `"abcde"`},
		},
		{
			"additionalProperties",
			`{"type":"object","properties":{"x":{"type":"integer"}},"required":["x"],"additionalProperties":false}`,
			[]string{`{"x":0}`, `{}`, `{"x":1,"y":2}`},
		},
		{
			"composition",
			`{"allOf":[{"type":"integer","minimum":0},{"oneOf":[{"maximum":2},{"minimum":4}]}],"not":{"enum":[5]}}`,
			[]string{`0`, `3`, `4`, `5`, `-1`},
		},
		{"nullable", `{"type":"string","nullable":true,"minLength":2}`, []string{`null`, `""`, `"ab"`, `0`}},
		{
			"array",
			`{"type":"array","items":{"type":"string"},"uniqueItems":true,"minItems":1,"maxItems":2}`,
			[]string{`[]`, `["a"]`, `["a","a"]`, `["a","b","c"]`},
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange: the original OpenAPI schema and projected executable schema use independent validators.
			var original openapi3.Schema
			if err := json.Unmarshal([]byte(fixture.schema), &original); err != nil {
				t.Fatal(err)
			}
			value, err := jsonschemax.Decode([]byte(fixture.schema))
			if err != nil {
				t.Fatal(err)
			}
			projection := &projector{source: map[string]any{}, active: make(map[string]bool)}
			projected, err := projection.schema(value, 0)
			if err != nil {
				t.Fatal(err)
			}
			validator, err := jsonschemax.Compile(projected)
			if err != nil {
				t.Fatal(err)
			}
			for _, raw := range fixture.values {
				var originalValue any
				if err := json.Unmarshal([]byte(raw), &originalValue); err != nil {
					t.Fatal(err)
				}
				mappedValue, err := jsonschemax.Decode([]byte(raw))
				if err != nil {
					t.Fatal(err)
				}
				// Act.
				sourceErr := original.VisitJSON(originalValue)
				targetErr := validator.Validate(mappedValue)
				// Assert.
				if (sourceErr == nil) != (targetErr == nil) {
					t.Fatalf("value=%s source=%v projected=%v schema=%v", raw, sourceErr, targetErr, projected)
				}
			}
		})
	}
}

func TestDiscoveryTransportLocationsAndArrayBody(t *testing.T) {
	// Arrange: same parameter name has three identities, operation query overrides path-item query.
	spec := fixtureSpec(
		`{"/items/{id}":{"parameters":[{"name":"id","in":"query","schema":{"type":"string"}}],"post":{"operationId":"create","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}},{"name":"id","in":"query","required":true,"schema":{"type":"integer"}},{"name":"tags","in":"query","schema":{"type":"array","minItems":1,"items":{"type":"string"}}},{"name":"compact","in":"query","style":"form","explode":false,"schema":{"type":"array","minItems":1,"items":{"type":"string"}}}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}}}}},"responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}}}}}}}}`,
		`{}`,
	)
	var receivedPath, receivedQuery, receivedBody string
	tools, err := discoverFixture(t, spec, func(w http.ResponseWriter, r *http.Request) {
		receivedPath, receivedQuery = r.URL.EscapedPath(), r.URL.RawQuery
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	if err != nil {
		t.Fatal(err)
	}
	var result toolsy.Chunk
	// Act.
	err = tools[0].Execute(
		t.Context(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{
			ArgsJSON: []byte(
				`{"path":{"id":"a/b ?"},"query":{"id":7,"tags":["a,b","x y"],"compact":["a,b","c"]},"body":[{"id":"body"}]}`,
			),
		},
		func(chunk toolsy.Chunk) error { result = chunk; return nil },
	)
	// Assert: evidence is the request received by an actual HTTP server.
	if err != nil {
		t.Fatal(err)
	}
	if receivedPath != "/items/a%2Fb%20%3F" || receivedQuery != "compact=a%2Cb,c&id=7&tags=a%2Cb&tags=x%20y" ||
		receivedBody != `[{"id":"body"}]` {
		t.Fatalf("path=%s query=%s body=%s", receivedPath, receivedQuery, receivedBody)
	}
	if result.MimeType != toolsy.MimeTypeJSON || string(result.Data) != `{"ok":true}` {
		t.Fatalf("result=%+v", result)
	}
	for _, args := range []string{`{"path":{"id":"x"},"query":{"id":"wrong"},"body":[]}`, `{"path":{"id":"x"},"query":{"id":1}}`, `{"path":{"id":"x"},"query":{"id":1},"body":[],"id":1}`} {
		if err := tools[0].Execute(
			t.Context(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(args)},
			func(toolsy.Chunk) error { return nil },
		); err == nil {
			t.Fatalf("accepted %s", args)
		}
	}
}

func TestUnsupportedContractsFailBeforePublication(t *testing.T) {
	for _, parameter := range []string{
		`{"name":"x","in":"header","schema":{"type":"string"}}`,
		`{"name":"x","in":"cookie","schema":{"type":"string"}}`,
		`{"name":"x","in":"query","style":"deepObject","schema":{"type":"object"}}`,
		`{"name":"x","in":"query","schema":{"type":"string","format":"uuid"}}`,
		`{"name":"x","in":"query","schema":{"type":"string","readOnly":true}}`,
	} {
		spec := fixtureSpec(
			`{"/items":{"get":{"parameters":[`+parameter+`],"responses":{"200":{"description":"ok"}}}}}`,
			`{}`,
		)
		tools, err := discoverFixture(t, spec, nil)
		var unsupportedError *UnsupportedError
		if !errors.As(err, &unsupportedError) || len(tools) != 0 {
			t.Fatalf("parameter=%s tools=%v err=%v", parameter, tools, err)
		}
	}
	spec31 := strings.Replace(
		fixtureSpec(`{"/items":{"get":{"responses":{"200":{"description":"ok"}}}}}`, `{}`),
		"3.0.3",
		"3.1.0",
		1,
	)
	_, err := discoverFixture(t, spec31, nil)
	if _, ok := errors.AsType[*UnsupportedError](err); !ok {
		t.Fatalf("3.1 error=%v", err)
	}
}

func TestRecursiveDiscoveryBoundedSubprocess(t *testing.T) {
	const marker = "TOOLSY_OPENAPI_RECURSION"
	if os.Getenv(marker) == "1" {
		spec := fixtureSpec(
			`{"/nodes":{"post":{"requestBody":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Node"}}}},"responses":{"200":{"description":"ok"}}}}}`,
			`{"schemas":{"Node":{"type":"object","properties":{"child":{"$ref":"#/components/schemas/Node"}}}}}`,
		)
		tools, err := discoverFixture(t, spec, nil)
		var unsupportedError *UnsupportedError
		if !errors.As(err, &unsupportedError) || len(tools) > 0 {
			t.Fatalf("tools=%v err=%v", tools, err)
		}
		return
	}
	// Arrange / Act: isolate stack safety and enforce a hard deadline.
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRecursiveDiscoveryBoundedSubprocess$")
	command.Env = append(os.Environ(), marker+"=1")
	output, err := command.CombinedOutput()
	// Assert.
	if err != nil {
		t.Fatalf("recursive discovery failed: %v %s", err, output)
	}
}

func TestRepeatedReferencesAndStableNaming(t *testing.T) {
	component := `{"schemas":{"Value":{"type":"string","pattern":"^x"}}}`
	operation := `{"post":{"operationId":"same name","requestBody":{"content":{"application/json":{"schema":{"type":"object","properties":{"a":{"$ref":"#/components/schemas/Value"},"b":{"$ref":"#/components/schemas/Value"}}}}}},"responses":{"200":{"description":"ok"}}}}`
	other := strings.Replace(operation, "same name", "same$name", 1)
	specs := []string{
		fixtureSpec(`{"/b":`+other+`,"/a":`+operation+`}`, component),
		fixtureSpec(`{"/a":`+operation+`,"/b":`+other+`}`, component),
	}
	var baseline []toolsy.ToolManifest
	for _, spec := range specs {
		tools, err := discoverFixture(t, spec, nil)
		if err != nil {
			t.Fatal(err)
		}
		manifests := []toolsy.ToolManifest{}
		for _, tool := range tools {
			manifests = append(manifests, tool.Manifest())
		}
		if baseline == nil {
			baseline = manifests
		} else if !reflect.DeepEqual(baseline, manifests) {
			t.Fatalf("unstable manifests %v %v", baseline, manifests)
		}
	}
	if len(baseline) != 2 || baseline[0].Name == baseline[1].Name {
		t.Fatalf("names=%v", baseline)
	}
}

func TestServerPrecedenceAndRelativeURL(t *testing.T) {
	for _, fixture := range []struct{ name, root, item, operation, want string }{
		{"root", `"servers":[{"url":"/root"}],`, "", "", "/root/items"},
		{"path", `"servers":[{"url":"/root"}],`, `"servers":[{"url":"/path"}],`, "", "/path/items"},
		{"operation", `"servers":[{"url":"/root"}],`, `"servers":[{"url":"/path"}],`, `"servers":[{"url":"/op/{version}","variables":{"version":{"default":"v1"}}}],`, "/op/v1/items"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			spec := `{"openapi":"3.0.3","info":{"title":"fixture","version":"1"},` + fixture.root + `"paths":{"/items":{` + fixture.item + `"get":{` + fixture.operation + `"responses":{"200":{"description":"ok"}}}}}}`
			var received string
			tools, err := discoverFixture(
				t,
				spec,
				func(w http.ResponseWriter, r *http.Request) { received = r.URL.Path; w.WriteHeader(http.StatusOK) },
			)
			if err != nil {
				t.Fatal(err)
			}
			err = tools[0].Execute(
				t.Context(),
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
				func(toolsy.Chunk) error { return nil },
			)
			if err != nil || received != fixture.want {
				t.Fatalf("received=%s want=%s err=%v", received, fixture.want, err)
			}
		})
	}
}

func TestNamingNaturalHashCollision(t *testing.T) {
	identity := sha256.Sum256([]byte("get /c"))
	natural := fmt.Sprintf("x_%x", identity[:6])
	used := make(map[string]bool)
	names := []string{
		toolNameFromOperation(natural, "get", "/a", used),
		toolNameFromOperation("x", "get", "/b", used),
		toolNameFromOperation("x", "get", "/c", used),
	}
	if names[0] == names[1] || names[0] == names[2] || names[1] == names[2] {
		t.Fatalf("duplicate names=%v", names)
	}
}

func TestPathExtensionDiscovery(t *testing.T) {
	spec := fixtureSpec(`{"x-meta":"hello","/items":{"get":{"responses":{"200":{"description":"ok"}}}}}`, `{}`)
	tools, err := discoverFixture(t, spec, nil)
	if err != nil || len(tools) != 1 {
		t.Fatalf("tools=%v error=%v", tools, err)
	}
}
func TestSuccessfulResponseContract(t *testing.T) {
	for _, fixture := range []struct {
		name, body, contentType string
		status                  int
		valid                   bool
	}{
		{"valid", `{"count":2}`, "application/json", 200, true},
		{"invalid-constraint", `{"count":0}`, "application/json", 200, false},
		{"invalid-json", `{"count":`, "application/json", 200, false},
		{"missing-json-body", "", "application/json", 200, false},
		{"wrong-media-type", `{"count":2}`, "text/plain", 200, false},
		{"undeclared-status", `{"count":2}`, "application/json", 201, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			spec := fixtureSpec(
				`{"/items":{"get":{"responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"type":"object","properties":{"count":{"type":"integer","minimum":1}},"required":["count"],"additionalProperties":false}}}}}}}}`,
				`{}`,
			)
			tools, err := discoverFixture(t, spec, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", fixture.contentType)
				w.WriteHeader(fixture.status)
				_, _ = io.WriteString(w, fixture.body)
			})
			if err != nil {
				t.Fatal(err)
			}
			delivered := 0
			// Act.
			err = tools[0].Execute(
				t.Context(),
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
				func(toolsy.Chunk) error { delivered++; return nil },
			)
			// Assert.
			if (err == nil) != fixture.valid || (fixture.valid && delivered != 1) ||
				(!fixture.valid && delivered != 0) {
				t.Fatalf("err=%v delivered=%d", err, delivered)
			}
		})
	}
}

func TestSimpleExpansionEncodesReservedBytes(t *testing.T) {
	spec := fixtureSpec(
		`{"/items/{id}":{"get":{"parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"ok"}}}}}`,
		`{}`,
	)
	var received string
	tools, err := discoverFixture(
		t,
		spec,
		func(w http.ResponseWriter, r *http.Request) { received = r.RequestURI; w.WriteHeader(http.StatusOK) },
	)
	if err != nil {
		t.Fatal(err)
	}
	err = tools[0].Execute(
		t.Context(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"path":{"id":"a:b@c+d=e$f&g"}}`)},
		func(toolsy.Chunk) error { return nil },
	)
	if err != nil || received != "/items/a%3Ab%40c%2Bd%3De%24f%26g" {
		t.Fatalf("URI=%s error=%v", received, err)
	}
}
func TestRepeatedLiteralExpansionBounded(t *testing.T) {
	enum := make([]any, 1000)
	for i := range enum {
		enum[i] = fmt.Sprintf("value%d", i)
	}
	properties := make(map[string]any)
	for i := range 10 {
		properties[fmt.Sprintf("p%d", i)] = map[string]any{"$ref": "#/components/schemas/Large"}
	}
	source := map[string]any{
		"components": map[string]any{
			"schemas": map[string]any{"Large": map[string]any{"type": "string", "enum": enum}},
		},
	}
	projection := &projector{source: source, active: make(map[string]bool)}
	_, err := projection.schema(map[string]any{"type": "object", "properties": properties}, 0)
	if _, ok := errors.AsType[*UnsupportedError](err); !ok {
		t.Fatalf("expected bounded rejection, got %v", err)
	}
}

func TestDiscoveryNamingNaturalHashCollision(t *testing.T) {
	identity := sha256.Sum256([]byte("get /c"))
	natural := fmt.Sprintf("x_%x", identity[:6])
	operation := func(id string) string {
		return fmt.Sprintf(`{"get":{"operationId":%q,"responses":{"200":{"description":"ok"}}}}`, id)
	}
	spec := fixtureSpec(`{"/a":`+operation(natural)+`,"/b":`+operation("X")+`,"/c":`+operation("x")+`}`, `{}`)
	tools, err := discoverFixture(t, spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		name := tool.Manifest().Name
		if names[name] {
			t.Fatalf("duplicate name=%s", name)
		}
		names[name] = true
	}
	if len(names) != 3 {
		t.Fatalf("names=%v", names)
	}
}

func TestEmptyDeclaredResponseIgnoresMediaType(t *testing.T) {
	for _, status := range []string{"200", "204"} {
		spec := fixtureSpec(`{"/items":{"get":{"responses":{"`+status+`":{"description":"empty"}}}}}`, `{}`)
		tools, err := discoverFixture(t, spec, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if status == "204" {
				w.WriteHeader(http.StatusNoContent)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		var result toolsy.Chunk
		err = tools[0].Execute(
			t.Context(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
			func(chunk toolsy.Chunk) error { result = chunk; return nil },
		)
		if err != nil || !result.EmptyResult || result.MimeType != "" {
			t.Fatalf("result=%+v error=%v", result, err)
		}
	}
}
func TestEmptyQueryArraysUseUndefinedExpansion(t *testing.T) {
	spec := fixtureSpec(
		`{"/items":{"get":{"parameters":[{"name":"a","in":"query","required":true,"schema":{"type":"array","items":{"type":"string"}}},{"name":"b","in":"query","explode":false,"schema":{"type":"array","items":{"type":"string"}}}],"responses":{"200":{"description":"empty"}}}}}`,
		`{}`,
	)
	var received string
	tools, err := discoverFixture(
		t,
		spec,
		func(w http.ResponseWriter, r *http.Request) { received = r.RequestURI; w.WriteHeader(http.StatusOK) },
	)
	if err != nil {
		t.Fatal(err)
	}
	err = tools[0].Execute(
		t.Context(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":{"a":[],"b":[]}}`)},
		func(toolsy.Chunk) error { return nil },
	)
	if err != nil || received != "/items" {
		t.Fatalf("URI=%s error=%v", received, err)
	}
}
func TestRequestBodyUnsupportedMethods(t *testing.T) {
	for _, method := range []string{"get", "head", "delete", "options"} {
		spec := fixtureSpec(
			`{"/items":{"`+method+`":{"requestBody":{"content":{"application/json":{"schema":{"type":"string"}}}},"responses":{"200":{"description":"empty"}}}}}`,
			`{}`,
		)
		tools, err := discoverFixture(t, spec, nil)
		if _, ok := errors.AsType[*UnsupportedError](err); !ok || len(tools) != 0 {
			t.Fatalf("method=%s tools=%v err=%v", method, tools, err)
		}
	}
}

func TestTraceDiscoveryRejected(t *testing.T) {
	spec := fixtureSpec(`{"/items":{"trace":{"responses":{"200":{"description":"empty"}}}}}`, `{}`)
	tools, err := discoverFixture(t, spec, nil)
	if _, ok := errors.AsType[*UnsupportedError](err); !ok || len(tools) > 0 {
		t.Fatalf("tools=%v err=%v", tools, err)
	}
	_, err = ParseURL(t.Context(), "http://127.0.0.1/unused", Options{AllowedMethods: []string{http.MethodTrace}})
	if _, ok := errors.AsType[*UnsupportedError](err); !ok {
		t.Fatalf("expected selectedmethod rejection, got %v", err)
	}
}
