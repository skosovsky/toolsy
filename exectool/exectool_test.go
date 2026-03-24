package exectool

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type mockSandbox struct {
	languages []string
	result    RunResult
	err       error
	lastReq   RunRequest
	runFn     func(context.Context, RunRequest) (RunResult, error)
}

func (m *mockSandbox) SupportedLanguages() []string {
	return append([]string(nil), m.languages...)
}

func (m *mockSandbox) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	m.lastReq = req
	if m.runFn != nil {
		return m.runFn(ctx, req)
	}
	if m.err != nil {
		return RunResult{}, m.err
	}
	return m.result, nil
}

func TestNewBuildsDynamicSchema(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python", "bash", "python"}}
	tool, err := New(sb, WithTimeout(5*time.Second))
	require.NoError(t, err)

	params := tool.Parameters()
	props := params["properties"].(map[string]any)
	language := props["language"].(map[string]any)
	require.Equal(t, []any{"bash", "python"}, language["enum"])
	_, hasTimeout := props["timeout"]
	require.False(t, hasTimeout)
}

func TestNewAllowedLanguagesIntersection(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python", "bash", "node"}}
	tool, err := New(sb, WithTimeout(5*time.Second), WithAllowedLanguages("python", "bash"))
	require.NoError(t, err)

	params := tool.Parameters()
	props := params["properties"].(map[string]any)
	language := props["language"].(map[string]any)
	require.Equal(t, []any{"bash", "python"}, language["enum"])
}

func TestNewAllowedLanguagesMustIntersect(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	_, err := New(sb, WithTimeout(5*time.Second), WithAllowedLanguages("bash"))
	require.Error(t, err)
}

func TestNewRequiresTimeout(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	_, err := New(sb)
	require.Error(t, err)
	require.Contains(t, err.Error(), "execution timeout is required for safety")
	require.Contains(t, err.Error(), "exectool.WithTimeout")
}

func TestNewRejectsNilSandbox(t *testing.T) {
	_, err := New(nil, WithTimeout(5*time.Second))
	require.Error(t, err)
}

func TestExecuteSuccess(t *testing.T) {
	sb := &mockSandbox{
		languages: []string{"python"},
		result: RunResult{
			Stdout:   "hello",
			Stderr:   "",
			ExitCode: 0,
		},
	}
	tool, err := New(sb, WithTimeout(2*time.Second))
	require.NoError(t, err)

	var result RunResult
	err = tool.Execute(
		context.Background(),
		[]byte(`{"language":"python","code":"print(1)","env":{"A":"B"},"files":{"main.txt":"hello"}}`),
		func(c toolsy.Chunk) error {
			result = c.RawData.(RunResult)
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, "hello", result.Stdout)
	require.Equal(t, "python", sb.lastReq.Language)
	require.Equal(t, "print(1)", sb.lastReq.Code)
	require.Equal(t, "B", sb.lastReq.Env["A"])
	require.Equal(t, []byte("hello"), sb.lastReq.Files["main.txt"])
	require.Equal(t, 2*time.Second, sb.lastReq.Timeout)
}

func TestExecuteEmptyCodeReturnsClientError(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	tool, err := New(sb, WithTimeout(5*time.Second))
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		[]byte(`{"language":"python","code":"   "}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestExecuteRejectsUnsupportedLanguageBeforeSandbox(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	tool, err := New(sb, WithTimeout(5*time.Second), WithAllowedLanguages("python"))
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		[]byte(`{"language":"bash","code":"echo 1"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
}

func TestExecuteClampsTimeoutToContextDeadline(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	tool, err := New(sb, WithTimeout(5*time.Second))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err = tool.Execute(ctx, []byte(`{"language":"python","code":"print(1)"}`), func(toolsy.Chunk) error { return nil })
	require.NoError(t, err)
	require.Greater(t, sb.lastReq.Timeout, time.Duration(0))
	require.Less(t, sb.lastReq.Timeout, 5*time.Second)
}

func TestExecuteReturnsTimeoutWhenContextExpired(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	tool, err := New(sb, WithTimeout(time.Second))
	require.NoError(t, err)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	err = tool.Execute(ctx, []byte(`{"language":"python","code":"print(1)"}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsSystemError(err))
	require.ErrorIs(t, err, ErrTimeout)
}

func TestExecuteEnforcesTimeoutViaContext(t *testing.T) {
	sb := &mockSandbox{
		languages: []string{"python"},
		runFn: func(ctx context.Context, _ RunRequest) (RunResult, error) {
			<-ctx.Done()
			return RunResult{}, ctx.Err()
		},
	}
	tool, err := New(sb, WithTimeout(20*time.Millisecond))
	require.NoError(t, err)

	start := time.Now()
	err = tool.Execute(
		context.Background(),
		[]byte(`{"language":"python","code":"print(1)"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsSystemError(err))
	require.ErrorIs(t, err, ErrTimeout)
	require.Equal(t, 20*time.Millisecond, sb.lastReq.Timeout)
	require.Less(t, time.Since(start), 200*time.Millisecond)
}

func TestExecutePreservesSandboxSentinels(t *testing.T) {
	sb := &mockSandbox{
		languages: []string{"python"},
		err:       ErrTimeout,
	}
	tool, err := New(sb, WithTimeout(5*time.Second))
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		[]byte(`{"language":"python","code":"print(1)"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsSystemError(err))
	require.ErrorIs(t, err, ErrTimeout)
}

func TestExecutePropagatesToolOptionMetadata(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	tool, err := New(
		sb,
		WithTimeout(time.Second),
		WithToolOptions(toolsy.WithDangerous(), toolsy.WithRequiresConfirmation()),
	)
	require.NoError(t, err)

	meta, ok := tool.(toolsy.ToolMetadata)
	require.True(t, ok)
	require.True(t, meta.IsDangerous())
	require.True(t, meta.RequiresConfirmation())
}

func TestNewRejectsEmptyLanguageNames(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python", ""}}
	_, err := New(sb, WithTimeout(time.Second))
	require.Error(t, err)
}

func TestExecuteWrapsSandboxErrorChain(t *testing.T) {
	rootErr := fmt.Errorf("boom: %w", ErrSandboxFailure)
	sb := &mockSandbox{languages: []string{"python"}, err: rootErr}
	tool, err := New(sb, WithTimeout(time.Second))
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		[]byte(`{"language":"python","code":"print(1)"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsSystemError(err))
	require.ErrorIs(t, err, ErrSandboxFailure)
	require.NotErrorIs(t, err, ErrUnsupportedLanguage)
}
