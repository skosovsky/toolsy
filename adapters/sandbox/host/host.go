package host

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/internal/sandboxfs"
)

const cleanupTimeout = 5 * time.Second

// Sandbox executes code by invoking configured binaries on the local host.
//
// DANGER: NO ISOLATION. USE ONLY WITH HUMAN-IN-THE-LOOP.
type Sandbox struct {
	runtimes    map[string]Runtime
	languages   []string
	tempDirRoot string
	environment map[string]string
}

// New creates a host-backed sandbox.
func New(opts ...Option) (*Sandbox, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if len(o.runtimes) == 0 {
		return nil, errors.New("host sandbox: at least one runtime must be configured")
	}

	environment := make(map[string]string)
	if o.inheritEnvironment {
		for _, entry := range os.Environ() {
			key, value, _ := strings.Cut(entry, "=")
			environment[key] = value
		}
	}
	maps.Copy(environment, o.environment)
	if err := validateEnv(environment); err != nil {
		return nil, err
	}
	runtimes := make(map[string]Runtime, len(o.runtimes))
	languages := make([]string, 0, len(o.runtimes))
	for language, runtime := range o.runtimes {
		trimmed := strings.TrimSpace(language)
		if trimmed == "" {
			return nil, errors.New("host sandbox: runtime language must be non-empty")
		}
		if _, exists := runtimes[trimmed]; exists {
			return nil, fmt.Errorf("host sandbox: duplicate runtime language %q", trimmed)
		}
		command := strings.TrimSpace(runtime.Command)
		if command == "" {
			return nil, fmt.Errorf("host sandbox: runtime %q command must be non-empty", trimmed)
		}
		rawScriptName := strings.TrimSpace(runtime.ScriptName)
		if rawScriptName == "" {
			return nil, fmt.Errorf("host sandbox: runtime %q script name must be non-empty", trimmed)
		}
		scriptName, err := sandboxfs.NormalizeRelativePath(rawScriptName)
		if err != nil {
			return nil, fmt.Errorf("host sandbox: runtime %q script name: %w", trimmed, err)
		}
		runtimes[trimmed] = Runtime{
			Command:    command,
			Args:       append([]string(nil), runtime.Args...),
			ScriptName: scriptName,
		}
		languages = append(languages, trimmed)
	}
	sort.Strings(languages)

	return &Sandbox{
		runtimes:    runtimes,
		languages:   languages,
		tempDirRoot: o.tempDirRoot,
		environment: environment,
	}, nil
}

// SupportedLanguages returns a sorted copy of configured languages.
func (s *Sandbox) SupportedLanguages() []string {
	return append([]string(nil), s.languages...)
}

// Run executes code in a temporary workspace on the host.
//
//nolint:nonamedreturns // Deferred cleanup must attach diagnostics while preserving the result and primary error.
func (s *Sandbox) Run(ctx context.Context, req exectool.RunRequest) (result exectool.RunResult, runErr error) {
	runtime, ok := s.runtimes[strings.TrimSpace(req.Language)]
	if !ok {
		return exectool.RunResult{}, fmt.Errorf("%w: %s", exectool.ErrUnsupportedLanguage, req.Language)
	}

	if err := validateEnv(req.Env); err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: %w", exectool.ErrSandboxFailure, err)
	}
	workspace, err := os.MkdirTemp(s.tempDirRoot, "toolsy-host-*")
	if err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: create workspace: %w", exectool.ErrSandboxFailure, err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		runErr = sandboxfs.WithCleanupError(
			runErr,
			"host",
			"remove workspace",
			sandboxfs.RemoveWorkspace(cleanupCtx, workspace),
		)
	}()

	canonicalFiles, err := sandboxfs.CanonicalizeFiles(req.Files, runtime.ScriptName)
	if err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: validate files: %w", exectool.ErrSandboxFailure, err)
	}

	if err = sandboxfs.WriteWorkspace(workspace, canonicalFiles); err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: materialize files: %w", exectool.ErrSandboxFailure, err)
	}
	if err = sandboxfs.WriteFile(workspace, runtime.ScriptName, []byte(req.Code)); err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: write script: %w", exectool.ErrSandboxFailure, err)
	}

	args := append(append([]string(nil), runtime.Args...), runtime.ScriptName)
	// #nosec G204 -- runtime commands are explicit host-sandbox configuration, not LLM-controlled input.
	cmd := exec.CommandContext(ctx, runtime.Command, args...)
	prepareCommand(cmd)
	cmd.Dir = workspace
	env := make(map[string]string, len(s.environment)+len(req.Env))
	maps.Copy(env, s.environment)
	maps.Copy(env, req.Env)
	cmd.Env = encodeEnv(env)
	if cmd.Env == nil {
		cmd.Env = []string{}
	}
	cmd.WaitDelay = cleanupTimeout

	var stdout = sandboxfs.NewCappedBuffer("stdout", sandboxfs.DefaultMaxSandboxOutputBytes)
	var stderr = sandboxfs.NewCappedBuffer("stderr", sandboxfs.DefaultMaxSandboxOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)

	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return sandboxfs.FinalizeOrInterrupt(
				ctx, nil, stdout, stderr, exitErr.ExitCode(), duration, false,
			)
		}

		return sandboxfs.FinalizeOrInterrupt(
			ctx,
			fmt.Errorf("%w: execute runtime: %w", exectool.ErrSandboxFailure, err),
			stdout, stderr, 0, duration, false,
		)
	}

	return sandboxfs.FinalizeOrInterrupt(ctx, nil, stdout, stderr, 0, duration, false)
}

func encodeEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	sort.Strings(out)
	return out
}

func validateEnv(env map[string]string) error {
	for key, value := range env {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return errors.New("host sandbox: invalid environment entry")
		}
	}
	return nil
}
