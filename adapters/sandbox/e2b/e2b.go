package e2b

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/internal/sandboxfs"
)

const (
	workspacePrefix = "/workspace/"
	cleanupTimeout  = 5 * time.Second
)

// Client abstracts the E2B control plane operations required by this adapter.
type Client interface {
	CreateSandbox(ctx context.Context) (Session, error)
}

// Session is an active remote sandbox instance. StartAndWait must preserve literal
// executable/argv semantics and stream all output through the supplied writers.
// A transport requiring a command string owns its single serialization boundary;
// it must not concatenate raw arguments or reinterpret them as shell syntax.
type Session interface {
	WriteFile(ctx context.Context, path string, data []byte) error
	StartAndWait(
		ctx context.Context,
		command string,
		args []string,
		env map[string]string,
		stdout, stderr io.Writer,
	) (CommandResult, error)
	Kill(ctx context.Context) error
}

// CommandResult contains execution status; output comes only from supplied writers.
type CommandResult struct {
	ExitCode int
}

// Sandbox executes code in a remote E2B-style sandbox via an injected client.
type Sandbox struct {
	client    Client
	runtimes  map[string]Runtime
	languages []string
}

func cleanupSession(session Session) error {
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cleanupCancel()
	return session.Kill(cleanupCtx)
}

func classifyControlPlaneError(runCtx context.Context, err error, op string) error {
	if runCtx.Err() == context.DeadlineExceeded ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, exectool.ErrTimeout) {
		return exectool.ErrTimeout
	}
	if runCtx.Err() != nil {
		return runCtx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if mapped := toolsy.MapSandboxReadLimitError(err); mapped != nil {
		return mapped
	}
	return fmt.Errorf("%w: %s: %w", exectool.ErrSandboxFailure, op, err)
}

// New creates an E2B-backed sandbox adapter.
func New(client Client, opts ...Option) (*Sandbox, error) {
	if client == nil {
		return nil, errors.New("e2b sandbox: client is nil")
	}

	o := options{runtimes: defaultRuntimes()}
	for _, opt := range opts {
		opt(&o)
	}

	runtimes := make(map[string]Runtime, len(o.runtimes))
	languages := make([]string, 0, len(o.runtimes))
	for language, runtime := range o.runtimes {
		trimmed := strings.TrimSpace(language)
		if trimmed == "" {
			return nil, errors.New("e2b sandbox: runtime language must be non-empty")
		}
		if _, exists := runtimes[trimmed]; exists {
			return nil, fmt.Errorf("e2b sandbox: duplicate runtime language %q", trimmed)
		}
		command := runtime.Command
		if strings.TrimSpace(command) == "" {
			return nil, fmt.Errorf("e2b sandbox: runtime %q command must be non-empty", trimmed)
		}
		rawScriptName := strings.TrimSpace(runtime.ScriptName)
		if rawScriptName == "" {
			return nil, fmt.Errorf("e2b sandbox: runtime %q script name must be non-empty", trimmed)
		}
		scriptName, err := sandboxfs.NormalizeRelativePath(rawScriptName)
		if err != nil {
			return nil, fmt.Errorf("e2b sandbox: runtime %q script name: %w", trimmed, err)
		}
		args, err := normalizeRuntimeArgs(command, runtime.Args, rawScriptName, scriptName)
		if err != nil {
			return nil, fmt.Errorf("e2b sandbox: runtime %q command: %w", trimmed, err)
		}
		runtimes[trimmed] = Runtime{
			Command:    command,
			Args:       args,
			ScriptName: scriptName,
		}
		languages = append(languages, trimmed)
	}
	sort.Strings(languages)

	return &Sandbox{
		client:    client,
		runtimes:  runtimes,
		languages: languages,
	}, nil
}

// SupportedLanguages returns a sorted copy of configured languages.
func (s *Sandbox) SupportedLanguages() []string {
	return append([]string(nil), s.languages...)
}

// normalizeRuntimeArgs snapshots literal argv and rewrites only the script argument.
func normalizeRuntimeArgs(command string, args []string, rawScriptName, cleanScriptName string) ([]string, error) {
	if !utf8.ValidString(command) || strings.ContainsRune(command, 0) {
		return nil, errors.New("executable must be valid UTF8 without NUL")
	}
	rawScriptArg := workspacePrefix + rawScriptName
	cleanScriptArg := workspacePrefix + cleanScriptName
	normalized := append([]string(nil), args...)
	found := false
	for i, arg := range normalized {
		if !utf8.ValidString(arg) || strings.ContainsRune(arg, 0) {
			return nil, fmt.Errorf("argument %d must be valid UTF8 without NUL", i)
		}
		if arg != rawScriptArg {
			continue
		}
		if found {
			return nil, errors.New("script path must occur exactly once in Args")
		}
		found = true
		normalized[i] = cleanScriptArg
	}
	if !found {
		return nil, fmt.Errorf("args must contain script path %q exactly once", rawScriptArg)
	}
	// A separately supplied canonical path must not become a second script arg
	// after rewriting a noncanonical reference.
	canonicalCount := 0
	for _, arg := range normalized {
		if arg == cleanScriptArg {
			canonicalCount++
		}
	}
	if canonicalCount != 1 {
		return nil, errors.New("canonical script path must occur exactly once in args")
	}
	return normalized, nil
}

// Run executes code in a remote sandbox session.
//
//nolint:nonamedreturns // Deferred cleanup must attach diagnostics while preserving the result and primary error.
func (s *Sandbox) Run(ctx context.Context, req exectool.RunRequest) (result exectool.RunResult, runErr error) {
	runtime, ok := s.runtimes[strings.TrimSpace(req.Language)]
	if !ok {
		return exectool.RunResult{}, fmt.Errorf("%w: %s", exectool.ErrUnsupportedLanguage, req.Language)
	}

	canonicalFiles, err := sandboxfs.CanonicalizeFiles(req.Files, runtime.ScriptName)
	if err != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: validate files: %w", exectool.ErrSandboxFailure, err)
	}

	session, err := s.client.CreateSandbox(ctx)
	if err != nil {
		return exectool.RunResult{}, classifyControlPlaneError(ctx, err, "create sandbox")
	}
	if session == nil {
		return exectool.RunResult{}, fmt.Errorf("%w: client returned nil session", exectool.ErrSandboxFailure)
	}
	defer func() { runErr = sandboxfs.WithCleanupError(runErr, "e2b", "kill session", cleanupSession(session)) }()

	for name, data := range canonicalFiles {
		if err = session.WriteFile(ctx, workspacePrefix+name, data); err != nil {
			return exectool.RunResult{}, classifyControlPlaneError(ctx, err, "write file")
		}
	}

	if err = session.WriteFile(ctx, workspacePrefix+runtime.ScriptName, []byte(req.Code)); err != nil {
		return exectool.RunResult{}, classifyControlPlaneError(ctx, err, "write script")
	}

	start := time.Now()
	var stdoutBuf = sandboxfs.NewCappedBuffer("stdout", sandboxfs.DefaultMaxSandboxOutputBytes)
	var stderrBuf = sandboxfs.NewCappedBuffer("stderr", sandboxfs.DefaultMaxSandboxOutputBytes)
	commandResult, err := session.StartAndWait(
		ctx,
		runtime.Command,
		append([]string(nil), runtime.Args...),
		req.Env,
		stdoutBuf,
		stderrBuf,
	)
	if err != nil {
		return sandboxfs.FinalizeOrInterrupt(
			ctx,
			classifyControlPlaneError(ctx, err, "start process"),
			stdoutBuf, stderrBuf, 0, time.Since(start),
		)
	}

	return sandboxfs.FinalizeOrInterrupt(
		ctx, nil, stdoutBuf, stderrBuf, commandResult.ExitCode, time.Since(start),
	)
}
