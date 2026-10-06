package toolsy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:gocognit // Keep the four adversarial Session scenarios and their shared host wiring together.
func TestNestedSuppliedSessionPreservesPolicyViewBudgetAndApproval(t *testing.T) {
	for _, mode := range []string{"policy", "view", "budget", "approval"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: the host supplies one scoped Session; parent has no root-registry escape.
			ctx := context.Background()
			now := time.Now()
			writes, hiddenCalls, parents := 0, 0, 0
			revoked, grantID, attempt := false, "", "challenge"
			write, err := NewTypedTool(TypedToolSpec[string, string, struct{}, string, string]{
				Name:        "write",
				Description: "Write",
				Options:     []ToolOption{WithRequiresConfirmation()},
				Policy: func(_ context.Context, req TypedPolicyRequest[string, string, struct{}]) Decision {
					if revoked || req.Context.Subject != "alice" || req.Context.Scope != "tenant" {
						return DenyDecision("revoked scope")
					}
					return AllowDecision()
				},
				Handler: func(context.Context, TypedCallContext[string, string], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
					writes++
					return NewToolResult[string, string]("written"), nil
				},
			})
			require.NoError(t, err)
			hidden, err := NewTool(
				"hidden",
				"Hidden",
				func(context.Context, *RunEnv, struct{}) (string, error) { hiddenCalls++; return "hidden", nil },
			)
			require.NoError(t, err)
			var supplied *Session
			parent, err := NewProxyTool(
				"parent",
				"Parent",
				[]byte(`{"type":"object","properties":{"child":{"type":"string"}},"required":["child"]}`),
				func(ctx context.Context, env *RunEnv, raw []byte, yield func(Chunk) error) error {
					parents++
					var args struct {
						Child string `json:"child"`
					}
					if decodeErr := json.Unmarshal(raw, &args); decodeErr != nil {
						return decodeErr
					}
					return supplied.Execute(
						ctx,
						ToolCall{ToolName: args.Child, Env: env, CallContext: env.CallContext(),
							Input: ToolInput{CallID: attempt, ArgsJSON: []byte(`{}`)}},
						yield,
					)
				},
			)
			require.NoError(t, err)
			store := NewMemoryOperationStore()
			operation, err := NewOperationProfile(
				OperationProfileConfig{
					Store: store,
					Prepare: func(_ context.Context, call PreparedCall) (OperationIntent, error) {
						return OperationIntent{
							Namespace:         "nested",
							Subject:           call.Context.Subject.(string),
							Scope:             call.Context.Scope.(string),
							OperationID:       "stable-write",
							AttemptID:         call.Input.CallID,
							GrantID:           grantID,
							PolicyFingerprint: "host policy",
							CanonicalDigest:   "host dependencies",
							CanonicalRules:    "json",
							DisplayJSON:       []byte(`{"action":"write"}`),
						}, nil
					},
					Codec:    JSONResultCodec[string, string]{},
					Issuer:   "host",
					Clock:    func() time.Time { return now },
					Lease:    time.Minute,
					MaxBytes: 0,
				},
			)
			require.NoError(t, err)
			// Host routing: only write is journaled; parent is a side-effect-free harness.
			profile := executionProfileFunc(
				func(ctx context.Context, call PreparedCall, invoke InvocationHandler, yield func(Chunk) error) error {
					if call.Manifest.Name != "write" {
						return invoke(yield)
					}
					return operation.ExecutePrepared(ctx, call, invoke, yield)
				},
			)
			reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(parent, write, hidden).Build()
			require.NoError(t, err)
			names, allowed := []string{"parent", "write"}, []string{"parent", "write"}
			if mode == "policy" {
				names = append(names, "hidden")
			}
			if mode == "view" {
				allowed = append(allowed, "hidden")
			}
			view, err := reg.View(RegistryViewSpec{ToolNames: names, Owner: "host", Reason: "nested test"})
			require.NoError(t, err)
			maxCalls := 20
			if mode == "budget" {
				maxCalls = 1
			}
			supplied, err = view.NewSession(WithMaxCalls(maxCalls), WithRunPolicy(RunPolicy{AllowedTools: allowed}))
			require.NoError(t, err)
			child := "write"
			if mode == "policy" || mode == "view" {
				child = "hidden"
			}
			raw, err := json.Marshal(map[string]string{"child": child})
			require.NoError(t, err)
			call := ToolCall{
				ToolName:    "parent",
				Env:         NewRunEnv(supplied),
				CallContext: NewCallContext("alice", "tenant"),
				Input:       ToolInput{CallID: "outer", ArgsJSON: raw},
			}
			// Act: nested dispatch uses the supplied Session, never Registry.Execute directly.
			err = supplied.Execute(ctx, call, func(Chunk) error { return nil })
			// Assert: denied selection/budget cannot reach any child handler or grant claim.
			switch mode {
			case "policy":
				require.ErrorContains(t, err, "not allowed by session run policy")
				assert.Equal(t, int64(1), supplied.Track().CallAttempts())
			case "view":
				require.ErrorIs(t, err, ErrCapabilityDenied)
				assert.Equal(t, int64(2), supplied.Track().CallAttempts())
			case "budget":
				require.ErrorIs(t, err, ErrMaxCallsExceeded)
				assert.Equal(t, int64(2), supplied.Track().CallAttempts())
			default:
				var pending *PendingApprovalError
				require.ErrorAs(t, err, &pending)
				assert.Zero(t, writes)
				require.NoError(
					t,
					store.PutGrant(
						ctx,
						ApprovalGrant{
							ID:        "grant",
							Issuer:    "host",
							Binding:   pending.Challenge.Binding,
							IssuedAt:  now,
							ExpiresAt: now.Add(time.Hour),
						},
					),
				)
				grantID, attempt = "grant", "resume"
				require.NoError(t, supplied.Execute(ctx, call, func(Chunk) error { return nil }))
				attempt = "replay"
				require.NoError(t, supplied.Execute(ctx, call, func(Chunk) error { return nil }))
				revoked = true
				attempt = "revoked"
				leaked := 0
				err = supplied.Execute(ctx, call, func(Chunk) error { leaked++; return nil })
				require.ErrorIs(t, err, ErrPolicyDenied)
				assert.Zero(t, leaked)
				assert.Equal(t, 1, writes)
				assert.Equal(t, 4, parents)
				assert.Equal(t, int64(8), supplied.Track().CallAttempts())
			}
			assert.Zero(t, hiddenCalls)
			if mode != "approval" {
				assert.Zero(t, writes)
				assert.Equal(t, 1, parents)
			}
		})
	}
}
