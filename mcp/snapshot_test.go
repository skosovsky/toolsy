package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComputeSnapshotDigestSupportsEveryCacheableSnapshot(t *testing.T) {
	// Arrange.
	cache := CacheInfo{TTLMS: JSONNumber("10"), CacheScope: CacheScopePrivate}
	snapshots := []Snapshot{
		DiscoverResult{
			ResultType:        ResultTypeComplete,
			SupportedVersions: []string{ProtocolVersion},
			Capabilities:      ServerCapabilities{},
			CacheInfo:         cache,
		},
		ToolsListResult{ResultType: ResultTypeComplete, Tools: []MCPTool{}, CacheInfo: cache},
		ResourcesListResult{ResultType: ResultTypeComplete, Resources: []Resource{}, CacheInfo: cache},
		ResourceTemplatesListResult{
			ResultType:        ResultTypeComplete,
			ResourceTemplates: []ResourceTemplate{},
			CacheInfo:         cache,
		},
		ResourcesReadResult{ResultType: ResultTypeComplete, Contents: []ResourceContents{}, CacheInfo: cache},
		PromptsListResult{ResultType: ResultTypeComplete, Prompts: []Prompt{}, CacheInfo: cache},
	}

	// Act.
	digests := make(map[SnapshotDigest]struct{}, len(snapshots))
	for _, snapshot := range snapshots {
		digest, err := ComputeSnapshotDigest(snapshot)
		require.NoError(t, err)
		digests[digest] = struct{}{}
		require.Len(t, digest.String(), 64)
	}

	// Assert.
	require.Len(t, digests, len(snapshots), "snapshot type must separate otherwise similar payloads")
}

func TestComputeSnapshotDigestCanonicalizesMapsNumbersAndPointers(t *testing.T) {
	// Arrange.
	left := ToolsListResult{
		ResultType: ResultTypeComplete,
		Tools:      []MCPTool{},
		TTLMS:      JSONNumber("1000"), CacheScope: CacheScopePrivate,
		Extra: Meta{"vendor.example/data": json.RawMessage(`{"b":2.0,"a":1e0}`)},
	}
	right := ToolsListResult{
		ResultType: ResultTypeComplete,
		Tools:      []MCPTool{},
		TTLMS:      JSONNumber("1e3"), CacheScope: CacheScopePrivate,
		Extra: Meta{"vendor.example/data": json.RawMessage(` { "a" : 1.00, "b" : 2 } `)},
	}

	// Act.
	leftDigest, leftErr := ComputeSnapshotDigest(left)
	rightDigest, rightErr := ComputeSnapshotDigest(&right)

	// Assert.
	require.NoError(t, leftErr)
	require.NoError(t, rightErr)
	require.Equal(t, leftDigest, rightDigest)
}

func TestComputeSnapshotDigestIncludesCacheMetadataAndWireOrder(t *testing.T) {
	// Arrange.
	base := ToolsListResult{
		ResultType: ResultTypeComplete,
		Tools: []MCPTool{
			{Name: "a", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "b", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		TTLMS: JSONNumber("10"), CacheScope: CacheScopePrivate,
	}
	differentTTL := base
	differentTTL.TTLMS = JSONNumber("11")
	differentScope := base
	differentScope.CacheScope = CacheScopePublic
	differentOrder := base
	differentOrder.Tools = []MCPTool{base.Tools[1], base.Tools[0]}

	// Act.
	baseDigest, baseErr := ComputeSnapshotDigest(base)
	ttlDigest, ttlErr := ComputeSnapshotDigest(differentTTL)
	scopeDigest, scopeErr := ComputeSnapshotDigest(differentScope)
	orderDigest, orderErr := ComputeSnapshotDigest(differentOrder)

	// Assert.
	require.NoError(t, baseErr)
	require.NoError(t, ttlErr)
	require.NoError(t, scopeErr)
	require.NoError(t, orderErr)
	require.NotEqual(t, baseDigest, ttlDigest)
	require.NotEqual(t, baseDigest, scopeDigest)
	require.NotEqual(t, baseDigest, orderDigest)
}

func TestComputeSnapshotDigestRejectsInvalidAndNilSnapshots(t *testing.T) {
	// Arrange.
	invalid := ToolsListResult{
		ResultType: ResultTypeComplete,
		Tools:      []MCPTool{},
		TTLMS:      JSONNumber("0"), CacheScope: CacheScopePrivate,
		Extra: Meta{"vendor.example/data": json.RawMessage(`{"x":1,"x":2}`)},
	}
	var nilSnapshot *ToolsListResult

	// Act.
	_, invalidErr := ComputeSnapshotDigest(invalid)
	_, nilErr := ComputeSnapshotDigest(nilSnapshot)

	// Assert.
	require.Error(t, invalidErr)
	require.ErrorContains(t, invalidErr, "duplicate")
	require.Error(t, nilErr)
}

func TestComputeSnapshotDigestRejectsOversizedEncodingBeforeCanonicalization(t *testing.T) {
	// Arrange.
	snapshot := ToolsListResult{
		ResultType: ResultTypeComplete,
		Tools: []MCPTool{{
			Name:        "oversized",
			Description: strings.Repeat("x", maxSnapshotEncodingBytes),
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		TTLMS: JSONNumber("0"), CacheScope: CacheScopePrivate,
	}

	// Act.
	_, err := ComputeSnapshotDigest(snapshot)

	// Assert.
	require.ErrorContains(t, err, "maximum size")
}
