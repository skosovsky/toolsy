package starlark

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	toolstarlark "go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/internal/sandboxfs"
)

// Sandbox executes Starlark code with in-memory files and env bindings.
type Sandbox struct {
	languages []string
}

// New creates a Starlark sandbox exposing only the "starlark" language.
func New() *Sandbox {
	return &Sandbox{languages: []string{"starlark"}}
}

// SupportedLanguages returns a sorted copy of supported language names.
func (s *Sandbox) SupportedLanguages() []string {
	return append([]string(nil), s.languages...)
}

// Run executes the request in-process using go.starlark.net/starlark.
func (s *Sandbox) Run(ctx context.Context, req exectool.RunRequest) (exectool.RunResult, error) {
	if !s.supports(req.Language) {
		return exectool.RunResult{}, fmt.Errorf("%w: %s", exectool.ErrUnsupportedLanguage, req.Language)
	}

	runCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	var stdout strings.Builder
	thread := new(toolstarlark.Thread)
	thread.Name = "toolsy-starlark"
	thread.Print = func(_ *toolstarlark.Thread, msg string) {
		stdout.WriteString(msg)
		stdout.WriteByte('\n')
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-runCtx.Done():
			thread.Cancel(runCtx.Err().Error())
		case <-done:
		}
	}()

	predeclared, err := buildPredeclared(req.Env, req.Files)
	if err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: build bindings: %w", exectool.ErrSandboxFailure, err)
	}

	start := time.Now()
	var fileOpts syntax.FileOptions
	_, err = toolstarlark.ExecFileOptions(&fileOpts, thread, "main.star", req.Code, predeclared)
	duration := time.Since(start)

	if err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return exectool.RunResult{}, exectool.ErrTimeout
		}
		if errors.Is(runCtx.Err(), context.Canceled) {
			return exectool.RunResult{}, runCtx.Err()
		}

		return exectool.RunResult{
			Stdout:   strings.TrimSuffix(stdout.String(), "\n"),
			Stderr:   err.Error(),
			ExitCode: 1,
			Duration: duration,
		}, nil
	}

	return exectool.RunResult{
		Stdout:   strings.TrimSuffix(stdout.String(), "\n"),
		Stderr:   "",
		ExitCode: 0,
		Duration: duration,
	}, nil
}

func (s *Sandbox) supports(language string) bool {
	trimmed := strings.TrimSpace(language)
	return slices.Contains(s.languages, trimmed)
}

func buildPredeclared(env map[string]string, files map[string][]byte) (toolstarlark.StringDict, error) {
	envDict := toolstarlark.NewDict(len(env))
	for key, value := range env {
		if err := envDict.SetKey(toolstarlark.String(key), toolstarlark.String(value)); err != nil {
			return nil, err
		}
	}
	envDict.Freeze()

	immutableFiles, err := sandboxfs.CanonicalizeFiles(files)
	if err != nil {
		return nil, err
	}

	return toolstarlark.StringDict{
		"env": envDict,
		"fs":  &fsObject{files: immutableFiles},
	}, nil
}

type fsObject struct {
	files map[string][]byte
}

func (f *fsObject) String() string           { return "fs" }
func (f *fsObject) Type() string             { return "fs" }
func (f *fsObject) Freeze()                  {}
func (f *fsObject) Truth() toolstarlark.Bool { return toolstarlark.True }
func (f *fsObject) Hash() (uint32, error)    { return 0, errors.New("unhashable: fs") }

func (f *fsObject) Attr(name string) (toolstarlark.Value, error) {
	switch name {
	case "read":
		return toolstarlark.NewBuiltin("read", f.read), nil
	default:
		return nil, fmt.Errorf("fs: unknown attribute %q", name)
	}
}

func (f *fsObject) AttrNames() []string {
	return []string{"read"}
}

func (f *fsObject) read(
	_ *toolstarlark.Thread,
	_ *toolstarlark.Builtin,
	args toolstarlark.Tuple,
	kwargs []toolstarlark.Tuple,
) (toolstarlark.Value, error) {
	if len(kwargs) > 0 {
		return nil, errors.New("fs.read does not accept keyword arguments")
	}
	if args.Len() != 1 {
		return nil, errors.New("fs.read expects exactly one path argument")
	}

	pathValue, ok := args.Index(0).(toolstarlark.String)
	if !ok {
		return nil, errors.New("fs.read path must be a string")
	}

	path, err := sandboxfs.NormalizeRelativePath(pathValue.GoString())
	if err != nil {
		return nil, err
	}
	data, ok := f.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return toolstarlark.String(string(data)), nil
}
