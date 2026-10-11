package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestSDKImportUnknownEffectOutcomeNeverRedispatches(t *testing.T) {
	// Arrange: remote call effects, then returns invalid output under a claimed operation.
	var calls atomic.Int32
	importer, _ := adapterPair(t, echoServer(&calls, `{"n":"invalid"}`), true, importConfig(), nil)
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	profile, err := toolsy.NewOperationProfile(toolsy.OperationProfileConfig{
		Store: toolsy.NewMemoryOperationStore(), Issuer: "host", Clock: time.Now, Lease: time.Minute,
		Codec: toolsy.JSONResultCodec[*sdk.CallToolResult, string]{},
		Prepare: func(_ context.Context, call toolsy.PreparedCall) (toolsy.OperationIntent, error) {
			return toolsy.OperationIntent{
				Namespace:         "remote",
				Scope:             "tenant",
				Subject:           "host",
				OperationID:       "effect",
				AttemptID:         call.Input.CallID,
				PolicyFingerprint: "current",
				CanonicalDigest:   "fixed",
				CanonicalRules:    "json",
				DisplayJSON:       []byte(`{}`),
			}, nil
		},
	})
	require.NoError(t, err)
	registry, err := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(profile)).Add(snapshot.Tools...).Build()
	require.NoError(t, err)
	call := toolsy.ToolCall{
		ToolName: "echo",
		Input:    toolsy.ToolInput{CallID: "attempt-one", ArgsJSON: []byte(`{"n":1}`)},
	}
	// Act.
	first := registry.Execute(
		t.Context(),
		call,
		func(toolsy.Chunk) error { t.Fatal("invalid output delivered"); return nil },
	)
	call.Input.CallID = "attempt-two"
	second := registry.Execute(
		t.Context(),
		call,
		func(toolsy.Chunk) error { t.Fatal("unknown outcome replayed"); return nil },
	)
	// Assert.
	var outcome *toolsy.OperationOutcomeError
	var state *toolsy.OperationStateError
	require.ErrorAs(t, first, &outcome)
	require.ErrorAs(t, second, &state)
	require.Equal(t, toolsy.OperationUnknown, state.Record.State)
	require.EqualValues(t, 1, calls.Load())
}

func TestSDKExportApprovalBindingReplayAndRevoke(t *testing.T) {
	// Arrange: only a trusted host store can issue the approval grant.
	var calls atomic.Int32
	var revoked atomic.Bool
	var grant atomic.Value
	grant.Store("")
	var challenge atomic.Pointer[toolsy.ApprovalChallenge]
	store := toolsy.NewMemoryOperationStore()
	local, err := toolsy.NewProxyTool("write", "write", []byte(objectSchema),
		func(_ context.Context, _ *toolsy.RunEnv, args []byte, yield func(toolsy.Chunk) error) error {
			calls.Add(1)
			return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: args, MimeType: toolsy.MimeTypeJSON})
		}, toolsy.WithRequiresConfirmation(), toolsy.WithOutputSchema(map[string]any{"type": "object"}))
	require.NoError(t, err)
	profile, err := toolsy.NewOperationProfile(toolsy.OperationProfileConfig{
		Store: store, Issuer: "host", Clock: time.Now, Lease: time.Minute, Codec: toolsy.JSONResultCodec[any, string]{},
		Prepare: func(_ context.Context, call toolsy.PreparedCall) (toolsy.OperationIntent, error) {
			return toolsy.OperationIntent{
				Namespace:         "export",
				Scope:             "tenant",
				Subject:           "host",
				OperationID:       "write",
				AttemptID:         call.Input.CallID,
				GrantID:           grant.Load().(string),
				PolicyFingerprint: "current",
				CanonicalDigest:   "fixed",
				CanonicalRules:    "json",
				DisplayJSON:       []byte(`{}`),
			}, nil
		},
	})
	require.NoError(t, err)
	registry, err := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(profile), toolsy.WithPolicy("current", toolsy.PolicyFunc(func(context.Context, toolsy.PolicyRequest) toolsy.Decision {
		if revoked.Load() {
			return toolsy.DenyDecision("revoked")
		}
		return toolsy.AllowDecision()
	}))).
		Add(local).
		Build()
	require.NoError(t, err)
	view, err := registry.View(toolsy.RegistryViewSpec{ToolNames: []string{"write"}, Owner: "tenant"})
	require.NoError(t, err)
	config := ExportConfig{
		Names:          []string{"write"},
		Implementation: &sdk.Implementation{Name: "approval", Version: "1"},
		Resolve: func(ctx context.Context, _ sdk.Request) (toolsy.CallContext, error) {
			info := auth.TokenInfoFromContext(ctx)
			if info == nil || info.UserID != "host" {
				return toolsy.CallContext{}, auth.ErrInvalidToken
			}
			return toolsy.NewCallContext(info.UserID, "tenant"), nil
		},
		Control: func(_ context.Context, _ *sdk.CallToolRequest, err error) (*sdk.CallToolResult, error) {
			var pending *toolsy.PendingApprovalError
			if !errors.As(err, &pending) {
				return nil, err
			}
			challenge.Store(&pending.Challenge)
			return &sdk.CallToolResult{InputRequests: sdk.InputRequestMap{}, RequestState: "host-approval-needed"}, nil
		},
	}
	server, err := Export(view, config)
	require.NoError(t, err)
	session := authenticatedSDKPair(t, server)
	params := &sdk.CallToolParams{
		Name:      "write",
		Arguments: json.RawMessage(`{"n":7}`),
		Meta:      sdk.Meta{"approval": "forged"},
	}
	// Act: model text and state do not grant approval.
	result, err := session.CallTool(t.Context(), params)
	// Assert.
	require.NoError(t, err)
	require.True(t, result.NeedsInput())
	require.Zero(t, calls.Load())
	_, err = session.CallTool(
		t.Context(),
		&sdk.CallToolParams{Name: "write", Arguments: json.RawMessage(`{"n":7}`), RequestState: "host-approval-needed"},
	)
	require.NoError(t, err)
	require.Zero(t, calls.Load())
	// Act: host issues a grant bound to the original canonical arguments.
	now := time.Now()
	require.NoError(
		t,
		store.PutGrant(
			t.Context(),
			toolsy.ApprovalGrant{
				ID:        "approved",
				Issuer:    "host",
				Binding:   challenge.Load().Binding,
				IssuedAt:  now,
				ExpiresAt: now.Add(time.Hour),
			},
		),
	)
	grant.Store("approved")
	_, changed := session.CallTool(
		t.Context(),
		&sdk.CallToolParams{Name: "write", Arguments: json.RawMessage(`{"n":8}`)},
	)
	_, approved := session.CallTool(
		t.Context(),
		&sdk.CallToolParams{Name: "write", Arguments: json.RawMessage(`{"n":7}`)},
	)
	_, replay := session.CallTool(
		t.Context(),
		&sdk.CallToolParams{Name: "write", Arguments: json.RawMessage(`{"n":7}`)},
	)
	revoked.Store(true)
	_, denied := session.CallTool(
		t.Context(),
		&sdk.CallToolParams{Name: "write", Arguments: json.RawMessage(`{"n":7}`)},
	)
	// Assert: changed args and revoked authorization fail; replay causes no second effect.
	require.Error(t, changed)
	require.NoError(t, approved)
	require.NoError(t, replay)
	require.Error(t, denied)
	require.EqualValues(t, 1, calls.Load())
}

func TestSDKResourcesTemplatesPromptsAndResourceTool(t *testing.T) {
	// Arrange: SDK owns the ordinary resource/prompt APIs, with a tool projection for reads.
	server := sdk.NewServer(&sdk.Implementation{Name: "resources", Version: "1"}, nil)
	read := func(context.Context, *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		return &sdk.ReadResourceResult{
			Contents: []*sdk.ResourceContents{
				{
					URI:      "resource://one",
					MIMEType: "application/octet-stream",
					Blob:     []byte{0, 1, 2},
					Meta:     sdk.Meta{"vendor.example/tag": "keep"},
				},
			},
		}, nil
	}
	server.AddResource(&sdk.Resource{URI: "resource://one", Name: "one", MIMEType: "application/octet-stream"}, read)
	server.AddResourceTemplate(&sdk.ResourceTemplate{URITemplate: "resource://{name}", Name: "template"}, read)
	server.AddPrompt(
		&sdk.Prompt{Name: "hello"},
		func(context.Context, *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
			return &sdk.GetPromptResult{
				Messages: []*sdk.PromptMessage{{Role: "user", Content: &sdk.TextContent{Text: "hello"}}},
			}, nil
		},
	)
	importer, session := adapterPair(t, server, true, importConfig(), nil)
	resource, err := importer.ResourceTool()
	require.NoError(t, err)
	// Act.
	resources, listErr := session.ListResources(t.Context(), nil)
	templates, templateErr := session.ListResourceTemplates(t.Context(), nil)
	prompts, promptErr := session.ListPrompts(t.Context(), nil)
	message, getErr := session.GetPrompt(t.Context(), &sdk.GetPromptParams{Name: "hello"})
	chunks, toolErr := execute(t, resource, `{"uri":"resource://one"}`)
	// Assert.
	require.NoError(t, listErr)
	require.NoError(t, templateErr)
	require.NoError(t, promptErr)
	require.NoError(t, getErr)
	require.NoError(t, toolErr)
	require.Len(t, resources.Resources, 1)
	require.Len(t, templates.ResourceTemplates, 1)
	require.Len(t, prompts.Prompts, 1)
	require.Equal(t, "hello", message.Messages[0].Content.(*sdk.TextContent).Text)
	retained := chunks[0].TypedResult.(*sdk.ReadResourceResult)
	require.Equal(t, []byte{0, 1, 2}, retained.Contents[0].Blob)
	require.Equal(t, "keep", retained.Contents[0].Meta["vendor.example/tag"])
}

func TestSDKUnsupportedExportModesRejectedBeforeEffects(t *testing.T) {
	// Arrange: manifest declares independent stream or confirmation.
	for _, option := range []toolsy.ToolOption{toolsy.WithIndependentStream(), toolsy.WithRequiresConfirmation()} {
		var calls atomic.Int32
		local, err := toolsy.NewProxyTool("tool", "tool", []byte(`{"type":"object"}`),
			func(context.Context, *toolsy.RunEnv, []byte, func(toolsy.Chunk) error) error {
				calls.Add(1)
				return nil
			}, option)
		require.NoError(t, err)
		registry, err := toolsy.NewRegistry(local)
		require.NoError(t, err)
		view, err := registry.View(toolsy.RegistryViewSpec{ToolNames: []string{"tool"}})
		require.NoError(t, err)
		// Act.
		_, err = Export(
			view,
			ExportConfig{Names: []string{"tool"}, Implementation: &sdk.Implementation{Name: "test", Version: "1"},
				Resolve: func(context.Context, sdk.Request) (toolsy.CallContext, error) {
					return toolsy.NewCallContext("host", "tenant"), nil
				}},
		)
		// Assert.
		require.ErrorIs(t, err, ErrUnsupported)
		require.Zero(t, calls.Load())
	}
}
