package toolsy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type consumerWriteArgs struct {
	Value string `json:"value"`
}

func exerciseOrdinaryConsumer[S, C any](
	t *testing.T, sharedTool toolsy.Tool, subject S, scope C,
	authorize func(S, C) bool, identity func(S, C) (string, string), useSession bool, effects *int,
) {
	t.Helper()
	// Arrange: only the host knows these domain types and identity mappings.
	ctx := context.Background()
	now := time.Now()
	allowed, grantID, policyRevision := false, "", "initial"
	store := toolsy.NewMemoryOperationStore()
	policies := 0
	wrapped, err := toolsy.NewPolicyTool(toolsy.ToolPolicySpec[S, C, consumerWriteArgs]{
		Tool: sharedTool,
		ArgsBinder: func(_ context.Context, req toolsy.ArgsBindRequest) (toolsy.ValidatedArgs[consumerWriteArgs], error) {
			var args consumerWriteArgs
			decodeErr := json.Unmarshal(req.Input.ArgsJSON, &args)
			return toolsy.ValidatedArgs[consumerWriteArgs]{Value: args, Raw: req.Input.ArgsJSON}, decodeErr
		},
		Policy: func(_ context.Context, req toolsy.TypedPolicyRequest[S, C, consumerWriteArgs]) toolsy.Decision {
			policies++
			if allowed && authorize(req.Context.Subject, req.Context.Scope) {
				return toolsy.AllowDecision()
			}
			return toolsy.DenyDecision("host ACL")
		},
	})
	require.NoError(t, err)
	profile, err := toolsy.NewOperationProfile(
		toolsy.OperationProfileConfig{
			Store: store,
			Prepare: func(_ context.Context, call toolsy.PreparedCall) (toolsy.OperationIntent, error) {
				typed, contextErr := toolsy.TypedContext[S, C](call.Context)
				if contextErr != nil {
					return toolsy.OperationIntent{}, contextErr
				}
				principal, partition := identity(typed.Subject, typed.Scope)
				return toolsy.OperationIntent{
					Namespace:         "ordinary",
					Subject:           principal,
					Scope:             partition,
					OperationID:       "write-intent",
					AttemptID:         call.Input.CallID,
					GrantID:           grantID,
					PolicyFingerprint: policyRevision,
					CanonicalDigest:   "host dependencies",
					CanonicalRules:    "json",
					DisplayJSON:       call.Input.ArgsJSON,
				}, nil
			},
			Codec:    toolsy.JSONResultCodec[string, string]{},
			Issuer:   "authenticated-host",
			Clock:    func() time.Time { return now },
			Lease:    time.Minute,
			MaxBytes: 0,
		},
	)
	require.NoError(t, err)
	reg, err := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(profile)).Add(wrapped).Build()
	require.NoError(t, err)
	execute := reg.Execute
	if useSession {
		session, sessionErr := toolsy.NewSession(
			reg,
			toolsy.WithRunPolicy(toolsy.RunPolicy{AllowedTools: []string{"write"}}),
		)
		require.NoError(t, sessionErr)
		execute = session.Execute
	}
	call := toolsy.ToolCall{ToolName: "write", CallContext: toolsy.NewCallContext(subject, scope),
		Input: toolsy.ToolInput{CallID: "denied", ArgsJSON: []byte(`{"value":"ordinary"}`)}}
	initialEffects := *effects
	// Act/Assert: denial cannot touch the shared tool.
	err = execute(ctx, call, func(toolsy.Chunk) error { t.Fatal("denied consumer output"); return nil })
	require.ErrorIs(t, err, toolsy.ErrPolicyDenied)
	assert.Equal(t, initialEffects, *effects)
	allowed = true
	call.Input.CallID = "challenge"
	err = execute(ctx, call, func(toolsy.Chunk) error { return nil })
	var pending *toolsy.PendingApprovalError
	require.ErrorAs(t, err, &pending)
	assert.Equal(t, initialEffects, *effects)
	require.NoError(
		t,
		store.PutGrant(
			ctx,
			toolsy.ApprovalGrant{ID: "grant", Issuer: "authenticated-host", Binding: pending.Challenge.Binding,
				IssuedAt: now, ExpiresAt: now.Add(time.Hour)},
		),
	)
	grantID = "grant"
	// Act/Assert: a policy revision invalidates an otherwise current grant.
	policyRevision = "changed"
	call.Input.CallID = "stale"
	err = execute(ctx, call, func(toolsy.Chunk) error { t.Fatal("stale approval output"); return nil })
	var stale *toolsy.OperationError
	require.ErrorAs(t, err, &stale)
	assert.Equal(t, "binding_mismatch", stale.Kind)
	assert.Equal(t, initialEffects, *effects)
	// Act: restore exact binding, execute and repeat delivery.
	policyRevision = "initial"
	call.Input.CallID = "approved"
	require.NoError(t, execute(ctx, call, func(toolsy.Chunk) error { return nil }))
	call.Input.CallID = "repeat"
	require.NoError(t, execute(ctx, call, func(c toolsy.Chunk) error {
		assert.Equal(t, toolsy.ReplaySourceOperation, c.ToolEnvelope().Metadata[toolsy.ReplaySourceMetadata])
		return nil
	}))
	// Assert: the same generic Tool worked with host-owned types, no agent loop.
	assert.Equal(t, initialEffects+1, *effects)
	assert.Equal(t, 5, policies)
}

func TestTwoOrdinaryBYOTConsumersShareOneTool(t *testing.T) {
	// Arrange: one library-independent tool instance, two unrelated host domains.
	effects := 0
	shared, err := toolsy.NewTool(
		"write",
		"Write",
		func(_ context.Context, _ *toolsy.RunEnv, args consumerWriteArgs) (string, error) {
			effects++
			return args.Value, nil
		},
		toolsy.WithRequiresConfirmation(),
	)
	require.NoError(t, err)
	type operator struct {
		ID       string
		CanWrite bool
	}
	type project struct{ Name string }
	type servicePrincipal string
	type warehouse struct {
		Account int
		Region  string
	}
	// Act/Assert: user/project through registry and service/warehouse through Session.
	t.Run("operator-registry", func(t *testing.T) {
		exerciseOrdinaryConsumer(t, shared, operator{ID: "operator", CanWrite: true}, project{Name: "project"},
			func(s operator, c project) bool { return s.CanWrite && c.Name == "project" },
			func(s operator, c project) (string, string) { return s.ID, c.Name }, false, &effects)
	})
	t.Run("service-session", func(t *testing.T) {
		exerciseOrdinaryConsumer(t, shared, servicePrincipal("service"), warehouse{Account: 42, Region: "local"},
			func(s servicePrincipal, c warehouse) bool { return s == "service" && c.Account == 42 },
			func(s servicePrincipal, c warehouse) (string, string) {
				return string(s), fmt.Sprintf("%d/%s", c.Account, c.Region)
			}, true, &effects)
	})
	assert.Equal(t, 2, effects)
}
