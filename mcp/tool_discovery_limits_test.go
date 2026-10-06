package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFullToolDiscoveryBudgetsPreserveExistingAuthority(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limits PaginationLimits
	}{
		{"items", PaginationLimits{MaxItems: 1}},
		{"bytes", PaginationLimits{MaxBytes: 220}},
		{"pages", PaginationLimits{MaxPages: 1}},
		{"cursors", PaginationLimits{MaxCursorBytes: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: a valid old snapshot, then two pages exceeding one aggregate bound.
			phase := false
			base := newContractTransport(func(method string, raw json.RawMessage) (json.RawMessage, error) {
				if method == MethodServerDiscover {
					return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil
				}
				var params ToolsListParams
				require.NoError(t, json.Unmarshal(raw, &params))
				name, next := "old", ""
				if phase {
					name, next = "new", "second"
					if params.Cursor != "" {
						name, next = "last", ""
					}
				}
				return json.Marshal(
					ToolsListResult{
						ResultType: ResultTypeComplete,
						TTLMS:      JSONNumber("0"),
						CacheScope: CacheScopePrivate,
						Tools:      []MCPTool{{Name: name, InputSchema: json.RawMessage(`{"type":"object"}`)}},
						NextCursor: next,
					},
				)
			})
			transport := &bindingContractTransport{
				contractTransport: base,
				bindings:          make(map[string][]HTTPToolHeaderBinding),
			}
			client, err := Connect(t.Context(), transport, WithPaginationLimits(tc.limits))
			require.NoError(t, err)
			defer client.Close()
			snapshot, err := client.DiscoverTools(t.Context())
			require.NoError(t, err)
			require.Equal(t, "old", snapshot.Tools[0].Name)
			phase = true
			// Act.
			snapshot, err = client.DiscoverTools(t.Context())
			// Assert: no partial snapshot or authority replacement escapes.
			require.Error(t, err)
			require.Empty(t, snapshot.Tools)
			transport.bindingsMu.Lock()
			require.Contains(t, transport.bindings, "old")
			require.NotContains(t, transport.bindings, "new")
			transport.bindingsMu.Unlock()
		})
	}
}

func TestToolDiscoveryItemBudgetPrecedesSchemaCompilation(t *testing.T) {
	// Arrange: the second descriptor has an invalid schema, but the page is over budget.
	base := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		if method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil
		}
		return json.RawMessage(
			`{"resultType":"complete","ttlMs":0,"cacheScope":"private","tools":[{"name":"one","inputSchema":{"type":"object"}},{"name":"bad","inputSchema":{"type":"invalid"}}]}`,
		), nil
	})
	client, err := Connect(t.Context(), base, WithPaginationLimits(PaginationLimits{MaxItems: 1}))
	require.NoError(t, err)
	defer client.Close()
	// Act.
	_, err = client.DiscoverTools(t.Context())
	// Assert.
	require.ErrorContains(t, err, "aggregate discovery item limit")
	require.Empty(t, client.toolBindings)
}

func TestToolDiscoveryInvalidInputSchemaCannotPublishAuthority(t *testing.T) {
	// Arrange.
	base := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		if method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil
		}
		return json.RawMessage(
			`{"resultType":"complete","ttlMs":0,"cacheScope":"private","tools":[{"name":"bad","inputSchema":{"type":"invalid"}}]}`,
		), nil
	})
	transport := &bindingContractTransport{contractTransport: base, bindings: make(map[string][]HTTPToolHeaderBinding)}
	client, err := Connect(t.Context(), transport)
	require.NoError(t, err)
	defer client.Close()
	// Act.
	_, err = client.DiscoverTools(t.Context())
	// Assert.
	require.ErrorContains(t, err, "inputSchema")
	require.Empty(t, client.toolBindings)
	require.Empty(t, transport.bindings)
}

func TestClientNegativeAndNilOptionsFailBeforeTransportStart(t *testing.T) {
	for _, option := range []ClientOption{
		nil,
		WithPaginationLimits(PaginationLimits{MaxPages: -1}),
		WithPaginationLimits(PaginationLimits{MaxCursorBytes: -1}),
		WithPaginationLimits(PaginationLimits{MaxItems: -1}),
		WithPaginationLimits(PaginationLimits{MaxBytes: -1}),
		WithSubscriptionLimits(SubscriptionLimits{MaxActive: -1}),
		WithSubscriptionLimits(SubscriptionLimits{MaxTracked: -1}),
		WithSubscriptionLimits(SubscriptionLimits{RetireTTL: -1}),
		WithSubscriptionLimits(SubscriptionLimits{MaxActive: 2, MaxTracked: 1}),
	} {
		// Arrange.
		transport := &connectCaptureTransport{}
		// Act.
		client, err := Connect(t.Context(), transport, option)
		// Assert.
		require.Error(t, err)
		require.Nil(t, client)
		require.Zero(t, transport.startCalls)
	}
}

type publicationBarrierPort struct {
	*bindingContractTransport

	entered chan struct{}
	resume  chan struct{}
	block   bool
}

func (p *publicationBarrierPort) ReplaceToolHeaderBindings(bindings map[string][]HTTPToolHeaderBinding) error {
	if p.block {
		close(p.entered)
		<-p.resume
	}
	return p.bindingContractTransport.ReplaceToolHeaderBindings(bindings)
}
func TestDiscoveryCancellationAfterCommitStarts(t *testing.T) {
	// Arrange: full discovery blocks inside the trusted publication facet.
	name := "old"
	base := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		if method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil
		}
		return json.Marshal(
			ToolsListResult{
				ResultType: ResultTypeComplete,
				TTLMS:      JSONNumber("0"),
				CacheScope: CacheScopePrivate,
				Tools:      []MCPTool{{Name: name, InputSchema: json.RawMessage(`{"type":"object"}`)}},
			},
		)
	})
	port := &publicationBarrierPort{
		bindingContractTransport: &bindingContractTransport{
			contractTransport: base,
			bindings:          make(map[string][]HTTPToolHeaderBinding),
		},
		entered: make(chan struct{}),
		resume:  make(chan struct{}),
	}
	c, err := Connect(t.Context(), port)
	require.NoError(t, err)
	defer c.Close()
	_, err = c.DiscoverTools(t.Context())
	require.NoError(t, err)
	name = "new"
	port.block = true
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	// Act.
	go func() { _, discoveryErr := c.DiscoverTools(ctx); done <- discoveryErr }()
	<-port.entered
	cancel()
	close(port.resume)
	err = <-done
	// Assert: cancellation after the commit point cannot undo the successful snapshot.
	require.NoError(t, err)
	require.Contains(t, port.bindings, "new")
	require.NotContains(t, port.bindings, "old")
}

func TestDiscoveryCanceledBeforePublicationRetainsAuthority(t *testing.T) {
	// Arrange: cancellation must be rechecked inside the authority lock.
	base := newContractTransport(func(string, json.RawMessage) (json.RawMessage, error) { return nil, nil })
	port := &bindingContractTransport{contractTransport: base, bindings: make(map[string][]HTTPToolHeaderBinding)}
	c := &Client{
		transport:    port,
		toolBindings: map[string]struct{}{"old": {}},
		toolSchemas:  make(map[string]schemaValidator),
	}
	old := []toolBindingCandidate{{descriptor: MCPTool{Name: "old"}}}
	require.NoError(t, c.publishToolAuthority(t.Context(), old, 0))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Act.
	err := c.publishToolAuthority(ctx, []toolBindingCandidate{{descriptor: MCPTool{Name: "new"}}}, 0)
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Contains(t, c.toolBindings, "old")
	require.Contains(t, port.bindings, "old")
	require.NotContains(t, port.bindings, "new")
}
