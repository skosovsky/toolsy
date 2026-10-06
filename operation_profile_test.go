package toolsy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationProfileRegistryAndSessionReauthorize(t *testing.T) {
	for _, entry := range []string{"registry", "session", "direct"} {
		t.Run(entry, func(t *testing.T) {
			// Arrange: confirmation and typed ACL share the prepared execution path.
			ctx := context.Background()
			now := time.Now()
			store := NewMemoryOperationStore()
			var revoked bool
			var calls int
			tool, buildErr := NewTypedTool(TypedToolSpec[string, string, struct{}, string, string]{
				Name:        "write",
				Description: "Write",
				Options:     []ToolOption{WithRequiresConfirmation()},
				Policy: func(_ context.Context, req TypedPolicyRequest[string, string, struct{}]) Decision {
					if revoked || req.Context.Subject != "alice" {
						return DenyDecision("revoked")
					}
					return AllowDecision()
				},
				Handler: func(context.Context, TypedCallContext[string, string], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
					calls++
					result := NewToolResult[string, string]("private")
					result.Audience = AudienceInternal
					return result, nil
				},
			})
			require.NoError(t, buildErr)
			grantID := ""
			profile, profileErr := NewOperationProfile(
				store,
				func(_ context.Context, call PreparedCall) (OperationIntent, error) {
					digest := sha256.Sum256(call.Input.ArgsJSON)
					return OperationIntent{
						Namespace:         "writes",
						Scope:             call.Context.Scope.(string),
						Subject:           call.Context.Subject.(string),
						OperationID:       "intent",
						AttemptID:         call.Input.CallID,
						GrantID:           grantID,
						PolicyFingerprint: "policy",
						CanonicalDigest: hex.EncodeToString(
							digest[:],
						),
						CanonicalRules: "json",
						DisplayJSON:    []byte(`{"action":"write"}`),
					}, nil
				},
				JSONResultCodec[string, string]{},
				"host",
				func() time.Time { return now },
				time.Minute,
				0,
			)
			require.NoError(t, profileErr)
			reg, registryErr := NewRegistryBuilder(WithExecutionProfile(profile)).Add(tool).Build()
			require.NoError(t, registryErr)
			execute := reg.Execute
			if entry == "direct" {
				execute = func(ctx context.Context, call ToolCall, yield func(Chunk) error) error {
					return tool.Execute(
						ctx,
						NewRunEnv(nil, WithRunCallContext(call.CallContext), WithRunExecutionProfile(profile)),
						call.Input,
						yield,
					)
				}
			}
			if entry == "session" {
				session, sessionErr := NewSession(reg)
				require.NoError(t, sessionErr)
				execute = session.Execute
			}
			call := ToolCall{
				ToolName:    "write",
				Input:       ToolInput{CallID: "initial", ArgsJSON: []byte(`{}`)},
				CallContext: NewCallContext("alice", "tenant"),
			}
			// Act: approval, resume, replay, then revoked rights.
			pendingErr := execute(ctx, call, func(Chunk) error { return nil })
			var pending *PendingApprovalError
			require.ErrorAs(t, pendingErr, &pending)
			assert.Zero(t, calls)
			require.NoError(
				t,
				store.PutGrant(ctx, ApprovalGrant{ID: "grant", Issuer: "host", Binding: pending.Challenge.Binding,
					IssuedAt: now, ExpiresAt: now.Add(time.Hour)}),
			)
			grantID = "grant"
			call.Input.CallID = "resume"
			var received Chunk
			require.NoError(t, execute(ctx, call, func(c Chunk) error { received = c; return nil }))
			call.Input.CallID = "repeat"
			require.NoError(t, execute(ctx, call, func(c Chunk) error { received = c; return nil }))
			revoked = true
			var leaked int
			err := execute(ctx, call, func(Chunk) error { leaked++; return nil })
			// Assert.
			require.ErrorIs(t, err, ErrPolicyDenied)
			assert.Equal(t, 1, calls)
			assert.Zero(t, leaked)
			assert.Equal(t, AudienceInternal, received.ToolEnvelope().Audience)
			assert.Equal(t, "repeat", received.CallID)
		})
	}
}

func TestOperationProfileTimeoutAfterDispatchIsNotRetryable(t *testing.T) {
	// Arrange: the handler may have committed before timing out.
	now := time.Now()
	store := NewMemoryOperationStore()
	profile, err := NewOperationProfile(store, func(context.Context, PreparedCall) (OperationIntent, error) {
		return OperationIntent{
			Namespace:         "writes",
			Scope:             "tenant",
			Subject:           "user",
			OperationID:       "intent",
			AttemptID:         "attempt",
			PolicyFingerprint: "policy",
			CanonicalDigest:   "digest",
			CanonicalRules:    "json",
			DisplayJSON:       []byte(`{}`),
		}, nil
	}, JSONResultCodec[string, string]{}, "host", func() time.Time { return now }, time.Minute, 0)
	require.NoError(t, err)
	var calls int
	tool, err := NewTool(
		"write",
		"Write",
		func(context.Context, *RunEnv, struct{}) (string, error) { calls++; return "", context.DeadlineExceeded },
	)
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(tool).Build()
	require.NoError(t, err)
	call := ToolCall{ToolName: "write", Input: ToolInput{ArgsJSON: []byte(`{}`)}}
	// Act.
	err = reg.Execute(context.Background(), call, func(Chunk) error { return nil })
	// Assert: original timeout remains diagnosable, but the operation is not retryable.
	var unknown *OperationOutcomeError
	require.ErrorAs(t, err, &unknown)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	toolErr, ok := AsToolError(err)
	require.True(t, ok)
	assert.False(t, toolErr.Retryable)
	err = reg.Execute(context.Background(), call, func(Chunk) error { return nil })
	require.ErrorContains(t, err, string(OperationUnknown))
	assert.Equal(t, 1, calls)
}

func TestOperationProfilePendingResumeReplayAndDeliveryFailure(t *testing.T) {
	// Arrange: host identity and a redacted action snapshot.
	ctx := context.Background()
	now := time.Now()
	store := NewMemoryOperationStore()
	intent := OperationIntent{Namespace: "writes", Scope: "tenant", Subject: "user", OperationID: "intent",
		PolicyFingerprint: "policy", CanonicalDigest: "opaque digest", CanonicalRules: "host canonical",
		AttemptID: "first", DisplayJSON: []byte(`{"action":"write","value":"redacted"}`)}
	call := PreparedCall{
		Manifest: ToolManifest{Name: "write", RequiresConfirmation: true},
		Input:    ToolInput{CallID: "first"},
	}
	profile, err := NewOperationProfile(
		store,
		func(context.Context, PreparedCall) (OperationIntent, error) { return intent, nil },
		JSONResultCodec[string, string]{},
		"host",
		func() time.Time { return now },
		time.Minute,
		0,
	)
	require.NoError(t, err)
	var calls int
	invoke := func(yield func(Chunk) error) error {
		calls++
		return yield(Chunk{Event: EventResult, Data: []byte(`"ok"`), MimeType: MimeTypeJSON, TypedResult: "ok"})
	}
	// Act: no grant pauses without executing. Host authenticates approval separately.
	err = profile.ExecutePrepared(
		ctx,
		call,
		invoke,
		func(c Chunk) error { assert.Equal(t, EventControl, c.Event); return nil },
	)
	var pending *PendingApprovalError
	require.ErrorAs(t, err, &pending)
	require.ErrorIs(t, err, ErrPause)
	assert.Zero(t, calls)
	require.NoError(
		t,
		store.PutGrant(ctx, ApprovalGrant{ID: "grant", Issuer: "host", Binding: pending.Challenge.Binding,
			IssuedAt: now, ExpiresAt: now.Add(time.Hour)}),
	)
	intent.GrantID = "grant"
	deliveryErr := errors.New("lost response")
	err = profile.ExecutePrepared(ctx, call, invoke, func(Chunk) error { return deliveryErr })
	require.ErrorIs(t, err, deliveryErr)
	call.Input.CallID = "repeat"
	intent.AttemptID = "repeat"
	var replay Chunk
	require.NoError(t, profile.ExecutePrepared(ctx, call, invoke, func(c Chunk) error { replay = c; return nil }))
	// Assert: lost delivery did not authorize a second write.
	assert.Equal(t, 1, calls)
	assert.Equal(t, "repeat", replay.CallID)
	assert.Equal(t, "ok", replay.TypedResult)
	assert.Equal(t, true, replay.ToolEnvelope().Metadata[CacheReplayMetadata])
}

func TestOperationProfileLateFailureCannotReplaySuccess(t *testing.T) {
	// Arrange.
	store := NewMemoryOperationStore()
	now := time.Now()
	intent := OperationIntent{
		Namespace:         "writes",
		Scope:             "tenant",
		Subject:           "user",
		OperationID:       "intent",
		PolicyFingerprint: "policy",
		CanonicalDigest:   "digest",
		CanonicalRules:    "canonical",
		AttemptID:         "attempt",
		DisplayJSON:       []byte(`{}`),
	}
	profile, err := NewOperationProfile(
		store,
		func(context.Context, PreparedCall) (OperationIntent, error) { return intent, nil },
		JSONResultCodec[string, string]{},
		"host",
		func() time.Time { return now },
		time.Minute,
		0,
	)
	require.NoError(t, err)
	call := PreparedCall{Manifest: ToolManifest{Name: "write"}}
	var calls, deliveries int
	failure := errors.New("remote may have committed")
	invoke := func(yield func(Chunk) error) error {
		calls++
		if yieldErr := yield(Chunk{Event: EventResult, Data: []byte(`"ok"`), MimeType: MimeTypeJSON}); yieldErr != nil {
			return yieldErr
		}
		return failure
	}
	// Act.
	err = profile.ExecutePrepared(context.Background(), call, invoke, func(Chunk) error { deliveries++; return nil })
	require.ErrorIs(t, err, failure)
	err = profile.ExecutePrepared(context.Background(), call, invoke, func(Chunk) error { deliveries++; return nil })
	// Assert.
	require.ErrorContains(t, err, string(OperationUnknown))
	assert.Equal(t, 1, calls)
	assert.Zero(t, deliveries)
}

func TestOperationProfileApprovalBindsPreparedArgsAndAttachments(t *testing.T) {
	for _, changed := range []string{"args", "attachment", "attachment_mime"} {
		t.Run(changed, func(t *testing.T) {
			// Arrange: one host dependency binding, one approved immutable input.
			ctx := context.Background()
			now := time.Now()
			store := NewMemoryOperationStore()
			grantID := ""
			profile, err := NewOperationProfile(store, func(context.Context, PreparedCall) (OperationIntent, error) {
				return OperationIntent{
					Namespace:         "writes",
					Scope:             "tenant",
					Subject:           "subject",
					OperationID:       "intent",
					AttemptID:         "attempt",
					GrantID:           grantID,
					PolicyFingerprint: "policy",
					CanonicalDigest:   "opaque dependency identity",
					CanonicalRules:    "json",
					DisplayJSON:       []byte(`{"action":"write"}`),
				}, nil
			}, JSONResultCodec[string, string]{}, "host", func() time.Time { return now }, time.Minute, 0)
			require.NoError(t, err)
			call := PreparedCall{
				Manifest: ToolManifest{Name: "write", RequiresConfirmation: true},
				Input: ToolInput{ArgsJSON: []byte(`{"amount":500}`),
					Attachments: []Attachment{{MimeType: "text/plain", Data: []byte("approved file")}}},
			}
			var calls int
			invoke := func(func(Chunk) error) error { calls++; return nil }
			err = profile.ExecutePrepared(ctx, call, invoke, func(Chunk) error { return nil })
			var pending *PendingApprovalError
			require.ErrorAs(t, err, &pending)
			require.NoError(
				t,
				store.PutGrant(ctx, ApprovalGrant{ID: "grant", Issuer: "host", Binding: pending.Challenge.Binding,
					IssuedAt: now, ExpiresAt: now.Add(time.Hour)}),
			)
			grantID = "grant"
			// Act: the host's opaque identity is unchanged, but prepared input changes.
			switch changed {
			case "args":
				call.Input.ArgsJSON = []byte(`{"amount":5000}`)
			case "attachment":
				call.Input.Attachments[0].Data = []byte("different file")
			case "attachment_mime":
				call.Input.Attachments[0].MimeType = "application/octet-stream"
			}
			err = profile.ExecutePrepared(ctx, call, invoke, func(Chunk) error { return nil })
			// Assert: this action cannot use the old grant.
			require.ErrorContains(t, err, "binding_mismatch")
			assert.Zero(t, calls)
		})
	}
}
