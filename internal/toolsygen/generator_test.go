package toolsygen

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

func testGen() *generator {
	return &generator{fs: newDefaultOSFacade(defaultMaxGeneratorFileBytes), maxFileBytes: defaultMaxGeneratorFileBytes}
}

// memDirEnt is a minimal [fs.DirEntry] for testing walkDir without a real directory tree.
type memDirEnt struct {
	name  string
	isDir bool
}

func (e memDirEnt) Name() string { return e.name }

func (e memDirEnt) IsDir() bool { return e.isDir }

func (e memDirEnt) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}

func (e memDirEnt) Info() (fs.FileInfo, error) { return nil, fs.ErrNotExist }

func TestWalkDirUsesInjectedReadDir(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "virt")
	sub := filepath.Join(root, "sub")
	tree := map[string][]os.DirEntry{
		root: {
			memDirEnt{name: "sub", isDir: true},
			memDirEnt{name: "m.yaml", isDir: false},
		},
		sub: {
			memDirEnt{name: "n.yaml", isDir: false},
		},
	}
	facade := newDefaultOSFacade(defaultMaxGeneratorFileBytes)
	facade.readDir = func(name string) ([]os.DirEntry, error) {
		ents, ok := tree[name]
		if !ok {
			return nil, &os.PathError{Op: "readdir", Path: name, Err: os.ErrNotExist}
		}
		return ents, nil
	}
	g := &generator{fs: facade}

	var walked []string
	err := g.walkDir(context.Background(), root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			walked = append(walked, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walkDir: %v", err)
	}
	sort.Strings(walked)
	want := []string{
		filepath.Join(root, "m.yaml"),
		filepath.Join(root, "sub", "n.yaml"),
	}
	if len(walked) != len(want) {
		t.Fatalf("got %v, want %v", walked, want)
	}
	for i := range want {
		if walked[i] != want[i] {
			t.Fatalf("walked[%d] = %q, want %q", i, walked[i], want[i])
		}
	}
}

func TestGenerateWithFSParityWithGenerate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	cfg := Config{Inputs: []string{dir}}
	r1, err1 := Generate(ctx, cfg)
	r2, err2 := generateWithFS(ctx, cfg, newDefaultOSFacade(defaultMaxGeneratorFileBytes))
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v, %v", err1, err2)
	}
	if len(r1.Files) != len(r2.Files) {
		t.Fatalf("result mismatch: %+v vs %+v", r1, r2)
	}
}

func TestDiscoverManifestsSortedAndSkipsHidden(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(
		t,
		filepath.Join(root, "b.yaml"),
		"name: b\ndescription: b\nparameters:\n  type: object\n  properties: {}\n",
	)
	writeFile(
		t,
		filepath.Join(root, "nested", "a.json"),
		`{"name":"a","description":"a","parameters":{"type":"object","properties":{}}}`,
	)
	writeFile(
		t,
		filepath.Join(root, ".git", "ignored.yaml"),
		"name: ignored\ndescription: ignored\nparameters:\n  type: object\n  properties: {}\n",
	)
	writeFile(
		t,
		filepath.Join(root, "vendor", "ignored.yml"),
		"name: ignored\ndescription: ignored\nparameters:\n  type: object\n  properties: {}\n",
	)

	paths, err := testGen().discoverManifests(context.Background(), []string{root})
	if err != nil {
		t.Fatalf("discover manifests: %v", err)
	}

	want := []string{
		filepath.Join(root, "b.yaml"),
		filepath.Join(root, "nested", "a.json"),
	}
	sort.Strings(want)
	if len(paths) != len(want) {
		t.Fatalf("discover manifests length = %d, want %d (%v)", len(paths), len(want), paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("discover manifests[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestInferPackageNameMixedPackages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), "package alpha\n")
	writeFile(t, filepath.Join(dir, "b.go"), "package beta\n")

	_, err := testGen().inferPackageName(context.Background(), dir)
	if err == nil || !strings.Contains(err.Error(), "mixed package names") {
		t.Fatalf("inferPackageName error = %v, want mixed package names", err)
	}
}

func TestLoadManifestValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		manifest   string
		wantSubstr string
	}{
		{
			name: "missing property description",
			manifest: `
name: "book_appointment"
description: "desc"
parameters:
  type: object
  properties:
    doctor_id:
      type: string
  required: ["doctor_id"]
`,
			wantSubstr: "parameters.properties.doctor_id: description: required string",
		},
		{
			name: "nested object",
			manifest: `
name: "book_appointment"
description: "desc"
parameters:
  type: object
  properties:
    payload:
      type: object
      description: "payload"
      properties:
        nested:
          type: string
          description: "nested"
`,
			wantSubstr: "nested objects are not supported",
		},
		{
			name: "unsupported anyOf",
			manifest: `
name: "book_appointment"
description: "desc"
parameters:
  type: object
  properties:
    doctor_id:
      description: "doctor"
      anyOf:
        - type: string
        - type: integer
`,
			wantSubstr: "anyOf",
		},
		{
			name: "unsupported oneOf",
			manifest: `
name: "t"
description: "desc"
parameters:
  type: object
  properties:
    x:
      description: "x"
      oneOf:
        - type: string
        - type: integer
`,
			wantSubstr: "oneOf",
		},
		{
			name: "unsupported allOf",
			manifest: `
name: "t"
description: "desc"
parameters:
  type: object
  properties:
    x:
      description: "x"
      allOf:
        - type: string
`,
			wantSubstr: "allOf",
		},
		{
			name: "unsupported not",
			manifest: `
name: "t"
description: "desc"
parameters:
  type: object
  properties:
    x:
      description: "x"
      not:
        type: string
`,
			wantSubstr: "not",
		},
		{
			name: "unsupported ref",
			manifest: `
name: "t"
description: "desc"
parameters:
  type: object
  properties:
    x:
      description: "x"
      $ref: "#/definitions/Foo"
`,
			wantSubstr: "$ref",
		},
		{
			name: "unsupported patternProperties",
			manifest: `
name: "t"
description: "desc"
parameters:
  type: object
  properties:
    x:
      description: "x"
      type: string
  patternProperties:
    "^x":
      type: string
`,
			wantSubstr: "patternProperties",
		},
		{
			name: "nested array",
			manifest: `
name: "t"
description: "desc"
parameters:
  type: object
  properties:
    tags:
      type: array
      description: "tags"
      items:
        type: array
        items:
          type: string
          description: "inner"
`,
			wantSubstr: "nested arrays are not supported",
		},
		{
			name: "non object root",
			manifest: `
name: "book_appointment"
description: "desc"
parameters:
  type: string
`,
			wantSubstr: `parameters.type: expected "object", got "string"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "tool.yaml")
			writeFile(t, path, strings.TrimSpace(tt.manifest))

			_, err := testGen().loadManifest(context.Background(), path)
			if err == nil || !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("loadManifest error = %v, want substring %q", err, tt.wantSubstr)
			}
		})
	}
}

func TestRenderManifestImportsAndPackageFallback(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "apptools")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "book_appointment.yaml")
	writeFile(t, path, `
name: "book_appointment"
description: "Book an appointment"
stream: true
parameters:
  type: object
  properties:
    doctor_id:
      type: string
      description: "Doctor id"
    slot_time:
      type: string
      format: date-time
      description: "Slot time"
  required: ["doctor_id", "slot_time"]
`)

	m, err := testGen().loadManifest(context.Background(), path)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	if m.PackageName != "apptools" {
		t.Fatalf("package name = %q, want apptools", m.PackageName)
	}

	rendered, err := renderManifest(m)
	if err != nil {
		t.Fatalf("renderManifest: %v", err)
	}
	code := string(rendered)

	for _, want := range []string{
		`"iter"`,
		"package apptools",
		"type BookAppointmentStreamHandler interface",
		"DoctorID string",
		`return toolsy.NewSchemaError("Validation failed: invalid JSON format or type mismatch"`,
		"return proxy, nil",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("generated code missing %q:\n%s", want, code)
		}
	}
	if strings.Contains(code, `validate:"required"`) {
		t.Fatalf("generated code must not use validate struct tags:\n%s", code)
	}
	if !strings.Contains(code, "for part, err := range handler.ExecuteStream") {
		t.Fatalf("stream tool must propagate ExecuteStream errors:\n%s", code)
	}
}

func TestRenderManifestRequiredPrimitiveAndArrayChecks(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "types")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "typed.yaml")
	writeFile(t, path, `
name: "typed_tool"
description: "Typed tool"
parameters:
  type: object
  properties:
    count:
      type: integer
      description: "Count"
    active:
      type: boolean
      description: "Active flag"
    tags:
      type: array
      description: "Tags"
      items:
        type: string
        description: "Tag"
  required: ["count", "active", "tags"]
`)
	m, err := testGen().loadManifest(context.Background(), path)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	code, err := renderManifest(m)
	if err != nil {
		t.Fatalf("renderManifest: %v", err)
	}
	s := string(code)
	for _, want := range []string{
		"*json.Number",
		"*bool",
		"*[]string",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("generated code missing %q:\n%s", want, s)
		}
	}
}

func TestRenderManifestNonStreamNoAsyncWrapper(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "tools")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "echo.yaml")
	writeFile(t, path, `
name: "echo"
description: "Echo"
parameters:
  type: object
  properties:
    text:
      type: string
      description: "Text"
  required: ["text"]
`)
	m, err := testGen().loadManifest(context.Background(), path)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	code, err := renderManifest(m)
	if err != nil {
		t.Fatalf("renderManifest: %v", err)
	}
	s := string(code)
	if strings.Contains(s, "toolsy.AsAsyncTool") {
		t.Fatalf("non-stream tool must not use AsAsyncTool:\n%s", s)
	}
	if !strings.Contains(s, "return proxy, nil") {
		t.Fatalf("non-stream tool must return proxy directly:\n%s", s)
	}
}

func TestValidateManifestSetDetectsCollisions(t *testing.T) {
	t.Parallel()

	dirOne := filepath.Join(t.TempDir(), "one")
	dirTwo := filepath.Join(t.TempDir(), "two")
	if err := os.MkdirAll(dirOne, 0o750); err != nil {
		t.Fatalf("mkdir one: %v", err)
	}
	if err := os.MkdirAll(dirTwo, 0o750); err != nil {
		t.Fatalf("mkdir two: %v", err)
	}

	errs := testGen().validateManifestSet(context.Background(), []*manifest{
		{
			Path:          filepath.Join(dirOne, "tool.yaml"),
			Dir:           dirOne,
			Name:          "book_appointment",
			OutputPath:    filepath.Join(dirOne, "book_appointment_gen.go"),
			InputTypeName: "BookAppointmentInput",
			HandlerName:   "BookAppointmentHandler",
			FactoryName:   "NewBookAppointmentTool",
		},
		{
			Path:          filepath.Join(dirTwo, "tool.yaml"),
			Dir:           dirTwo,
			Name:          "book_appointment",
			OutputPath:    filepath.Join(dirOne, "book_appointment_gen.go"),
			InputTypeName: "BookAppointmentInput",
			HandlerName:   "BookAppointmentHandler",
			FactoryName:   "NewBookAppointmentTool",
		},
	})

	if len(errs) < 2 {
		t.Fatalf("validateManifestSet errors = %d, want at least 2", len(errs))
	}
}

func TestGenerateDetectsExistingSymbolCollision(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "apptools")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(dir, "existing.go"), "package apptools\n\ntype BookAppointmentInput struct{}\n")
	writeFile(t, filepath.Join(dir, "book_appointment.yaml"), `
name: "book_appointment"
description: "Book an appointment"
parameters:
  type: object
  properties:
    doctor_id:
      type: string
      description: "Doctor id"
  required: ["doctor_id"]
`)

	_, err := Generate(context.Background(), Config{Inputs: []string{dir}})
	if err == nil || !strings.Contains(err.Error(), "collides with existing symbol") {
		t.Fatalf("Generate error = %v, want existing symbol collision", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "book_appointment_gen.go")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("generated file stat err = %v, want not exists", statErr)
	}
}

func TestCommitFilesWithRollbackOnFailure(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first_gen.go")
	secondPath := filepath.Join(dir, "second_gen.go")
	writeFile(t, firstPath, "original")

	fs := newDefaultOSFacade(defaultMaxGeneratorFileBytes)
	originalRename := fs.rename
	renameCalls := 0
	fs.rename = func(oldPath, newPath string) error {
		renameCalls++
		if renameCalls == 3 {
			return errors.New("forced rename failure")
		}
		return originalRename(oldPath, newPath)
	}

	g := &generator{fs: fs}
	err := g.commitFilesWithRollback(context.Background(), []generatedFile{
		{Path: firstPath, Content: []byte("updated")},
		{Path: secondPath, Content: []byte("created")},
	}, 0o644)
	if err == nil || !strings.Contains(err.Error(), "forced rename failure") {
		t.Fatalf("commitFilesWithRollback error = %v, want forced rename failure", err)
	}

	// #nosec G304 -- firstPath is created inside this test workspace.
	data, readErr := os.ReadFile(firstPath)
	if readErr != nil {
		t.Fatalf("read restored first file: %v", readErr)
	}
	if string(data) != "original" {
		t.Fatalf("first file = %q, want original", string(data))
	}
	if _, statErr := os.Stat(secondPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("second file stat err = %v, want not exists", statErr)
	}
}

func TestToolsyGenEndToEnd(t *testing.T) {
	repoRoot := findRepoRoot(t)
	moduleDir := t.TempDir()
	goCache := filepath.Join(t.TempDir(), "gocache")
	repoLink := filepath.Join(t.TempDir(), "toolsy-repo")
	if err := os.Symlink(repoRoot, repoLink); err != nil {
		t.Fatalf("symlink repo root: %v", err)
	}

	writeFile(
		t,
		filepath.Join(moduleDir, "go.mod"),
		"module fixture\n\ngo 1.26.3\n\nrequire github.com/skosovsky/toolsy v0.0.0\n\nreplace github.com/skosovsky/toolsy => "+repoLink+"\n",
	)

	appDir := filepath.Join(moduleDir, "apptools")
	streamDir := filepath.Join(moduleDir, "streamtools")
	if err := os.MkdirAll(appDir, 0o750); err != nil {
		t.Fatalf("mkdir apptools: %v", err)
	}
	if err := os.MkdirAll(streamDir, 0o750); err != nil {
		t.Fatalf("mkdir streamtools: %v", err)
	}

	writeFile(t, filepath.Join(appDir, "book_appointment.yaml"), `
name: "book_appointment"
description: "Book an appointment"
parameters:
  type: object
  properties:
    doctor_id:
      type: string
      description: "Doctor id"
    slot_time:
      type: string
      format: date-time
      description: "Slot time"
  required: ["doctor_id", "slot_time"]
`)
	writeFile(t, filepath.Join(appDir, "book_appointment_test.go"), `
package apptools

import (
	"context"
	"errors"
	"testing"

	"github.com/skosovsky/toolsy"
)

type bookHandler struct{}

func (bookHandler) Execute(_ context.Context, input BookAppointmentInput) (string, error) {
	return input.DoctorID + "|" + input.SlotTime, nil
}

func TestGeneratedNonStreamTool(t *testing.T) {
	tool, err := NewBookAppointmentTool(bookHandler{})
	if err != nil {
		t.Fatalf("NewBookAppointmentTool: %v", err)
	}

	var got string
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte("{\"doctor_id\":\"d1\",\"slot_time\":\"2026-03-18T09:00:00Z\"}")},
		func(c toolsy.Chunk) error {
		got = string(c.Data)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute valid payload: %v", err)
	}
	if got != "d1|2026-03-18T09:00:00Z" {
		t.Fatalf("result = %q", got)
	}

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte("{}")},
		func(toolsy.Chunk) error { return nil },
	)
	if te, ok := toolsy.AsToolError(err); err == nil || !ok || !toolsy.ClientCorrectable(te.Code) {
		t.Fatalf("missing required field error = %v, want client error", err)
	}

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte("{\"doctor_id\":\"\",\"slot_time\":\"2026-03-18T09:00:00Z\"}")},
		func(toolsy.Chunk) error { return nil },
	)
	if err != nil { t.Fatalf("empty required string must be accepted: %v", err) }
 var ce *toolsy.ToolError

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte("{not-json") },
		func(toolsy.Chunk) error { return nil },
	)
	if te, ok := toolsy.AsToolError(err); err == nil || !ok || !toolsy.ClientCorrectable(te.Code) {
		t.Fatalf("invalid json error = %v, want client error", err)
	}
	if !errors.As(err, &ce) {
		t.Fatalf("invalid json error type = %T", err)
	}
	// NewProxyTool validates ArgsJSON before the generated handler; malformed JSON uses wrapJSONParseError.
	if ce.Reason != "invalid JSON" {
		t.Fatalf("invalid json reason = %q, want invalid JSON", ce.Reason)
	}
	if ce.Unwrap() == nil {
		t.Fatal("invalid json unwrap is nil")
	}

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte("{\"doctor_id\":1,\"slot_time\":\"2026-03-18T09:00:00Z\"}") },
		func(toolsy.Chunk) error { return nil },
	)
	if te, ok := toolsy.AsToolError(err); err == nil || !ok || !toolsy.ClientCorrectable(te.Code) {
		t.Fatalf("type mismatch error = %v, want client error", err)
	}
	if !errors.As(err, &ce) {
		t.Fatalf("type mismatch error type = %T", err)
	}
	if !errors.Is(ce.Err, toolsy.ErrValidation) {
		t.Fatalf("type mismatch unwrap = %v, want ErrValidation", ce.Err)
	}
}
`)

	writeFile(t, filepath.Join(streamDir, "progress_demo.yaml"), `
name: "progress_demo"
description: "Demo streaming"
stream: true
parameters:
  type: object
  properties:
    count:
      type: integer
      description: "How many items to emit"
  required: ["count"]
`)
	writeFile(t, filepath.Join(streamDir, "progress_demo_test.go"), `
package streamtools

import (
 "context"
 "encoding/json"
 "errors"
 "iter"
 "sync/atomic"
 "testing"
 "time"
 "github.com/skosovsky/toolsy"
)
var invoked atomic.Int32
var boom=errors.New("handler failed")
type streamHandler struct{}
func(streamHandler)ExecuteStream(ctx context.Context,input ProgressDemoInput)iter.Seq2[string,error]{
 return func(yield func(string,error)bool){
  invoked.Add(1)
  n,err:=input.Count.Int64();if err!=nil{yield("",err);return}
  switch n{
  case 0:return
  case 1:yield("done",nil)
  case 3:for _,s:=range []string{"step1","step2","done"}{if !yield(s,nil){return}}
  case -1:if yield("step",nil){yield("",boom)}
  case -2:yield("step",nil);<-ctx.Done();yield("",ctx.Err())
  }
 }
}
func TestGeneratedSynchronousStream(t *testing.T){
 for _,n:=range []int{0,1,3,-1}{
  // Arrange.
  base,err:=NewProgressDemoTool(streamHandler{});if err!=nil{t.Fatal(err)}
  raw,_:=json.Marshal(map[string]int{"count":n});var chunks []toolsy.Chunk
  // Act.
  err=base.Execute(t.Context(),nil,toolsy.ToolInput{ArgsJSON:raw},func(c toolsy.Chunk)error{chunks=append(chunks,c);return nil})
  // Assert: accepted is not substituted for progress/result or handler error.
  if n==-1{if !errors.Is(err,boom)||len(chunks)!=1||chunks[0].Event!=toolsy.EventProgress{t.Fatalf("failure: %v %#v",err,chunks)};continue}
  if err!=nil{t.Fatal(err)}
  want:=1;if n==3{want=3};if len(chunks)!=want||chunks[len(chunks)-1].Event!=toolsy.EventResult{t.Fatalf("terminal: %#v",chunks)}
  if n>0&&string(chunks[len(chunks)-1].Data)!="done"{t.Fatalf("data: %#v",chunks)}
 }
}
func TestGeneratedInputCancellationAndYieldError(t *testing.T){
 // Arrange.
 base,err:=NewProgressDemoTool(streamHandler{});if err!=nil{t.Fatal(err)}
 before:=invoked.Load()
 // Act/Assert: invalid args reject before dispatch.
 err=base.Execute(t.Context(),nil,toolsy.ToolInput{ArgsJSON:[]byte("{}")},func(toolsy.Chunk)error{t.Fatal("invalid output");return nil})
 if err==nil||invoked.Load()!=before{t.Fatalf("validation: %v",err)}
 // Act/Assert: synchronous caller cancellation and consumer stop propagate causes.
 ctx,cancel:=context.WithTimeout(t.Context(),20*time.Millisecond);defer cancel()
 err=base.Execute(ctx,nil,toolsy.ToolInput{ArgsJSON:[]byte("{\"count\":-2}")},func(toolsy.Chunk)error{return nil})
 if !errors.Is(err,context.DeadlineExceeded){t.Fatalf("cancel: %v",err)}
 stop:=errors.New("consumer stopped")
 err=base.Execute(t.Context(),nil,toolsy.ToolInput{ArgsJSON:[]byte("{\"count\":3}")},func(toolsy.Chunk)error{return stop})
 if !errors.Is(err,stop){t.Fatalf("consumer: %v",err)}
}
func TestExplicitAsyncCompletion(t *testing.T){
 for _,raw:=range []string{"{\"count\":1}","{\"count\":-1}","{\"count\":-2}","{}"}{
  // Arrange: host owns timeout, collection and completion, independently of generated factory.
  base,err:=NewProgressDemoTool(streamHandler{});if err!=nil{t.Fatal(err)}
  type completion struct{chunks []toolsy.Chunk;err error}
  done:=make(chan completion,1)
  async:=toolsy.AsAsyncTool(base,toolsy.WithBackgroundTimeout(250*time.Millisecond),toolsy.WithMaxCollectedChunks(10),
   toolsy.WithOnComplete(func(_ context.Context,_ string,chunks []toolsy.Chunk,err error){done<-completion{chunks,err}}))
  var accepted toolsy.AsyncAccepted
  // Act.
  err=async.Execute(t.Context(),nil,toolsy.ToolInput{ArgsJSON:[]byte(raw)},func(c toolsy.Chunk)error{return json.Unmarshal(c.Data,&accepted)})
  // Assert: caller sees scheduling; terminal/error comes through completion hook.
  if err!=nil||accepted.Status!="accepted"||accepted.TaskID==""{t.Fatalf("accept: %v %#v",err,accepted)}
  select{
  case got:=<-done:
   if raw=="{\"count\":1}"{if got.err!=nil||len(got.chunks)!=1||got.chunks[0].Event!=toolsy.EventResult{t.Fatalf("completion: %#v",got)}}else if got.err==nil{t.Fatalf("missing completion error for %s",raw)}
   if raw=="{\"count\":-1}"&&!errors.Is(got.err,boom){t.Fatalf("error cause: %v",got.err)}
   if raw=="{\"count\":-2}"&&!errors.Is(got.err,context.DeadlineExceeded){t.Fatalf("timeout cause: %v",got.err)}
  case <-time.After(time.Second):t.Fatal("no async completion")
  }
 }
}

`)

	runGo(t, repoRoot, goCache, "run", "./cmd/toolsy-gen", moduleDir)

	if _, err := os.Stat(filepath.Join(appDir, "book_appointment_gen.go")); err != nil {
		t.Fatalf("generated app tool missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(streamDir, "progress_demo_gen.go")); err != nil {
		t.Fatalf("generated stream tool missing: %v", err)
	}

	runGo(t, moduleDir, goCache, "mod", "tidy")
	runGo(t, moduleDir, goCache, "test", "-race", "./...")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}

func runGo(t *testing.T, dir, cache string, args ...string) {
	t.Helper()

	// #nosec G204 -- test helper runs controlled go subcommands.
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOCACHE="+cache, "GOSUMDB=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s in %s failed: %v\n%s", strings.Join(args, " "), dir, err, output)
	}
}

func TestGeneratedProxyBareReadLimitExceeded(t *testing.T) {
	t.Parallel()
	tool, err := toolsy.NewProxyTool(
		"proxy_limit",
		"Returns read limit error",
		[]byte(`{"type":"object"}`),
		func(_ context.Context, _ *toolsy.RunEnv, _ []byte, _ func(toolsy.Chunk) error) error {
			return textprocessor.ErrReadLimitExceeded
		},
	)
	if err != nil {
		t.Fatalf("NewProxyTool: %v", err)
	}
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := toolsy.AsToolError(err)
	if !ok || te.Code != toolsy.CodeValidationFailed {
		t.Fatalf("error = %v, want CodeValidationFailed", err)
	}
	if !strings.Contains(te.Reason, "byte limit") {
		t.Fatalf("reason = %q, want byte limit hint", te.Reason)
	}
}
