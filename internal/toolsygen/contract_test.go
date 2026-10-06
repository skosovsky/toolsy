package toolsygen

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratorRejectsUnknownAndInapplicableKeywords(t *testing.T) {
	t.Parallel()
	for _, property := range []string{
		`{"type":"string","description":"x","minimum":0}`,
		`{"type":"integer","description":"x","x-ignored":true}`,
		`{"type":"array","description":"x","items":{"type":"string","contentEncoding":"base64"}}`,
		`{"type":["string","integer"],"description":"x"}`,
		`{"type":"object","description":"x","properties":{}}`,
	} {
		t.Run(property, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "invalid.json")
			writeFile(
				t,
				path,
				`{"name":"invalid","description":"invalid","parameters":{"type":"object","properties":{"value":`+property+`}}}`,
			)
			_, err := Generate(context.Background(), Config{Inputs: []string{path}})
			if err == nil ||
				!strings.Contains(err.Error(), "unsupported") && !strings.Contains(err.Error(), "not supported") {
				t.Fatalf("expected explicit unsupported error, got %v", err)
			}
		})
	}
}

func TestGeneratedSourceSchemaExecutionParity(t *testing.T) {
	repoRoot := findRepoRoot(t)
	dir := t.TempDir()
	repoLink := filepath.Join(t.TempDir(), "toolsy-repo")
	if err := os.Symlink(repoRoot, repoLink); err != nil {
		t.Fatal(err)
	}
	writeFile(
		t,
		filepath.Join(dir, "go.mod"),
		"module fixture\n\ngo 1.27.1\n\nrequire github.com/skosovsky/toolsy v0.0.0\nreplace github.com/skosovsky/toolsy => "+repoLink+"\n",
	)
	for _, name := range []string{"loose", "strict"} {
		constraints := ""
		arrayConstraints := ""
		if name == "strict" {
			constraints = `,"minLength":1`
			arrayConstraints = `,"minItems":1`
		}
		writeFile(
			t,
			filepath.Join(dir, name+".json"),
			`{"name":"`+name+`","description":"parity fixture","parameters":{"type":"object","properties":{"text":{"type":"string","description":"text"`+constraints+`},"tags":{"type":"array","description":"tags","items":{"type":"string"}`+arrayConstraints+`},"count":{"type":"integer","description":"count"},"active":{"type":"boolean","description":"active"},"nullable":{"type":["string","null"],"description":"nullable"}},"required":["text","tags","count","active"]}}`,
		)
	}
	writeFile(t, filepath.Join(dir, "fixture.go"), "package fixture\n")
	writeFile(
		t,
		filepath.Join(dir, "nullable_required.json"),
		`{"name":"nullable_required","description":"required null fixture","parameters":{"type":"object","properties":{"value":{"description":"nullable value","type":["string","null"]}},"required":["value"]}}`,
	)
	writeFile(t, filepath.Join(dir, "parity_test.go"), generatedParityTest)
	_, err := Generate(
		context.Background(),
		Config{
			Inputs: []string{
				filepath.Join(dir, "loose.json"),
				filepath.Join(dir, "strict.json"),
				filepath.Join(dir, "nullable_required.json"),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(t.TempDir(), "gocache")
	runGo(t, dir, cache, "mod", "tidy")
	runGo(t, dir, cache, "test", "-race", "./...")
}

const generatedParityTest = `package fixture
import("context";"encoding/json";"testing";"github.com/skosovsky/toolsy")
type handler struct { calls *int; nullable *string; count *string; raw *string }
func(h handler) Execute(_ context.Context, in LooseInput)(string,error){ *h.calls++; *h.nullable=string(in.Nullable); *h.count=in.Count.String(); *h.raw=string(in.RawJSON); return "ok",nil }
type nullHandler struct{}
func(nullHandler) Execute(_ context.Context, in NullableRequiredInput)(string,error){return string(in.Value),nil}
type strictHandler struct{}
func(strictHandler) Execute(context.Context, StrictInput)(string,error){return "ok",nil}
func TestParity(t *testing.T){
 nullableTool,err:=NewNullableRequiredTool(nullHandler{});if err!=nil{t.Fatal(err)}
 for _,tc:=range []struct{raw string;valid bool}{{"{}",false},{"{\"value\":null}",true},{"{\"value\":\"\"}",true}}{err:=nullableTool.Execute(context.Background(),toolsy.NewRunEnv(nil),toolsy.ToolInput{ArgsJSON:[]byte(tc.raw)},func(toolsy.Chunk)error{return nil});if (err==nil)!=tc.valid{t.Fatalf("required nullable %s: %v",tc.raw,err)}}
 calls:=0; nullable,count,raw:="","",""; tool,err:=NewLooseTool(handler{&calls,&nullable,&count,&raw});if err!=nil{t.Fatal(err)}
 strict,err:=NewStrictTool(strictHandler{});if err!=nil{t.Fatal(err)}
 for _,tc:=range []struct{raw string; valid bool; nullable string}{
 {` + "`" + `{"text":"","tags":[],"count":0,"active":false,"Nullable":"must remain unknown"}` + "`" + `,true,""},
 {` + "`" + `{"text":"","tags":[],"count":999999999999999999999999999999999,"active":false,"nullable":null,"extra":42}` + "`" + `,true,"null"},
 {` + "`" + `{"text":"","tags":[],"count":1.0,"active":false,"nullable":""}` + "`" + `,true,"\"\""},
 {` + "`" + `{"tags":[],"count":0,"active":false}` + "`" + `,false,""},
 {` + "`" + `{"text":null,"tags":[],"count":0,"active":false}` + "`" + `,false,""},
 {` + "`" + `{"text":"","tags":null,"count":0,"active":false}` + "`" + `,false,""},
 {` + "`" + `{"text":"","tags":[],"count":null,"active":false}` + "`" + `,false,""},
 {` + "`" + `{"text":"","tags":[],"count":0,"active":null}` + "`" + `,false,""},
 }{
 before:=calls;err:=tool.Execute(context.Background(),toolsy.NewRunEnv(nil),toolsy.ToolInput{ArgsJSON:[]byte(tc.raw)},func(toolsy.Chunk)error{return nil})
 if (err==nil)!=tc.valid{t.Fatalf("%s: %v",tc.raw,err)};if tc.valid{if calls!=before+1||nullable!=tc.nullable||raw!=tc.raw{t.Fatalf("lost representation: %d %q %q",calls,nullable,raw)};var obj map[string]json.RawMessage;if err:=json.Unmarshal([]byte(tc.raw),&obj);err!=nil{t.Fatal(err)};if count!=string(obj["count"]){t.Fatal("number changed")}}else if calls!=before{t.Fatal("invalid handler call")}
 }
 for _,raw:=range []string{` + "`" + `{"text":"","tags":["a"],"count":0,"active":false}` + "`" + `,` + "`" + `{"text":"a","tags":[],"count":0,"active":false}` + "`" + `}{if err:=strict.Execute(context.Background(),toolsy.NewRunEnv(nil),toolsy.ToolInput{ArgsJSON:[]byte(raw)},func(toolsy.Chunk)error{return nil});err==nil{t.Fatal("minimum constraint lost")}}
}
`

func TestYAMLManifestPrecisionAndBounds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, yaml string
		valid      bool
	}{
		{"large integer", "minimum: 999999999999999999999999999999999", true},
		{"hex integer", "minimum: 0xff", true},
		{"decimal", "minimum: .125", true},
		{"alias", "minimum: &value 1\nmaximum: *value", false},
		{"infinity", "minimum: .inf", false},
		{"duplicate", "minimum: 1\nminimum: 2", false},
		{"depth", "x: " + strings.Repeat("[", 130) + "0" + strings.Repeat("]", 130), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			value, err := decodeYAMLManifest([]byte(tc.yaml))
			if (err == nil) != tc.valid {
				t.Fatalf("decode %s: %v", tc.yaml, err)
			}
			if tc.name == "large integer" {
				if got := value.(map[string]any)["minimum"].(json.Number).String(); got != "999999999999999999999999999999999" {
					t.Fatalf("rounded number %s", got)
				}
			}
		})
	}
}

func TestGeneratorRejectsUnrepresentablePropertyNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"a,b", "a\\b", "a\nb", "a😀b", "raw_json"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			manifest := map[string]any{
				"name":        "invalid",
				"description": "invalid",
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{name: map[string]any{"type": "string", "description": "value"}},
				},
			}
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "invalid.json")
			writeFile(t, path, string(data))
			if _, err := Generate(context.Background(), Config{Inputs: []string{path}}); err == nil {
				t.Fatal("unrepresentable field published")
			}
		})
	}
}
