package e2b

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/internal/sandboxfs"
)

const (
	workspacePrefix           = "/workspace/"
	cleanupTimeout            = 5 * time.Second
	shellTokenSliceInitialCap = 8
)

// Client abstracts the E2B control plane operations required by this adapter.
type Client interface {
	CreateSandbox(ctx context.Context) (Session, error)
}

// Session is an active remote sandbox instance.
type Session interface {
	WriteFile(ctx context.Context, path string, data []byte) error
	StartAndWait(ctx context.Context, command string, env map[string]string) (CommandResult, error)
	Kill(ctx context.Context) error
}

// CommandResult is the observable output of a remote command execution.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Sandbox executes code in a remote E2B-style sandbox via an injected client.
type Sandbox struct {
	client    Client
	runtimes  map[string]Runtime
	languages []string
}

type shellTokenStyle int

const (
	shellTokenStyleBare shellTokenStyle = iota
	shellTokenStyleSingleQuoted
	shellTokenStyleDoubleQuoted
	shellTokenStyleMixed
)

type shellToken struct {
	start   int
	end     int
	raw     string
	decoded string
	style   shellTokenStyle
}

func runtimeCommandContractError(command, rawScriptArg, detail string) error {
	return fmt.Errorf(
		"Runtime.Command %q must contain script path %q exactly once as a top-level shell argument: %s",
		command,
		rawScriptArg,
		detail,
	)
}

func cleanupSession(session Session) {
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cleanupCancel()
	_ = session.Kill(cleanupCtx)
}

func classifyControlPlaneError(runCtx context.Context, err error, op string) error {
	if runCtx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
		return exectool.ErrTimeout
	}
	if runCtx.Err() != nil {
		return runCtx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
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
		command := strings.TrimSpace(runtime.Command)
		if command == "" {
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
		command, err = normalizeRuntimeCommand(command, rawScriptName, scriptName)
		if err != nil {
			return nil, fmt.Errorf("e2b sandbox: runtime %q command: %w", trimmed, err)
		}
		runtimes[trimmed] = Runtime{
			Command:    command,
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

func normalizeRuntimeCommand(command, rawScriptName, cleanScriptName string) (string, error) {
	trimmedScript := strings.TrimSpace(rawScriptName)
	if trimmedScript == "" || trimmedScript == cleanScriptName {
		return command, nil
	}

	rawScriptArg := workspacePrefix + trimmedScript
	cleanScriptArg := workspacePrefix + cleanScriptName

	tokens, err := tokenizeShellCommand(command)
	if err != nil {
		return "", runtimeCommandContractError(command, rawScriptArg, fmt.Sprintf("invalid shell syntax: %v", err))
	}

	var normalized strings.Builder
	normalized.Grow(len(command))
	last := 0
	replaced := false
	for _, token := range tokens {
		normalized.WriteString(command[last:token.start])

		switch {
		case token.decoded == rawScriptArg:
			if replaced {
				return "", runtimeCommandContractError(
					command,
					rawScriptArg,
					"multiple script path references are not supported",
				)
			}
			repl, err := encodeShellToken(cleanScriptArg, token.style)
			if err != nil {
				return "", runtimeCommandContractError(
					command,
					rawScriptArg,
					fmt.Sprintf("unsupported token quoting style: %v", err),
				)
			}
			normalized.WriteString(repl)
			replaced = true
		case strings.Contains(token.decoded, rawScriptArg), strings.Contains(token.decoded, trimmedScript):
			return "", runtimeCommandContractError(
				command,
				rawScriptArg,
				"nested shell wrappers, embedded references, and wrapper commands are not supported",
			)
		default:
			normalized.WriteString(token.raw)
		}
		last = token.end
	}
	normalized.WriteString(command[last:])

	return normalized.String(), nil
}

func scanSingleQuotedSegment(command string, i int, decoded *strings.Builder) (int, error) {
	for {
		if i >= len(command) {
			return 0, errors.New("unterminated single-quoted token")
		}
		r, size := utf8.DecodeRuneInString(command[i:])
		i += size
		if r == '\'' {
			return i, nil
		}
		decoded.WriteRune(r)
	}
}

func scanDoubleQuotedSegment(command string, i int, decoded *strings.Builder) (int, error) {
	for i < len(command) {
		r, size := utf8.DecodeRuneInString(command[i:])
		if r == '"' {
			i += size
			return i, nil
		}
		if r == '\\' {
			i += size
			if i >= len(command) {
				return 0, errors.New("dangling escape in double-quoted token")
			}
			next, nextSize := utf8.DecodeRuneInString(command[i:])
			switch next {
			case '\\', '"', '$', '`':
				decoded.WriteRune(next)
				i += nextSize
			case '\n':
				i += nextSize
			default:
				decoded.WriteRune('\\')
				decoded.WriteRune(next)
				i += nextSize
			}
			continue
		}
		decoded.WriteRune(r)
		i += size
	}
	return 0, errors.New("unterminated double-quoted token")
}

func scanBareBackslash(command string, i int, decoded *strings.Builder) (int, error) {
	if i >= len(command) {
		return 0, errors.New("dangling escape in command")
	}
	next, nextSize := utf8.DecodeRuneInString(command[i:])
	if next == '\n' {
		return i + nextSize, nil
	}
	decoded.WriteRune(next)
	return i + nextSize, nil
}

func tokenStyleFromSegments(
	sawBare, sawSingleQuoted, sawDoubleQuoted bool,
	singleQuotedSegments, doubleQuotedSegments int,
) shellTokenStyle {
	switch {
	case sawBare && !sawSingleQuoted && !sawDoubleQuoted:
		return shellTokenStyleBare
	case !sawBare && sawSingleQuoted && !sawDoubleQuoted && singleQuotedSegments == 1:
		return shellTokenStyleSingleQuoted
	case !sawBare && !sawSingleQuoted && sawDoubleQuoted && doubleQuotedSegments == 1:
		return shellTokenStyleDoubleQuoted
	default:
		return shellTokenStyleMixed
	}
}

func scanNextShellToken(command string, start int) (shellToken, int, error) {
	i := start
	var decoded strings.Builder
	sawBare := false
	sawSingleQuoted := false
	sawDoubleQuoted := false
	singleQuotedSegments := 0
	doubleQuotedSegments := 0

	for i < len(command) {
		r, size := utf8.DecodeRuneInString(command[i:])
		if unicode.IsSpace(r) {
			break
		}

		switch r {
		case '\'':
			sawSingleQuoted = true
			singleQuotedSegments++
			i += size
			next, err := scanSingleQuotedSegment(command, i, &decoded)
			if err != nil {
				return shellToken{}, 0, err
			}
			i = next
		case '"':
			sawDoubleQuoted = true
			doubleQuotedSegments++
			i += size
			next, err := scanDoubleQuotedSegment(command, i, &decoded)
			if err != nil {
				return shellToken{}, 0, err
			}
			i = next
		case '\\':
			sawBare = true
			i += size
			next, err := scanBareBackslash(command, i, &decoded)
			if err != nil {
				return shellToken{}, 0, err
			}
			i = next
		default:
			sawBare = true
			decoded.WriteRune(r)
			i += size
		}
	}

	style := tokenStyleFromSegments(
		sawBare, sawSingleQuoted, sawDoubleQuoted,
		singleQuotedSegments, doubleQuotedSegments,
	)

	return shellToken{
		start:   start,
		end:     i,
		raw:     command[start:i],
		decoded: decoded.String(),
		style:   style,
	}, i, nil
}

func tokenizeShellCommand(command string) ([]shellToken, error) {
	tokens := make([]shellToken, 0, shellTokenSliceInitialCap)
	for i := 0; i < len(command); {
		r, size := utf8.DecodeRuneInString(command[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}

		tok, next, err := scanNextShellToken(command, i)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		i = next
	}
	return tokens, nil
}

func encodeShellToken(value string, style shellTokenStyle) (string, error) {
	switch style {
	case shellTokenStyleBare:
		var out strings.Builder
		for _, r := range value {
			if unicode.IsSpace(r) || strings.ContainsRune(`'"\\$`+"`"+";&|<>()*?[]{}!#~", r) {
				out.WriteByte('\\')
			}
			out.WriteRune(r)
		}
		return out.String(), nil
	case shellTokenStyleSingleQuoted:
		var out strings.Builder
		out.WriteByte('\'')
		for _, r := range value {
			if r == '\'' {
				out.WriteString(`'\''`)
				continue
			}
			out.WriteRune(r)
		}
		out.WriteByte('\'')
		return out.String(), nil
	case shellTokenStyleDoubleQuoted:
		var out strings.Builder
		out.WriteByte('"')
		for _, r := range value {
			if strings.ContainsRune(`\"$`+"`", r) {
				out.WriteByte('\\')
			}
			out.WriteRune(r)
		}
		out.WriteByte('"')
		return out.String(), nil
	default:
		return "", errors.New("unsupported mixed quoting for script token")
	}
}

// Run executes code in a remote sandbox session.
func (s *Sandbox) Run(ctx context.Context, req exectool.RunRequest) (exectool.RunResult, error) {
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
	defer cleanupSession(session)

	for name, data := range canonicalFiles {
		if err = session.WriteFile(ctx, workspacePrefix+name, data); err != nil {
			return exectool.RunResult{}, classifyControlPlaneError(ctx, err, "write file")
		}
	}

	if err = session.WriteFile(ctx, workspacePrefix+runtime.ScriptName, []byte(req.Code)); err != nil {
		return exectool.RunResult{}, classifyControlPlaneError(ctx, err, "write script")
	}

	start := time.Now()
	result, err := session.StartAndWait(ctx, runtime.Command, req.Env)
	if err != nil {
		return exectool.RunResult{}, classifyControlPlaneError(ctx, err, "start process")
	}

	return exectool.RunResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
		Duration: time.Since(start),
	}, nil
}
