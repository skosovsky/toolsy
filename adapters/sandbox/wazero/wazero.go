package wazero

import (
	"context"
	"errors"
	"fmt"
	"io"
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

const cleanupTimeout = 5 * time.Second

type guestEngine interface {
	Run(
		ctx context.Context,
		module []byte,
		workspaceDir string,
		env map[string]string,
		stdout, stderr io.Writer,
	) (time.Duration, error)
}

// engineCleanupError keeps a completed guest outcome separate from cleanup diagnostics.
type engineCleanupError struct {
	primary error
	cleanup error
}

func (e *engineCleanupError) Error() string   { return errors.Join(e.primary, e.cleanup).Error() }
func (e *engineCleanupError) Unwrap() []error { return []error{e.primary, e.cleanup} }

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

	o := options{runtimeConfig: nil, memoryLimitPages: DefaultMemoryLimitPages}
	for _, opt := range opts {
		opt(&o)
	}
	if o.runtimeConfig == nil {
		o.runtimeConfig = wazero.NewRuntimeConfig()
	}

	if o.memoryLimitPages == 0 || o.memoryLimitPages > 65536 {
		return nil, errors.New("wazero sandbox: memory limit must be 1–65536 pages")
	}
	o.runtimeConfig = o.runtimeConfig.WithCloseOnContextDone(true).WithMemoryLimitPages(o.memoryLimitPages)

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
//
//nolint:nonamedreturns // Deferred cleanup must preserve the primary outcome and attach diagnostics.
func (s *Sandbox) Run(ctx context.Context, req exectool.RunRequest) (result exectool.RunResult, runErr error) {
	if strings.TrimSpace(req.Language) != s.language {
		return exectool.RunResult{}, fmt.Errorf("%w: %s", exectool.ErrUnsupportedLanguage, req.Language)
	}

	workspaceDir, err := os.MkdirTemp("", "toolsy-wazero-*")
	if err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: create workspace: %w", exectool.ErrSandboxFailure, err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		runErr = sandboxfs.WithCleanupError(
			runErr,
			"wazero",
			"remove workspace",
			sandboxfs.RemoveWorkspace(cleanupCtx, workspaceDir),
		)
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

	var stdout = sandboxfs.NewCappedBuffer("stdout", sandboxfs.DefaultMaxSandboxOutputBytes)
	var stderr = sandboxfs.NewCappedBuffer("stderr", sandboxfs.DefaultMaxSandboxOutputBytes)
	duration, err := s.engine.Run(ctx, s.module, workspaceDir, req.Env, stdout, stderr)
	if cleanup, ok := errors.AsType[*engineCleanupError](err); ok {
		err = cleanup.primary
		defer func() { runErr = errors.Join(runErr, cleanup.cleanup) }()
	}

	if err != nil {
		if exitErr, ok := errors.AsType[*wazerosys.ExitError](err); ok {
			result, runErr = sandboxfs.FinalizeOrInterrupt(
				ctx, nil, stdout, stderr, int(exitErr.ExitCode()), duration, false,
			)
			return result, runErr
		}

		return sandboxfs.FinalizeOrInterrupt(
			ctx,
			fmt.Errorf("%w: execute guest: %w", exectool.ErrSandboxFailure, err),
			stdout, stderr, 0, duration, false,
		)
	}

	return sandboxfs.FinalizeOrInterrupt(ctx, nil, stdout, stderr, 0, duration, false)
}

//nolint:nonamedreturns // Deferred runtime cleanup attaches diagnostics without replacing execution errors.
func (e *wazeroEngine) Run(
	ctx context.Context,
	module []byte,
	workspaceDir string,
	env map[string]string,
	stdout, stderr io.Writer,
) (duration time.Duration, runErr error) {
	workspace, err := os.OpenRoot(workspaceDir)
	if err != nil {
		return 0, err
	}
	runtime := wazero.NewRuntimeWithConfig(ctx, e.runtimeConfig)
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		cleanupErr := errors.Join(
			sandboxfs.WithCleanupError(nil, "wazero", "close runtime", runtime.Close(cleanupCtx)),
			sandboxfs.WithCleanupError(nil, "wazero", "close workspace root", workspace.Close()),
		)
		if cleanupErr != nil {
			runErr = &engineCleanupError{primary: runErr, cleanup: cleanupErr}
		}
	}()

	if _, err = wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		return 0, err
	}

	compiled, err := runtime.CompileModule(ctx, module)
	if err != nil {
		return 0, err
	}

	config := wazero.NewModuleConfig().
		WithStdout(stdout).
		WithStderr(stderr).
		WithFSConfig(wazero.NewFSConfig().WithFSMount(workspace.FS(), guestWorkspace))
	for key, value := range env {
		config = config.WithEnv(key, value)
	}

	start := time.Now()
	_, err = runtime.InstantiateModule(ctx, compiled, config)
	duration = time.Since(start)
	return duration, err
}
