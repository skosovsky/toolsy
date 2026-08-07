package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTask34PaginationPreservesChangingPerPageCacheSnapshots(t *testing.T) {
	// Arrange.
	listCalls := 0
	transport := newContractTransport(func(method string, raw json.RawMessage) (json.RawMessage, error) {
		if method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil
		}
		require.Equal(t, MethodToolsList, method)
		listCalls++
		var params ToolsListParams
		require.NoError(t, json.Unmarshal(raw, &params))
		result := ToolsListResult{
			ResultType: ResultTypeComplete,
			Tools: []MCPTool{{
				Name:        "first",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			}},
			NextCursor: "page-2",
			CacheInfo:  CacheInfo{TTLMS: JSONNumber("10"), CacheScope: CacheScopePrivate},
		}
		if params.Cursor == "page-2" {
			result.Tools[0].Name = "second"
			result.NextCursor = ""
			result.TTLMS = JSONNumber("20")
			result.CacheScope = CacheScopePublic
		}
		encoded, err := json.Marshal(result)
		return encoded, err
	})
	client, err := Connect(t.Context(), transport)
	require.NoError(t, err)
	defer client.Close()

	// Act.
	first, firstErr := client.ListTools(t.Context(), "")
	second, secondErr := client.ListTools(t.Context(), first.NextCursor)
	firstDigest, firstDigestErr := ComputeSnapshotDigest(first)
	secondDigest, secondDigestErr := ComputeSnapshotDigest(second)
	iterated := 0
	var iterationErr error
	for _, itemErr := range client.GetTools(context.Background()) {
		if itemErr != nil {
			iterationErr = itemErr
			break
		}
		iterated++
	}

	// Assert.
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	require.NoError(t, firstDigestErr)
	require.NoError(t, secondDigestErr)
	require.Equal(t, JSONNumber("10"), first.TTLMS)
	require.Equal(t, CacheScopePrivate, first.CacheScope)
	require.Equal(t, JSONNumber("20"), second.TTLMS)
	require.Equal(t, CacheScopePublic, second.CacheScope)
	require.NotEqual(t, firstDigest, secondDigest)
	require.NoError(t, iterationErr)
	require.Equal(t, 2, iterated)
	require.Equal(t, 4, listCalls, "explicit snapshots and iterator must fetch both pages without hidden reuse")
}
