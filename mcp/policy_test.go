package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
