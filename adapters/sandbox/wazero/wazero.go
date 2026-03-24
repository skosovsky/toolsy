package wazero

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	wazerosys "github.com/tetratelabs/wazero/sys"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/internal/sandboxfs"
)

const guestWorkspace = "/workspace"

type guestEngine interface {
	Run(
		ctx context.Context,
		module []byte,
		workspaceDir string,
		env map[string]string,
		stdout, stderr *bytes.Buffer,
	) (time.Duration, error)
}

type wazeroEngine struct {
	runtimeConfig wazero.RuntimeConfig
}

// Sandbox executes a precompiled guest interpreter inside wazero.
type Sandbox struct {
	language string
	module   []byte
	engine   guestEngine
}

// NewInterpreter creates a sandbox that presents a single text language backed
// by a precompiled WASI interpreter module.
func NewInterpreter(language string, module []byte, opts ...Option) (*Sandbox, error) {
	trimmed := strings.TrimSpace(language)
	if trimmed == "" {
		return nil, errors.New("wazero sandbox: language must be non-empty")
	}
	if strings.EqualFold(trimmed, "wasm") {
		return nil, fmt.Errorf("wazero sandbox: language %q is not LLM-safe", trimmed)
	}
	if len(module) == 0 {
		return nil, errors.New("wazero sandbox: module must not be empty")
	}

	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if o.runtimeConfig == nil {
		o.runtimeConfig = wazero.NewRuntimeConfig().WithCloseOnContextDone(true)
	}

	return &Sandbox{
		language: trimmed,
		module:   append([]byte(nil), module...),
		engine: &wazeroEngine{
			runtimeConfig: o.runtimeConfig,
		},
	}, nil
}

// SupportedLanguages returns the single configured language.
func (s *Sandbox) SupportedLanguages() []string {
	return []string{s.language}
}

// Run executes the configured guest interpreter in a mounted temporary
// workspace.
func (s *Sandbox) Run(ctx context.Context, req exectool.RunRequest) (exectool.RunResult, error) {
	if strings.TrimSpace(req.Language) != s.language {
		return exectool.RunResult{}, fmt.Errorf("%w: %s", exectool.ErrUnsupportedLanguage, req.Language)
	}

	runCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	workspaceDir, err := os.MkdirTemp("", "toolsy-wazero-*")
	if err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: create workspace: %w", exectool.ErrSandboxFailure, err)
	}
	defer func() {
		_ = os.RemoveAll(workspaceDir)
	}()

	canonicalFiles, err := sandboxfs.CanonicalizeFiles(req.Files, "main.code")
	if err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: validate files: %w", exectool.ErrSandboxFailure, err)
	}

	if err = sandboxfs.WriteWorkspace(workspaceDir, canonicalFiles); err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: materialize files: %w", exectool.ErrSandboxFailure, err)
	}
	if err = sandboxfs.WriteFile(workspaceDir, "main.code", []byte(req.Code)); err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: write code file: %w", exectool.ErrSandboxFailure, err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	duration, err := s.engine.Run(runCtx, s.module, workspaceDir, req.Env, &stdout, &stderr)

	if err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return exectool.RunResult{}, exectool.ErrTimeout
		}
		if errors.Is(runCtx.Err(), context.Canceled) {
			return exectool.RunResult{}, runCtx.Err()
		}

		var exitErr *wazerosys.ExitError
		if errors.As(err, &exitErr) {
			return exectool.RunResult{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: int(exitErr.ExitCode()),
				Duration: duration,
			}, nil
		}

		return exectool.RunResult{}, fmt.Errorf("%w: execute guest: %w", exectool.ErrSandboxFailure, err)
	}

	return exectool.RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
		Duration: duration,
	}, nil
}

func (e *wazeroEngine) Run(
	ctx context.Context,
	module []byte,
	workspaceDir string,
	env map[string]string,
	stdout, stderr *bytes.Buffer,
) (time.Duration, error) {
	runtime := wazero.NewRuntimeWithConfig(ctx, e.runtimeConfig)
	defer func() {
		_ = runtime.Close(context.Background())
	}()

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		return 0, err
	}

	compiled, err := runtime.CompileModule(ctx, module)
	if err != nil {
		return 0, err
	}

	config := wazero.NewModuleConfig().
		WithStdout(stdout).
		WithStderr(stderr).
		WithFSConfig(wazero.NewFSConfig().WithDirMount(workspaceDir, guestWorkspace))
	for key, value := range env {
		config = config.WithEnv(key, value)
	}

	start := time.Now()
	mod, err := runtime.InstantiateModule(ctx, compiled, config)
	duration := time.Since(start)
	if mod != nil {
		defer func() {
			_ = mod.Close(context.Background())
		}()
	}
	return duration, err
}
