package exectool

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type mockSandbox struct {
	result   *Result
	err      error
	language string // captures last Run(language, ...) for routing tests
}

func (m *mockSandbox) Run(ctx context.Context, language, code string) (*Result, error) {
	_ = ctx
	_ = code
	m.language = language
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func TestExecPython_Success(t *testing.T) {
	sb := &mockSandbox{result: &Result{Stdout: "hello", Stderr: "", ExitCode: 0}}
	tools, err := AsTools(sb, WithPython())
	require.NoError(t, err)
	require.Len(t, tools, 1)

	var output string
	require.NoError(t, tools[0].Execute(context.Background(), []byte(`{"code":"print(1)"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(runResult); ok {
				output = r.Output
			}
		}
		return nil
	}))
	require.Contains(t, output, "Exit Code: 0")
	require.Contains(t, output, "Stdout:")
	require.Contains(t, output, "hello")
	require.NotContains(t, output, "Stderr:")
}

func TestExecBash_Success(t *testing.T) {
	sb := &mockSandbox{result: &Result{Stdout: "ok", Stderr: "", ExitCode: 0}}
	tools, err := AsTools(sb, WithBash())
	require.NoError(t, err)
	require.Len(t, tools, 1)

	var output string
	require.NoError(t, tools[0].Execute(context.Background(), []byte(`{"code":"echo ok"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(runResult); ok {
				output = r.Output
			}
		}
		return nil
	}))
	require.Contains(t, output, "Exit Code: 0")
	require.Contains(t, output, "ok")
}

func TestExec_EmptyStdoutNonEmptyStderr_OnlyStderrBlock(t *testing.T) {
	sb := &mockSandbox{result: &Result{Stdout: "", Stderr: "error message", ExitCode: 1}}
	tools, err := AsTools(sb, WithPython())
	require.NoError(t, err)

	var output string
	require.NoError(t, tools[0].Execute(context.Background(), []byte(`{"code":"x"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(runResult); ok {
				output = r.Output
			}
		}
		return nil
	}))
	require.Contains(t, output, "Exit Code: 1")
	require.Contains(t, output, "Stderr:")
	require.Contains(t, output, "error message")
	require.NotContains(t, output, "Stdout:")
}

func TestExec_NoLanguageEnabled_Error(t *testing.T) {
	sb := &mockSandbox{}
	_, err := AsTools(sb)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one")
}

func TestExec_NilSandbox_Error(t *testing.T) {
	_, err := AsTools(nil, WithPython())
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

func TestExec_EmptyCode_ClientError(t *testing.T) {
	sb := &mockSandbox{result: &Result{}}
	tools, err := AsTools(sb, WithPython())
	require.NoError(t, err)

	err = tools[0].Execute(context.Background(), []byte(`{"code":"   "}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "code")
}

func TestExec_SandboxError_Wrapped(t *testing.T) {
	sb := &mockSandbox{err: errors.New("timeout")}
	tools, err := AsTools(sb, WithPython())
	require.NoError(t, err)

	err = tools[0].Execute(context.Background(), []byte(`{"code":"x"}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	// Framework may wrap the error; ensure the chain contains our message or the original
	msg := err.Error()
	require.True(t, strings.Contains(msg, "toolkit/exectool") || strings.Contains(msg, "timeout") || strings.Contains(msg, "internal system error"),
		"error message should mention toolkit, timeout, or system: %s", msg)
}

func TestExec_BothLanguages_TwoTools(t *testing.T) {
	sb := &mockSandbox{result: &Result{Stdout: "1", ExitCode: 0}}
	tools, err := AsTools(sb, WithPython(), WithBash())
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "exec_python", tools[0].Name())
	require.Equal(t, "exec_bash", tools[1].Name())
}

func TestExec_PythonTool_PassesPythonToSandbox(t *testing.T) {
	sb := &mockSandbox{result: &Result{Stdout: "ok", ExitCode: 0}}
	tools, err := AsTools(sb, WithPython())
	require.NoError(t, err)
	require.NoError(t, tools[0].Execute(context.Background(), []byte(`{"code":"print(1)"}`), func(toolsy.Chunk) error { return nil }))
	require.Equal(t, "python", sb.language)
}

func TestExec_BashTool_PassesBashToSandbox(t *testing.T) {
	sb := &mockSandbox{result: &Result{Stdout: "ok", ExitCode: 0}}
	tools, err := AsTools(sb, WithBash())
	require.NoError(t, err)
	require.NoError(t, tools[0].Execute(context.Background(), []byte(`{"code":"echo 1"}`), func(toolsy.Chunk) error { return nil }))
	require.Equal(t, "bash", sb.language)
}

func TestExec_Truncation(t *testing.T) {
	large := strings.Repeat("x", 1000)
	sb := &mockSandbox{result: &Result{Stdout: large, Stderr: "", ExitCode: 0}}
	tools, err := AsTools(sb, WithPython(), WithMaxOutputBytes(50))
	require.NoError(t, err)

	var output string
	require.NoError(t, tools[0].Execute(context.Background(), []byte(`{"code":"print('x'*1000)"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(runResult); ok {
				output = r.Output
			}
		}
		return nil
	}))
	require.Contains(t, output, "[Truncated]")
	require.Greater(t, len(output), 50)
}

func TestExec_Truncation_SmallLimit_NeverExceedsMaxBytes(t *testing.T) {
	// When maxOutputBytes is smaller than len("\n[Truncated]"), truncation must still respect the limit (no suffix).
	sb := &mockSandbox{result: &Result{Stdout: "abcdefghij", Stderr: "", ExitCode: 0}}
	tools, err := AsTools(sb, WithPython(), WithMaxOutputBytes(5))
	require.NoError(t, err)

	var result runResult
	require.NoError(t, tools[0].Execute(context.Background(), []byte(`{"code":"x"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(runResult); ok {
				result = r
			}
		}
		return nil
	}))
	// Stdout section shows at most 5 bytes of content (suffix doesn't fit)
	require.Contains(t, result.Output, "Stdout:")
	idx := strings.Index(result.Output, "Stdout:")
	afterStdout := result.Output[idx+len("Stdout:\n"):]
	if idxStderr := strings.Index(afterStdout, "Stderr:"); idxStderr >= 0 {
		afterStdout = afterStdout[:idxStderr]
	}
	stdoutPart := strings.TrimSuffix(strings.TrimSpace(afterStdout), "[Truncated]")
	stdoutPart = strings.TrimSpace(stdoutPart)
	require.LessOrEqual(t, len(stdoutPart), 5, "stdout content must not exceed 5 bytes when maxOutputBytes=5")
}

func TestTruncateUTF8_SmallLimit_NoSuffix(t *testing.T) {
	// Suffix is 14 bytes; for maxBytes=5 we must not append it.
	out := truncateUTF8("hello world", 5)
	require.LessOrEqual(t, len(out), 5)
	require.Equal(t, "hello", out)
}

func TestTruncateUTF8_LimitExactlySuffixLength(t *testing.T) {
	// When maxBytes equals suffix length (14), no room for content; result is truncated to 14 bytes max.
	out := truncateUTF8("ab", 14)
	require.LessOrEqual(t, len(out), 14)
	require.Equal(t, "ab", out)
}
