package prompts

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type mockProvider struct {
	text string
	err  error
}

func (m *mockProvider) Get(ctx context.Context, roleID string, variables map[string]any) (string, error) {
	_ = ctx
	_ = roleID
	_ = variables
	if m.err != nil {
		return "", m.err
	}
	return m.text, nil
}

// verifyingProvider records the last roleID and variables passed to Get for test assertions.
type verifyingProvider struct {
	text       string
	lastRoleID string
	lastVars   map[string]any
	callCount  int
}

func (v *verifyingProvider) Get(ctx context.Context, roleID string, variables map[string]any) (string, error) {
	_ = ctx
	v.callCount++
	v.lastRoleID = roleID
	v.lastVars = variables
	return v.text, nil
}

func decodePromptResult(t *testing.T, c toolsy.Chunk) getResult {
	t.Helper()
	require.Equal(t, toolsy.MimeTypeJSON, c.MimeType)
	var out getResult
	require.NoError(t, json.Unmarshal(c.Data, &out))
	return out
}

func TestAsTool_RoleAndVariables(t *testing.T) {
	p := &verifyingProvider{text: "You are a doctor for Ivan."}
	tool, err := AsTool(p)
	require.NoError(t, err)
	var result string
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.RunContext{},
			toolsy.ToolInput{ArgsJSON: []byte(`{"role_id":"doctor","variables":{"patient_name":"Ivan"}}`)},
			func(c toolsy.Chunk) error {
				result = decodePromptResult(t, c).Instructions
				return nil
			},
		),
	)
	require.Equal(t, "You are a doctor for Ivan.", result)
	require.Equal(t, 1, p.callCount)
	require.Equal(t, "doctor", p.lastRoleID)
	require.NotNil(t, p.lastVars)
	require.Equal(t, "Ivan", p.lastVars["patient_name"])
}

func TestAsTool_ProviderError(t *testing.T) {
	p := &mockProvider{err: errors.New("not found")}
	tool, err := AsTool(p)
	require.NoError(t, err)
	err = tool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"role_id":"x"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	cause := err
	for cause != nil {
		if strings.Contains(cause.Error(), "toolkit/prompts:") {
			return
		}
		cause = errors.Unwrap(cause)
	}
	t.Errorf("expected toolkit/prompts in error chain, got %q", err.Error())
}

func TestAsTool_WithNameAndDescription(t *testing.T) {
	p := &mockProvider{text: "Hi"}
	tool, err := AsTool(p, WithName("get_role"), WithDescription("Fetch role instructions"))
	require.NoError(t, err)
	require.Equal(t, "get_role", tool.Manifest().Name)
	require.Equal(t, "Fetch role instructions", tool.Manifest().Description)
}

func TestAsTool_MaxBytesTruncate(t *testing.T) {
	longText := strings.Repeat("x", 100)
	p := &mockProvider{text: longText}
	tool, err := AsTool(p, WithMaxBytes(20))
	require.NoError(t, err)
	var result string
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.RunContext{},
			toolsy.ToolInput{ArgsJSON: []byte(`{"role_id":"r"}`)},
			func(c toolsy.Chunk) error {
				result = decodePromptResult(t, c).Instructions
				return nil
			},
		),
	)
	require.True(t, strings.HasSuffix(result, "[Truncated]"), "expected [Truncated] suffix, got %q", result)
}
