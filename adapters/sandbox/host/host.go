package host

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/internal/sandboxfs"
)

// Sandbox executes code by invoking configured binaries on the local host.
//
// DANGER: NO ISOLATION. USE ONLY WITH HUMAN-IN-THE-LOOP.
type Sandbox struct {
	runtimes    map[string]Runtime
	languages   []string
	tempDirRoot string
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
	}, nil
}

// SupportedLanguages returns a sorted copy of configured languages.
func (s *Sandbox) SupportedLanguages() []string {
	return append([]string(nil), s.languages...)
}

// Run executes code in a temporary workspace on the host.
func (s *Sandbox) Run(ctx context.Context, req exectool.RunRequest) (exectool.RunResult, error) {
	runtime, ok := s.runtimes[strings.TrimSpace(req.Language)]
	if !ok {
		return exectool.RunResult{}, fmt.Errorf("%w: %s", exectool.ErrUnsupportedLanguage, req.Language)
	}

	runCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	workspace, err := os.MkdirTemp(s.tempDirRoot, "toolsy-host-*")
	if err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: create workspace: %w", exectool.ErrSandboxFailure, err)
	}
	defer func() {
		_ = os.RemoveAll(workspace)
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
	cmd := exec.CommandContext(runCtx, runtime.Command, args...)
	prepareCommand(cmd)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), encodeEnv(req.Env)...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)

	if err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return exectool.RunResult{}, exectool.ErrTimeout
		}
		if errors.Is(runCtx.Err(), context.Canceled) {
			return exectool.RunResult{}, runCtx.Err()
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exectool.RunResult{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: exitErr.ExitCode(),
				Duration: duration,
			}, nil
		}

		return exectool.RunResult{}, fmt.Errorf("%w: execute runtime: %w", exectool.ErrSandboxFailure, err)
	}

	return exectool.RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
		Duration: duration,
	}, nil
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
