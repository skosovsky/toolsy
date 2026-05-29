package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func noopProxyHandler(context.Context, toolsy.RunContext, []byte, func(toolsy.Chunk) error) error {
	return nil
}

func TestMcpToolPolicyOptions_ReadOnly(t *testing.T) {
	opts := mcpToolPolicyOptions(&ToolAnnotations{ReadOnlyHint: new(true)})
	require.Len(t, opts, 1)
	tool, err := toolsy.NewProxyTool("t", "d", []byte(`{"type":"object"}`), noopProxyHandler, opts...)
	require.NoError(t, err)
	require.True(t, tool.Manifest().ReadOnly)
}

func TestMcpToolPolicyOptions_Destructive(t *testing.T) {
	opts := mcpToolPolicyOptions(&ToolAnnotations{DestructiveHint: new(true)})
	require.Len(t, opts, 1)
	tool, err := toolsy.NewProxyTool("t", "d", []byte(`{"type":"object"}`), noopProxyHandler, opts...)
	require.NoError(t, err)
	require.True(t, tool.Manifest().Dangerous)
}

func TestMcpToolPolicyOptions_Idempotent(t *testing.T) {
	opts := mcpToolPolicyOptions(&ToolAnnotations{IdempotentHint: new(true)})
	require.Len(t, opts, 1)
	tool, err := toolsy.NewProxyTool("t", "d", []byte(`{"type":"object"}`), noopProxyHandler, opts...)
	require.NoError(t, err)
	require.True(t, tool.Manifest().Idempotent)
}

func TestMcpToolPolicyOptions_NilAnnotations(t *testing.T) {
	require.Nil(t, mcpToolPolicyOptions(nil))
}

func TestMcpToolPolicyOptions_OpenWorldHintIgnored(t *testing.T) {
	opts := mcpToolPolicyOptions(&ToolAnnotations{OpenWorldHint: new(true)})
	require.Empty(t, opts)
}

func TestGetResourceTool_ReadOnlyManifest(t *testing.T) {
	client := &Client{transport: &connectCaptureTransport{}}
	tool, err := client.GetResourceTool()
	require.NoError(t, err)
	require.True(t, tool.Manifest().ReadOnly)
}
