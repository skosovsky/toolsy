package toolsy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApprovalCannotDelegateAcrossSubjectScopeOrView(t *testing.T) {
	for _, changed := range []string{"subject", "scope", "view"} {
		t.Run(changed, func(t *testing.T) {
			// Arrange: both domains are ACL-authorized, but approval is for one binding.
			now := time.Now()
			calls, grantID := 0, ""
			store := NewMemoryOperationStore()
			profile, err := NewOperationProfile(
				OperationProfileConfig{
					Store: store,
					Prepare: func(_ context.Context, call PreparedCall) (OperationIntent, error) {
						return OperationIntent{
							Namespace:         "delegation",
							Subject:           call.Context.Subject.(string),
							Scope:             call.Context.Scope.(string),
							OperationID:       "intent",
							AttemptID:         call.Input.CallID,
							GrantID:           grantID,
							PolicyFingerprint: "allow both",
							CanonicalDigest:   "host dependencies",
							CanonicalRules:    "json",
							DisplayJSON:       []byte(`{}`),
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
			tool, err := NewTypedTool(
				TypedToolSpec[string, string, struct{}, string, string]{
					Name:        "write",
					Description: "Write",
					Options:     []ToolOption{WithRequiresConfirmation()},
					Policy:      func(context.Context, TypedPolicyRequest[string, string, struct{}]) Decision { return AllowDecision() },
					Handler: func(context.Context, TypedCallContext[string, string], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
						calls++
						return NewToolResult[string, string]("written"), nil
					},
				},
			)
			require.NoError(t, err)
			reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(tool).Build()
			require.NoError(t, err)
			first, err := reg.View(
				RegistryViewSpec{ToolNames: []string{"write"}, Owner: "original", Reason: "host delegation"},
			)
			require.NoError(t, err)
			second, err := reg.View(
				RegistryViewSpec{ToolNames: []string{"write"}, Owner: "another", Reason: "host delegation"},
			)
			require.NoError(t, err)
			call := ToolCall{
				ToolName:    "write",
				CallContext: NewCallContext("alice", "tenant-a"),
				Input:       ToolInput{CallID: "pending", ArgsJSON: []byte(`{}`)},
			}
			err = first.Execute(context.Background(), call, func(Chunk) error { return nil })
			var pending *PendingApprovalError
			require.ErrorAs(t, err, &pending)
			require.NoError(
				t,
				store.PutGrant(
					context.Background(),
					ApprovalGrant{
						ID:        "grant",
						Issuer:    "host",
						Binding:   pending.Challenge.Binding,
						IssuedAt:  now,
						ExpiresAt: now.Add(time.Hour),
					},
				),
			)
			grantID = "grant"
			execute := first.Execute
			switch changed {
			case "subject":
				call.CallContext.Subject = "bob"
			case "scope":
				call.CallContext.Scope = "tenant-b"
			case "view":
				execute = second.Execute
			}
			// Act: attempt to transfer the approval, while ACL still permits this caller.
			call.Input.CallID = "transfer"
			err = execute(
				context.Background(),
				call,
				func(Chunk) error { t.Fatal("transferred grant delivered"); return nil },
			)
			// Assert: binding fails before side effect and does not consume original approval.
			var mismatch *OperationError
			require.ErrorAs(t, err, &mismatch)
			assert.Equal(t, "binding_mismatch", mismatch.Kind)
			assert.Zero(t, calls)
			call.CallContext = NewCallContext("alice", "tenant-a")
			call.Input.CallID = "original-resume"
			require.NoError(t, first.Execute(context.Background(), call, func(Chunk) error { return nil }))
			assert.Equal(t, 1, calls)
		})
	}
}
