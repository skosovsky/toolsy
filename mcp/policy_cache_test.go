package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestRemoteHintsRequireHostTrustForCacheAndCurrentPolicy(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		t.Run(map[bool]string{false: "untrusted", true: "host-approved"}[trusted], func(t *testing.T) {
			// Arrange: server hints claim permissions; only the host can classify them.
			transport := newFakeTransport()
			calls := 0
			descriptor := MCPTool{
				Name:        "remote",
				InputSchema: json.RawMessage(`{"type":"object"}`),
				Annotations: &ToolAnnotations{
					ReadOnlyHint:    new(true),
					IdempotentHint:  new(true),
					DestructiveHint: new(false),
				},
			}
			transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
				switch request.Method {
				case MethodServerDiscover:
					return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil, true
				case MethodToolsList:
					return mustJSON(
						t,
						ToolsListResult{
							ResultType: ResultTypeComplete,
							Tools:      []MCPTool{descriptor},
							TTLMS:      JSONNumber("0"),
							CacheScope: CacheScopePrivate,
						},
					), nil, true
				case MethodToolsCall:
					calls++
					return mustJSON(
						t,
						CallToolResult{
							ResultType: ResultTypeComplete,
							Content:    []ContentBlock{{Type: "text", Text: "approved result"}},
						},
					), nil, true
				default:
					return nil, errors.New("unexpected method"), true
				}
			}
			type authorityKey struct{}
			ctx := context.WithValue(context.Background(), authorityKey{}, "host decision")
			opts := []ClientOption{quietConnectLogger()}
			if trusted {
				opts = append(
					opts,
					WithToolPolicyMapper(
						func(hostCtx context.Context, source MCPTool) (ToolExecutionProperties, error) {
							require.Equal(t, "host decision", hostCtx.Value(authorityKey{}))
							require.Equal(t, descriptor.Annotations, source.Annotations)
							source.Name = "attacker"
							require.Equal(t, "attacker", source.Name)
							*source.Annotations.IdempotentHint = false
							return ToolExecutionProperties{ReadOnly: true, Idempotent: true}, nil
						},
					),
				)
			}
			client, err := Connect(ctx, transport, opts...)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			var proxy toolsy.Tool
			for item, iterErr := range client.GetTools(ctx) {
				require.NoError(t, iterErr)
				proxy = item
			}
			require.NotNil(t, proxy)
			source, err := client.ListTools(ctx, "")
			require.NoError(t, err)
			require.True(t, *source.Tools[0].Annotations.IdempotentHint)
			require.Equal(t, trusted, proxy.Manifest().Idempotent)
			require.Equal(t, trusted, proxy.Manifest().ReadOnly)
			require.Equal(t, !trusted, proxy.Manifest().Dangerous)
			denied := false
			policyCalls := 0
			cache, err := toolsy.NewResultCache(
				toolsy.NewMemoryResultCacheStore(),
				func(context.Context, toolsy.PreparedCall) (string, error) { return "host-owned-tenant", nil },
				toolsy.JSONResultCodec[CallToolResult, string]{},
				0,
			)
			require.NoError(t, err)
			profile := &generationBoundaryProfile{next: cache}
			registry, err := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(profile), toolsy.WithPolicy("current-host-policy", toolsy.PolicyFunc(func(context.Context, toolsy.PolicyRequest) toolsy.Decision {
				policyCalls++
				if denied {
					return toolsy.DenyDecision("revoked")
				}
				return toolsy.AllowDecision()
			}))).
				Add(proxy).
				Build()
			require.NoError(t, err)
			call := toolsy.ToolCall{ToolName: "remote", Input: toolsy.ToolInput{ArgsJSON: json.RawMessage(`{}`)}}
			chunks := []toolsy.Chunk{}
			yield := func(chunk toolsy.Chunk) error { chunks = append(chunks, chunk); return nil }
			// Act: allowed invocations followed by current host revocation.
			require.NoError(t, registry.Execute(ctx, call, yield))
			require.NoError(t, registry.Execute(ctx, call, yield))
			denied = true
			delivered := len(chunks)
			err = registry.Execute(ctx, call, yield)
			// Assert: hints never enable replay, and current deny prevents handler/replay delivery.
			require.ErrorIs(t, err, toolsy.ErrPolicyDenied)
			require.Len(t, chunks, delivered)
			require.Equal(t, 3, policyCalls)
			denied = false
			profile.before = func() { client.toolGeneration.Add(1) }
			// Invalidation after authorization but before profile replay must still
			// reject cached delivery. No new tools/call may be dispatched.
			err = registry.Execute(ctx, call, yield)
			var stale *StaleDiscoveryError
			require.ErrorAs(t, err, &stale)
			require.Len(t, chunks, delivered)
			if trusted {
				require.Equal(t, 1, calls)
				require.Equal(t, true, chunks[1].ToolEnvelope().Metadata[toolsy.CacheReplayMetadata])
			} else {
				require.Equal(t, 2, calls)
				require.NotContains(t, chunks[1].ToolEnvelope().Metadata, toolsy.CacheReplayMetadata)
			}
		})
	}
}

func TestToolPolicyMapperRefusalPreventsProxyDelivery(t *testing.T) {
	// Arrange.
	refusal := errors.New("host does not trust server")
	client := &Client{
		opts: ClientOptions{ToolPolicyMapper: func(context.Context, MCPTool) (ToolExecutionProperties, error) {
			return ToolExecutionProperties{}, refusal
		}},
	}
	descriptor := MCPTool{
		Name:        "remote",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Annotations: &ToolAnnotations{ReadOnlyHint: new(true)},
	}
	// Act.
	proxy, err := client.toolToProxyAtGeneration(context.Background(), descriptor, 0)
	// Assert.
	require.Nil(t, proxy)
	require.ErrorIs(t, err, refusal)
}

// generationBoundaryProfile deterministically places an invalidation between
// the prepared boundary and cache replay, without sleeps or goroutine races.
type generationBoundaryProfile struct {
	next   toolsy.ExecutionProfile
	before func()
}

func (p *generationBoundaryProfile) ExecutePrepared(
	ctx context.Context,
	call toolsy.PreparedCall,
	invoke toolsy.InvocationHandler,
	yield func(toolsy.Chunk) error,
) error {
	if p.before != nil {
		p.before()
	}
	return p.next.ExecutePrepared(ctx, call, invoke, yield)
}
