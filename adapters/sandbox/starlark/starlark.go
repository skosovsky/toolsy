package starlark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	toolstarlark "go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/internal/sandboxfs"
	"github.com/skosovsky/toolsy/textprocessor"
)

const languageStarlark = "starlark"

// DefaultMaxExecutionSteps is a finite interpreter budget, not a wall-clock or memory limit.
const DefaultMaxExecutionSteps uint64 = 1_000_000

// ErrStepLimit indicates that the interpreter exhausted its execution budget.
var ErrStepLimit = errors.New("starlark execution step limit exceeded")

// Config describes enforceable in-process interpreter limits.
type Config struct {
	MaxExecutionSteps uint64
}

// DefaultConfig returns a finite execution policy.
func DefaultConfig() Config {
	return Config{MaxExecutionSteps: DefaultMaxExecutionSteps}
}

// Sandbox executes Starlark code with in-memory files and env bindings.
type Sandbox struct {
	languages         []string
	maxExecutionSteps uint64
}

// New creates a Starlark sandbox exposing only the "starlark" language.
func New(config Config) (*Sandbox, error) {
	if config.MaxExecutionSteps == 0 {
		return nil, fmt.Errorf("%w: MaxExecutionSteps must be positive", exectool.ErrSandboxFailure)
	}
	return &Sandbox{languages: []string{languageStarlark}, maxExecutionSteps: config.MaxExecutionSteps}, nil
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

	if ctx.Err() != nil {
		return sandboxfs.FinalizeOrInterrupt(ctx, nil, nil, nil, 0, 0)
	}
	if s.maxExecutionSteps == 0 {
		return exectool.RunResult{}, fmt.Errorf("%w: missing execution step policy", exectool.ErrSandboxFailure)
	}

	var stdout = sandboxfs.NewCappedBuffer("stdout", sandboxfs.DefaultMaxSandboxOutputBytes)
	var printErr error
	thread := new(toolstarlark.Thread)
	thread.Name = "toolsy-starlark"
	thread.SetMaxExecutionSteps(s.maxExecutionSteps)
	stepLimitReached := false
	thread.OnMaxSteps = func(thread *toolstarlark.Thread) {
		stepLimitReached = true
		thread.Cancel(ErrStepLimit.Error())
	}
	thread.Print = func(_ *toolstarlark.Thread, msg string) {
		if printErr != nil {
			return
		}
		if _, err := stdout.Write([]byte(msg)); err != nil {
			printErr = err
			thread.Cancel("stdout limit exceeded")
			return
		}
		if _, err := stdout.Write([]byte{'\n'}); err != nil {
			printErr = err
			thread.Cancel("stdout limit exceeded")
		}
	}

	stopWatching := watchCancellation(ctx, thread)
	defer stopWatching()

	predeclared, err := buildPredeclared(req.Env, req.Files)
	if err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: build bindings: %w", exectool.ErrSandboxFailure, err)
	}

	start := time.Now()
	var fileOpts syntax.FileOptions
	_, err = toolstarlark.ExecFileOptions(&fileOpts, thread, "main.star", req.Code, predeclared)
	duration := time.Since(start)

	if stepLimitReached {
		return sandboxfs.FinalizeOrInterrupt(ctx,
			fmt.Errorf("%w: %w (limit %d)", exectool.ErrSandboxFailure, ErrStepLimit, s.maxExecutionSteps),
			stdout, nil, 0, duration)
	}

	if printErr != nil {
		return sandboxfs.FinalizeOrInterrupt(
			ctx,
			fmt.Errorf("%w: stdout: %w", exectool.ErrSandboxFailure, printErr),
			stdout, nil, 0, duration,
		)
	}

	if err != nil {
		var stderrBuf = sandboxfs.NewCappedBuffer("stderr", sandboxfs.DefaultMaxSandboxOutputBytes)
		if _, writeErr := io.WriteString(stderrBuf, err.Error()); writeErr != nil {
			return sandboxfs.FinalizeOrInterrupt(ctx, writeErr, stdout, stderrBuf, 0, duration)
		}

		return sandboxfs.FinalizeOrInterrupt(
			ctx,
			nil,
			stdout,
			stderrBuf,
			1,
			duration,
		)
	}

	return sandboxfs.FinalizeOrInterrupt(ctx, nil, stdout, nil, 0, duration)
}

// watchCancellation owns and joins its watcher; no goroutine outlives Run.
func watchCancellation(ctx context.Context, thread *toolstarlark.Thread) func() {
	done := make(chan struct{})
	watchdogDone := make(chan struct{})
	go func() {
		defer close(watchdogDone)
		select {
		case <-ctx.Done():
			thread.Cancel(ctx.Err().Error())
		case <-done:
		}
	}()
	return func() {
		close(done)
		<-watchdogDone
	}
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
	if len(data) > sandboxfs.DefaultMaxSandboxFileReadBytes {
		return nil, fmt.Errorf(
			"%w: fs.read %s exceeds %d byte limit: %w",
			exectool.ErrSandboxFailure,
			path,
			sandboxfs.DefaultMaxSandboxFileReadBytes,
			textprocessor.ErrReadLimitExceeded,
		)
	}
	return toolstarlark.String(string(data)), nil
}
