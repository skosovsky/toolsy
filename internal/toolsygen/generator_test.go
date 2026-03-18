package toolsygen

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestDiscoverManifestsSortedAndSkipsHidden(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "b.yaml"), "name: b\ndescription: b\nparameters:\n  type: object\n  properties: {}\n")
	writeFile(t, filepath.Join(root, "nested", "a.json"), `{"name":"a","description":"a","parameters":{"type":"object","properties":{}}}`)
	writeFile(t, filepath.Join(root, ".git", "ignored.yaml"), "name: ignored\ndescription: ignored\nparameters:\n  type: object\n  properties: {}\n")
	writeFile(t, filepath.Join(root, "vendor", "ignored.yml"), "name: ignored\ndescription: ignored\nparameters:\n  type: object\n  properties: {}\n")

	paths, err := discoverManifests(context.Background(), []string{root})
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

	_, err := inferPackageName(dir)
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "tool.yaml")
			writeFile(t, path, strings.TrimSpace(tt.manifest))

			_, err := loadManifest(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("loadManifest error = %v, want substring %q", err, tt.wantSubstr)
			}
		})
	}
}

func TestRenderManifestImportsAndPackageFallback(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "apptools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
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

	m, err := loadManifest(path)
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
		`"time"`,
		"package apptools",
		"type BookAppointmentStreamHandler interface",
		"DoctorID string",
		`validate:"required"`,
		`return errors.New("doctor_id is required")`,
		`return &toolsy.ClientError{Reason: err.Error(), Err: toolsy.ErrValidation}`,
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("generated code missing %q:\n%s", want, code)
		}
	}
}

func TestValidateManifestSetDetectsCollisions(t *testing.T) {
	t.Parallel()

	dirOne := filepath.Join(t.TempDir(), "one")
	dirTwo := filepath.Join(t.TempDir(), "two")
	if err := os.MkdirAll(dirOne, 0o755); err != nil {
		t.Fatalf("mkdir one: %v", err)
	}
	if err := os.MkdirAll(dirTwo, 0o755); err != nil {
		t.Fatalf("mkdir two: %v", err)
	}

	errs := validateManifestSet([]*manifest{
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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

func TestCommitFilesAtomicallyRollsBackOnFailure(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first_gen.go")
	secondPath := filepath.Join(dir, "second_gen.go")
	writeFile(t, firstPath, "original")

	originalRename := osRename
	t.Cleanup(func() {
		osRename = originalRename
	})

	renameCalls := 0
	osRename = func(oldPath, newPath string) error {
		renameCalls++
		if renameCalls == 3 {
			return errors.New("forced rename failure")
		}
		return originalRename(oldPath, newPath)
	}

	err := commitFilesAtomically(context.Background(), []generatedFile{
		{Path: firstPath, Content: []byte("updated")},
		{Path: secondPath, Content: []byte("created")},
	}, 0o644)
	if err == nil || !strings.Contains(err.Error(), "forced rename failure") {
		t.Fatalf("commitFilesAtomically error = %v, want forced rename failure", err)
	}

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

	writeFile(t, filepath.Join(moduleDir, "go.mod"), "module fixture\n\ngo 1.26.0\n\nrequire github.com/skosovsky/toolsy v0.0.0\n\nreplace github.com/skosovsky/toolsy => "+repoLink+"\n")

	appDir := filepath.Join(moduleDir, "apptools")
	streamDir := filepath.Join(moduleDir, "streamtools")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir apptools: %v", err)
	}
	if err := os.MkdirAll(streamDir, 0o755); err != nil {
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
	"testing"
	"time"

	"github.com/skosovsky/toolsy"
)

type bookHandler struct{}

func (bookHandler) Execute(_ context.Context, input BookAppointmentInput) (string, error) {
	return input.DoctorID + "|" + input.SlotTime.UTC().Format(time.RFC3339), nil
}

func TestGeneratedNonStreamTool(t *testing.T) {
	tool, err := NewBookAppointmentTool(bookHandler{})
	if err != nil {
		t.Fatalf("NewBookAppointmentTool: %v", err)
	}

	var got string
	err = tool.Execute(context.Background(), []byte("{\"doctor_id\":\"d1\",\"slot_time\":\"2026-03-18T09:00:00Z\"}"), func(c toolsy.Chunk) error {
		got = string(c.Data)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute valid payload: %v", err)
	}
	if got != "d1|2026-03-18T09:00:00Z" {
		t.Fatalf("result = %q", got)
	}

	err = tool.Execute(context.Background(), []byte("{}"), func(toolsy.Chunk) error { return nil })
	if err == nil || !toolsy.IsClientError(err) {
		t.Fatalf("missing required field error = %v, want client error", err)
	}

	err = tool.Execute(context.Background(), []byte("{\"doctor_id\":\"\",\"slot_time\":\"2026-03-18T09:00:00Z\"}"), func(toolsy.Chunk) error { return nil })
	if err == nil || !toolsy.IsClientError(err) {
		t.Fatalf("empty doctor_id error = %v, want client error", err)
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
	"errors"
	"iter"
	"testing"

	"github.com/skosovsky/toolsy"
)

type streamHandler struct{}

func (streamHandler) ExecuteStream(_ context.Context, input ProgressDemoInput) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		switch input.Count {
		case 0:
			return
		case 1:
			yield("done", nil)
		case 3:
			if !yield("step-1", nil) {
				return
			}
			if !yield("step-2", nil) {
				return
			}
			yield("done", nil)
		case -1:
			if !yield("step-1", nil) {
				return
			}
			yield("", errors.New("boom"))
		default:
			yield("unexpected", nil)
		}
	}
}

func TestGeneratedStreamTool(t *testing.T) {
	tool, err := NewProgressDemoTool(streamHandler{})
	if err != nil {
		t.Fatalf("NewProgressDemoTool: %v", err)
	}

	type chunkInfo struct {
		Event string
		Data  string
	}
	tests := []struct {
		name       string
		args       string
		wantChunks []chunkInfo
		wantErr    bool
	}{
		{
			name: "zero items yields empty final",
			args: "{\"count\":0}",
			wantChunks: []chunkInfo{
				{Event: toolsy.EventResult, Data: ""},
			},
		},
		{
			name: "one item yields one final",
			args: "{\"count\":1}",
			wantChunks: []chunkInfo{
				{Event: toolsy.EventResult, Data: "done"},
			},
		},
		{
			name: "three items yields progress and final",
			args: "{\"count\":3}",
			wantChunks: []chunkInfo{
				{Event: toolsy.EventProgress, Data: "step-1"},
				{Event: toolsy.EventProgress, Data: "step-2"},
				{Event: toolsy.EventResult, Data: "done"},
			},
		},
		{
			name: "error keeps buffered item as progress and no final",
			args: "{\"count\":-1}",
			wantChunks: []chunkInfo{
				{Event: toolsy.EventProgress, Data: "step-1"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		var got []chunkInfo
		err := tool.Execute(context.Background(), []byte(tt.args), func(c toolsy.Chunk) error {
			got = append(got, chunkInfo{Event: c.Event, Data: string(c.Data)})
			return nil
		})
		if tt.wantErr {
			if err == nil || !toolsy.IsSystemError(err) {
				t.Fatalf("%s: error = %v, want system error", tt.name, err)
			}
		} else if err != nil {
			t.Fatalf("%s: unexpected error %v", tt.name, err)
		}

		if len(got) != len(tt.wantChunks) {
			t.Fatalf("%s: got %d chunks, want %d (%v)", tt.name, len(got), len(tt.wantChunks), got)
		}
		for i := range tt.wantChunks {
			if got[i] != tt.wantChunks[i] {
				t.Fatalf("%s: chunk %d = %#v, want %#v", tt.name, i, got[i], tt.wantChunks[i])
			}
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
	runGo(t, moduleDir, goCache, "test", "./...")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
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

	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOCACHE="+cache, "GOSUMDB=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s in %s failed: %v\n%s", strings.Join(args, " "), dir, err, output)
	}
}
