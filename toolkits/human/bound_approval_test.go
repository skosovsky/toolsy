package human

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type boundActionArgs struct {
	Note string `json:"note"`
}

func TestConversationPauseDoesNotGrantAction(t *testing.T) {
	// Arrange: the host authenticates identity, owns the action policy and issues grants.
	ctx := context.Background()
	now := time.Now()
	store := toolsy.NewMemoryOperationStore()
	grantID := ""
	revoked := false
	dispatched := []string{}
	profile, err := toolsy.NewOperationProfile(
		store,
		func(_ context.Context, call toolsy.PreparedCall) (toolsy.OperationIntent, error) {
			typed, contextErr := toolsy.TypedContext[string, string](call.Context)
			if contextErr != nil {
				return toolsy.OperationIntent{}, contextErr
			}
			digest := sha256.Sum256(call.Input.ArgsJSON)
			return toolsy.OperationIntent{
				Namespace: "notes", Subject: typed.Subject, Scope: typed.Scope,
				OperationID: "host-intent", AttemptID: call.Input.CallID, GrantID: grantID,
				PolicyFingerprint: "host-acl", CanonicalDigest: hex.EncodeToString(digest[:]),
				CanonicalRules: "prepared-json", DisplayJSON: call.Input.ArgsJSON,
			}, nil
		},
		toolsy.JSONResultCodec[string, string]{},
		"authenticated-host",
		func() time.Time { return now },
		time.Minute,
		0,
	)
	require.NoError(t, err)
	action, err := toolsy.NewTypedTool(toolsy.TypedToolSpec[string, string, boundActionArgs, string, string]{
		Name:        "write_note",
		Description: "Write a host-authorized note",
		Options:     []toolsy.ToolOption{toolsy.WithRequiresConfirmation()},
		Policy: func(_ context.Context, request toolsy.TypedPolicyRequest[string, string, boundActionArgs]) toolsy.Decision {
			if revoked || request.Context.Subject != "authenticated-user" {
				return toolsy.DenyDecision("host access denied")
			}
			return toolsy.AllowDecision()
		},
		Handler: func(_ context.Context, _ toolsy.TypedCallContext[string, string], _ *toolsy.RunEnv, args toolsy.ValidatedArgs[boundActionArgs]) (toolsy.ToolResult[string, string], error) {
			dispatched = append(dispatched, args.Value.Note)
			return toolsy.NewToolResult[string, string]("written"), nil
		},
	})
	require.NoError(t, err)
	registry, err := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(profile)).Add(action).Build()
	require.NoError(t, err)
	conversation, err := AsTools()
	require.NoError(t, err)
	yield := func(toolsy.Chunk) error { return nil }
	call := toolsy.ToolCall{
		ToolName: "write_note", CallContext: toolsy.NewCallContext("authenticated-user", "workspace"),
		Input: toolsy.ToolInput{CallID: "initial", ArgsJSON: []byte(`{"note":"approved unchanged <你好>"}`)},
	}

	// Act: a conversation pause alone cannot dispatch the action.
	err = conversation[0].Execute(ctx, nil, toolsy.ToolInput{
		ArgsJSON: []byte(`{"action":"write_note","reason":"the user said yes"}`),
	}, func(chunk toolsy.Chunk) error {
		var intent map[string]string
		require.NoError(t, json.Unmarshal([]byte(chunk.Control.(*toolsy.PauseSignal).Reason), &intent))
		require.Equal(
			t,
			map[string]string{"kind": "human_review", "action": "write_note", "reason": "the user said yes"},
			intent,
		)
		return nil
	})
	require.ErrorIs(t, err, toolsy.ErrPause)
	var pending *toolsy.PendingApprovalError
	require.ErrorAs(t, registry.Execute(ctx, call, yield), &pending)
	require.Empty(t, dispatched)
	require.JSONEq(t, string(call.Input.ArgsJSON), string(pending.Challenge.DisplayJSON))
	// Only the authenticated host can populate its operation store with this exact binding.
	require.NoError(t, store.PutGrant(ctx, toolsy.ApprovalGrant{
		ID: "host-grant", Issuer: "authenticated-host", Binding: pending.Challenge.Binding,
		IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	}))
	grantID = "host-grant"
	call.Input.CallID = "resume"
	require.NoError(t, registry.Execute(ctx, call, yield))
	call.Input.CallID = "replay"
	require.NoError(t, registry.Execute(ctx, call, yield))
	revoked = true
	call.Input.CallID = "revoked"
	err = registry.Execute(ctx, call, yield)

	// Assert: exact approved action once; even a completed replay rechecks current rights.
	require.Equal(t, []string{"approved unchanged <你好>"}, dispatched)
	require.ErrorIs(t, err, toolsy.ErrPolicyDenied)
}
