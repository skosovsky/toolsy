package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func noopProxyHandler(context.Context, *toolsy.RunEnv, []byte, func(toolsy.Chunk) error) error {
	return nil
}

func TestMcpToolPolicyOptions_ReadOnly(t *testing.T) {
	// Arrange.
	opts := mcpToolPolicyOptions(&ToolAnnotations{ReadOnlyHint: new(true)})
	require.Len(t, opts, 1)

	// Act.
	tool, err := toolsy.NewProxyTool(
		"t",
		"d",
		[]byte(`{"type":"object"}`),
		noopProxyHandler,
		opts...)

	// Assert.
	require.NoError(t, err)
	require.True(t, tool.Manifest().ReadOnly)
}

func TestMcpToolPolicyOptions_Destructive(t *testing.T) {
	// Arrange.
	opts := mcpToolPolicyOptions(&ToolAnnotations{DestructiveHint: new(true)})
	require.Len(t, opts, 1)

	// Act.
	tool, err := toolsy.NewProxyTool(
		"t",
		"d",
		[]byte(`{"type":"object"}`),
		noopProxyHandler,
		opts...)

	// Assert.
	require.NoError(t, err)
	require.True(t, tool.Manifest().Dangerous)
}

func TestMcpToolPolicyOptions_Idempotent(t *testing.T) {
	// Arrange.
	opts := mcpToolPolicyOptions(&ToolAnnotations{IdempotentHint: new(true)})
	require.Len(t, opts, 2)

	// Act.
	tool, err := toolsy.NewProxyTool(
		"t",
		"d",
		[]byte(`{"type":"object"}`),
		noopProxyHandler,
		opts...)

	// Assert.
	require.NoError(t, err)
	require.True(t, tool.Manifest().Idempotent)
	require.True(t, tool.Manifest().Dangerous)
}

func TestMcpToolPolicyOptions_NilAnnotations(t *testing.T) {
	// Arrange.
	opts := mcpToolPolicyOptions(nil)
	require.Len(t, opts, 1)

	// Act.
	tool, err := toolsy.NewProxyTool(
		"t",
		"d",
		[]byte(`{"type":"object"}`),
		noopProxyHandler,
		opts...)

	// Assert.
	require.NoError(t, err)
	require.True(t, tool.Manifest().Dangerous)
}

func TestMcpToolPolicyOptions_OpenWorldHintIgnored(t *testing.T) {
	// Arrange.
	annotations := &ToolAnnotations{OpenWorldHint: new(true)}

	// Act.
	opts := mcpToolPolicyOptions(annotations)

	// Assert.
	require.Len(t, opts, 1)
}

func TestMcpToolPolicyOptions_ExplicitNonDestructive(t *testing.T) {
	// Arrange.
	annotations := &ToolAnnotations{DestructiveHint: new(false)}

	// Act.
	opts := mcpToolPolicyOptions(annotations)

	// Assert.
	require.Empty(t, opts)
}

func TestGetResourceTool_ReadOnlyManifest(t *testing.T) {
	// Arrange.
	client := &Client{
		ready: true,
		server: DiscoverResult{Capabilities: ServerCapabilities{
			Resources: &ResourcesCapability{},
		}},
	}

	// Act.
	tool, err := client.GetResourceTool()

	// Assert.
	require.NoError(t, err)
	require.True(t, tool.Manifest().ReadOnly)
}
