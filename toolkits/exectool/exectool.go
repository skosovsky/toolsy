package exectool

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/skosovsky/toolsy"
)

const (
	languagePython = "python"
	languageBash   = "bash"
)

const truncateSuffix = "\n[Truncated]"

// Result holds the outcome of a sandbox run.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Sandbox is implemented by the orchestrator (e.g. Docker, Lambda, E2B).
// The toolkit never executes code on the host.
type Sandbox interface {
	Run(ctx context.Context, language, code string) (*Result, error)
}

type codeArgs struct {
	Code string `json:"code"`
}

type runResult struct {
	Output string `json:"output"`
}

// AsTools returns tools for each enabled language (WithPython, WithBash).
// At least one language must be enabled or an error is returned.
func AsTools(sandbox Sandbox, opts ...Option) ([]toolsy.Tool, error) {
	if sandbox == nil {
		return nil, fmt.Errorf("toolkit/exectool: sandbox is nil")
	}
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)

	if !o.enablePython && !o.enableBash {
		return nil, fmt.Errorf("toolkit/exectool: at least one of WithPython or WithBash must be set")
	}

	var tools []toolsy.Tool
	if o.enablePython {
		t, err := toolsy.NewTool[codeArgs, runResult](
			o.pythonName,
			o.pythonDesc,
			func(ctx context.Context, args codeArgs) (runResult, error) {
				return doRun(ctx, sandbox, languagePython, args.Code, o.maxOutputBytes)
			},
		)
		if err != nil {
			return nil, fmt.Errorf("toolkit/exectool: build python tool: %w", err)
		}
		tools = append(tools, t)
	}
	if o.enableBash {
		t, err := toolsy.NewTool[codeArgs, runResult](
			o.bashName,
			o.bashDesc,
			func(ctx context.Context, args codeArgs) (runResult, error) {
				return doRun(ctx, sandbox, languageBash, args.Code, o.maxOutputBytes)
			},
		)
		if err != nil {
			return nil, fmt.Errorf("toolkit/exectool: build bash tool: %w", err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}

func doRun(ctx context.Context, sandbox Sandbox, language, code string, maxOutputBytes int) (runResult, error) {
	if strings.TrimSpace(code) == "" {
		return runResult{}, &toolsy.ClientError{Reason: "code is required", Err: toolsy.ErrValidation}
	}
	res, err := sandbox.Run(ctx, language, code)
	if err != nil {
		return runResult{}, fmt.Errorf("toolkit/exectool: sandbox run: %w", err)
	}
	if res == nil {
		return runResult{}, fmt.Errorf("toolkit/exectool: sandbox returned nil result")
	}
	out := formatResult(res, maxOutputBytes)
	return runResult{Output: out}, nil
}

// formatResult formats stdout/stderr so empty blocks are omitted (clearer for LLM).
func formatResult(res *Result, maxBytes int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Exit Code: %d\n", res.ExitCode)
	if res.Stdout != "" {
		b.WriteString("Stdout:\n")
		b.WriteString(truncateUTF8(res.Stdout, maxBytes))
		b.WriteString("\n")
	}
	if res.Stderr != "" {
		b.WriteString("Stderr:\n")
		b.WriteString(truncateUTF8(res.Stderr, maxBytes))
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// truncateUTF8 returns s truncated to at most maxBytes (UTF-8 safe). If s exceeds maxBytes,
// content is cut and truncateSuffix is appended only when it fits (so total length never exceeds maxBytes).
func truncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	suffixLen := len(truncateSuffix)
	contentMax := maxBytes - suffixLen
	if contentMax <= 0 {
		// Suffix does not fit; truncate to maxBytes without suffix (UTF-8 safe)
		return truncateToBytes(s, maxBytes)
	}
	n := 0
	for _, r := range s {
		rn := utf8.RuneLen(r)
		if n+rn > contentMax {
			return s[:n] + truncateSuffix
		}
		n += rn
	}
	return s
}

// truncateToBytes returns s truncated to at most maxBytes without cutting a UTF-8 rune in half.
func truncateToBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && maxBytes < len(s) && (s[maxBytes]&0xC0) == 0x80 {
		maxBytes--
	}
	return s[:maxBytes]
}
