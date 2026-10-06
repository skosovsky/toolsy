package toolsy

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApprovalUsesCanonicalRedactedSnapshotAndSecretFreshness(t *testing.T) {
	// Arrange: model input never supplies a secret; host resolves an opaque reference.
	type args struct {
		Count     int    `json:"count"`
		SecretRef string `json:"secret_ref"`
	}
	ctx := context.Background()
	now := time.Now()
	secret := "host-only-secret-never-disclosed"
	freshness, grantID := "opaque-first-revision", ""
	calls, binds := 0, 0
	tool, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, args, string, string]{
		Name:        "write",
		Description: "Write",
		Options:     []ToolOption{WithRequiresConfirmation()},
		ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[args], error) {
			binds++
			value := args{Count: 10, SecretRef: "trusted-reference"}
			raw, encodeErr := json.Marshal(value)
			return ValidatedArgs[args]{Value: value, Raw: raw}, encodeErr
		},
		Handler: func(_ context.Context, _ TypedCallContext[NoSubject, NoScope], _ *RunEnv, bound ValidatedArgs[args]) (ToolResult[string, string], error) {
			calls++
			assert.Equal(t, 10, bound.Value.Count)
			assert.Equal(t, "trusted-reference", bound.Value.SecretRef)
			assert.NotEmpty(t, secret) // Only the host would resolve/use this credential.
			return NewToolResult[string, string]("written"), nil
		},
	})
	require.NoError(t, err)
	store := NewMemoryOperationStore()
	profile, err := NewOperationProfile(OperationProfileConfig{
		Store: store,
		Prepare: func(_ context.Context, call PreparedCall) (OperationIntent, error) {
			var canonical args
			if decodeErr := json.Unmarshal(call.Input.ArgsJSON, &canonical); decodeErr != nil {
				return OperationIntent{}, decodeErr
			}
			assert.Equal(t, 10, canonical.Count)
			display, encodeErr := json.Marshal(struct {
				Count      int    `json:"count"`
				Credential string `json:"credential"`
			}{Count: canonical.Count, Credential: "redacted"})
			return OperationIntent{
				Namespace:         "writes",
				Scope:             "tenant",
				Subject:           "host",
				OperationID:       "intent",
				AttemptID:         call.Input.CallID,
				GrantID:           grantID,
				PolicyFingerprint: "policy",
				CanonicalDigest:   freshness,
				CanonicalRules:    "host reference freshness",
				DisplayJSON:       display,
			}, encodeErr
		},
		Codec:    JSONResultCodec[string, string]{},
		Issuer:   "host",
		Clock:    func() time.Time { return now },
		Lease:    time.Minute,
		MaxBytes: 0,
	})
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(tool).Build()
	require.NoError(t, err)
	call := ToolCall{
		ToolName: "write",
		Input:    ToolInput{CallID: "initial", ArgsJSON: []byte(`{"count":1,"secret_ref":"model-reference"}`)},
	}
	// Act: challenge uses normalized args; old approval is invalid after secret rotation.
	err = reg.Execute(ctx, call, func(Chunk) error { return nil })
	var pending *PendingApprovalError
	require.ErrorAs(t, err, &pending)
	require.JSONEq(t, `{"count":10,"credential":"redacted"}`, string(pending.Challenge.DisplayJSON))
	challengeJSON, encodeErr := json.Marshal(pending.Challenge)
	require.NoError(t, encodeErr)
	assert.NotContains(t, string(challengeJSON), secret)
	assert.NotContains(t, err.Error(), secret)
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
	grantID = "grant"
	freshness = "opaque-rotated-revision"
	call.Input.CallID = "rotated"
	err = reg.Execute(ctx, call, func(Chunk) error { t.Fatal("rotated secret must not dispatch/deliver"); return nil })
	// Assert: same canonical args do not reuse approval for a different credential revision.
	require.Error(t, err)
	var decision *OperationError
	require.ErrorAs(t, err, &decision)
	assert.Equal(t, "binding_mismatch", decision.Kind)
	assert.NotContains(t, err.Error(), secret)
	assert.Zero(t, calls)
	// Act/Assert: restoring the original trusted binding permits that exact approved action.
	freshness = "opaque-first-revision"
	call.Input.CallID = "authorized"
	require.NoError(t, reg.Execute(ctx, call, func(Chunk) error { return nil }))
	assert.Equal(t, 1, calls)
	assert.Equal(t, 3, binds)
}

func TestApprovalRejectsWrongIssuerAndFutureIssuanceBeforeHandler(t *testing.T) {
	for _, future := range []bool{false, true} {
		t.Run(strconv.FormatBool(future), func(t *testing.T) {
			// Arrange: a host grant exists but its provenance/time is invalid for this gate.
			now := time.Now()
			store := NewMemoryOperationStore()
			grantID := ""
			profile, err := NewOperationProfile(
				OperationProfileConfig{
					Store: store,
					Prepare: func(_ context.Context, call PreparedCall) (OperationIntent, error) {
						return OperationIntent{
							Namespace:         "write",
							Scope:             "tenant",
							Subject:           "host",
							OperationID:       "intent",
							AttemptID:         call.Input.CallID,
							GrantID:           grantID,
							PolicyFingerprint: "policy",
							CanonicalDigest:   "dependency",
							CanonicalRules:    "json",
							DisplayJSON:       []byte(`{}`),
						}, nil
					},
					Codec:    JSONResultCodec[string, string]{},
					Issuer:   "trusted-host",
					Clock:    func() time.Time { return now },
					Lease:    time.Minute,
					MaxBytes: 0,
				},
			)
			require.NoError(t, err)
			calls := 0
			tool, err := NewTool(
				"write",
				"Write",
				func(context.Context, *RunEnv, struct{}) (string, error) { calls++; return "written", nil },
				WithRequiresConfirmation(),
			)
			require.NoError(t, err)
			execute := compositionExecutor(t, tool, profile, "session")
			call := ToolCall{ToolName: "write", Input: ToolInput{CallID: "initial", ArgsJSON: []byte(`{}`)}}
			err = execute(context.Background(), call, func(Chunk) error { return nil })
			var pending *PendingApprovalError
			require.ErrorAs(t, err, &pending)
			issuer, issuedAt := "other-host", now
			if future {
				issuer, issuedAt = "trusted-host", now.Add(time.Minute)
			}
			require.NoError(
				t,
				store.PutGrant(
					context.Background(),
					ApprovalGrant{
						ID:        "grant",
						Issuer:    issuer,
						Binding:   pending.Challenge.Binding,
						IssuedAt:  issuedAt,
						ExpiresAt: now.Add(time.Hour),
					},
				),
			)
			grantID = "grant"
			call.Input.CallID = "resume"
			// Act.
			err = execute(
				context.Background(),
				call,
				func(Chunk) error { t.Fatal("invalid grant delivered output"); return nil },
			)
			// Assert.
			require.Error(t, err)
			assert.Zero(t, calls)
		})
	}
}
