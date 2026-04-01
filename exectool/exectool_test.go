package exectool

import (
	"context"
	"encoding/json"
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
	tool, err := New(sb)
	require.NoError(t, err)

	params := tool.Manifest().Parameters
	props := params["properties"].(map[string]any)
	language := props["language"].(map[string]any)
	require.Equal(t, []any{"bash", "python"}, language["enum"])
	_, hasTimeout := props["timeout"]
	require.False(t, hasTimeout)
}

func TestNewAllowedLanguagesIntersection(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python", "bash", "node"}}
	tool, err := New(sb, WithAllowedLanguages("python", "bash"))
	require.NoError(t, err)

	params := tool.Manifest().Parameters
	props := params["properties"].(map[string]any)
	language := props["language"].(map[string]any)
	require.Equal(t, []any{"bash", "python"}, language["enum"])
}

func TestNewAllowedLanguagesMustIntersect(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	_, err := New(sb, WithAllowedLanguages("bash"))
	require.Error(t, err)
}

func TestNewSucceeds(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	tool, err := New(sb)
	require.NoError(t, err)
	err = tool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"language":"python","code":"x"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
}

func TestNewRejectsNilSandbox(t *testing.T) {
	_, err := New(nil)
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
	tool, err := New(sb)
	require.NoError(t, err)

	var result RunResult
	err = tool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{
			ArgsJSON: []byte(`{"language":"python","code":"print(1)","env":{"A":"B"},"files":{"main.txt":"hello"}}`),
		},
		func(c toolsy.Chunk) error {
			return json.Unmarshal(c.Data, &result)
		},
	)
	require.NoError(t, err)
	require.Equal(t, "hello", result.Stdout)
	require.Equal(t, "python", sb.lastReq.Language)
	require.Equal(t, "print(1)", sb.lastReq.Code)
	require.Equal(t, "B", sb.lastReq.Env["A"])
	require.Equal(t, []byte("hello"), sb.lastReq.Files["main.txt"])
}

func TestExecuteEmptyCodeReturnsClientError(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	tool, err := New(sb)
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"language":"python","code":"   "}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestExecuteRejectsUnsupportedLanguageBeforeSandbox(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	tool, err := New(sb, WithAllowedLanguages("python"))
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"language":"bash","code":"echo 1"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
}

func TestExecuteSucceedsWithinShortContextDeadline(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	tool, err := New(sb)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err = tool.Execute(
		ctx,
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"language":"python","code":"print(1)"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
}

func TestExecuteReturnsTimeoutWhenContextExpired(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python"}}
	tool, err := New(sb)
	require.NoError(t, err)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	err = tool.Execute(
		ctx,
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"language":"python","code":"print(1)"}`)},
		func(toolsy.Chunk) error { return nil },
	)
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
	tool, err := New(sb)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = tool.Execute(
		ctx,
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"language":"python","code":"print(1)"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsSystemError(err))
	require.ErrorIs(t, err, ErrTimeout)
	require.Less(t, time.Since(start), 200*time.Millisecond)
}

func TestExecutePreservesSandboxSentinels(t *testing.T) {
	sb := &mockSandbox{
		languages: []string{"python"},
		err:       ErrTimeout,
	}
	tool, err := New(sb)
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"language":"python","code":"print(1)"}`)},
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
		WithToolOptions(toolsy.WithDangerous(), toolsy.WithMetadata(map[string]any{"requires_confirmation": true})),
	)
	require.NoError(t, err)

	meta := tool.Manifest().Metadata
	require.Equal(t, true, meta["dangerous"])
	require.Equal(t, true, meta["requires_confirmation"])
}

func TestNewRejectsEmptyLanguageNames(t *testing.T) {
	sb := &mockSandbox{languages: []string{"python", ""}}
	_, err := New(sb)
	require.Error(t, err)
}

func TestExecuteWrapsSandboxErrorChain(t *testing.T) {
	rootErr := fmt.Errorf("boom: %w", ErrSandboxFailure)
	sb := &mockSandbox{languages: []string{"python"}, err: rootErr}
	tool, err := New(sb)
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"language":"python","code":"print(1)"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsSystemError(err))
	require.ErrorIs(t, err, ErrSandboxFailure)
	require.NotErrorIs(t, err, ErrUnsupportedLanguage)
}
