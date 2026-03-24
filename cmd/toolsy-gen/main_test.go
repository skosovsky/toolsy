package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHelpWritesUsageToStdout(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"-help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "Usage: toolsy-gen [path ...]") {
		t.Fatalf("stdout = %q, want usage", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunSuccessPrintsGeneratedFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "apptools")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(dir)

	if err := os.WriteFile(filepath.Join(dir, "book.yaml"), []byte(`
name: "book_appointment"
description: "Book an appointment"
parameters:
  type: object
  properties:
    doctor_id:
      type: string
      description: "Doctor id"
  required: ["doctor_id"]
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "generated 1 file(s)") {
		t.Fatalf("stdout = %q, want success summary", stdout.String())
	}
	if !strings.Contains(stdout.String(), filepath.Join(dir, "book_appointment_gen.go")) {
		t.Fatalf("stdout = %q, want generated file path", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunErrorPrefixesToolName(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "apptools")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte(`
name: "broken"
description: "Broken"
parameters:
  type: object
  properties:
    doctor_id:
      type: string
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "toolsy-gen:") {
		t.Fatalf("stderr = %q, want prefixed error", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}
