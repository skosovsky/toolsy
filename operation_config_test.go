package toolsy

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationProfileConfigurationRejectsInvalidFields(t *testing.T) {
	var store *MemoryOperationStore
	var codec *JSONResultCodec[string, string]
	cases := map[string]func(*OperationProfileConfig){
		"nil store":       func(c *OperationProfileConfig) { c.Store = nil },
		"typed nil store": func(c *OperationProfileConfig) { c.Store = store },
		"nil preparation": func(c *OperationProfileConfig) { c.Prepare = nil },
		"nil codec":       func(c *OperationProfileConfig) { c.Codec = nil },
		"typed nil codec": func(c *OperationProfileConfig) { c.Codec = codec },
		"empty issuer":    func(c *OperationProfileConfig) { c.Issuer = "" },
		"nil clock":       func(c *OperationProfileConfig) { c.Clock = nil },
		"zero lease":      func(c *OperationProfileConfig) { c.Lease = 0 },
		"negative lease":  func(c *OperationProfileConfig) { c.Lease = -time.Nanosecond },
		"negative cap":    func(c *OperationProfileConfig) { c.MaxBytes = -1 },
	}
	for name, invalidate := range cases {
		t.Run(name, func(t *testing.T) {
			// Arrange: construction must not execute either host callback.
			config := OperationProfileConfig{
				Store: NewMemoryOperationStore(), Codec: JSONResultCodec[string, string]{}, Issuer: "host",
				Prepare: func(context.Context, PreparedCall) (OperationIntent, error) {
					t.Fatal("constructor invoked host preparation")
					return OperationIntent{}, nil
				},
				Clock: func() time.Time { t.Fatal("constructor invoked host clock"); return time.Time{} },
				Lease: time.Minute, MaxBytes: 0,
			}
			invalidate(&config)
			// Act.
			profile, err := NewOperationProfile(config)
			// Assert.
			require.ErrorIs(t, err, ErrOperationProfileConfiguration)
			assert.Nil(t, profile)
		})
	}
}

func TestOperationProfileConfigurationSnapshotAndCap(t *testing.T) {
	for _, capBytes := range []int{0, 1, 2048} {
		t.Run(strconv.Itoa(capBytes), func(t *testing.T) {
			// Arrange.
			now := time.Date(2026, time.October, 6, 0, 0, 0, 0, time.UTC)
			store := NewMemoryOperationStore()
			calls := 0
			clockCalls := 0
			config := OperationProfileConfig{
				Store: store, Codec: JSONResultCodec[string, string]{}, Issuer: "host",
				Prepare: func(context.Context, PreparedCall) (OperationIntent, error) {
					calls++
					return OperationIntent{OperationID: "captured"}, nil
				},
				Clock: func() time.Time { clockCalls++; return now }, Lease: time.Minute, MaxBytes: capBytes,
			}
			// Act: changing the caller-owned configuration never changes the profile.
			profile, err := NewOperationProfile(config)
			require.NoError(t, err)
			require.Zero(t, calls)
			require.Zero(t, clockCalls)
			config = OperationProfileConfig{}
			intent, err := profile.prepare(context.Background(), PreparedCall{})
			// Assert.
			require.NoError(t, err)
			assert.Equal(t, "captured", intent.OperationID)
			assert.Equal(t, 1, calls)
			assert.Same(t, store, profile.store)
			assert.Equal(t, now, profile.clock())
			assert.Equal(t, 1, clockCalls)
			assert.Equal(t, "host", profile.issuer)
			assert.Equal(t, time.Minute, profile.lease)
			if capBytes == 0 {
				capBytes = defaultCacheResultLimit
			}
			assert.Equal(t, capBytes, profile.maxBytes)
			assert.Nil(t, config.Store)
		})
	}
}

func TestRecoveryCannotReplaceOriginalGrant(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid original", true: "expired original"}[expired], func(t *testing.T) {
			// Arrange: reconcile an uncertain original operation to retry-authorized.
			ctx := context.Background()
			now := time.Date(2026, time.October, 6, 0, 0, 0, 0, time.UTC)
			store := NewMemoryOperationStore()
			original := testApprovalGrant(now)
			require.NoError(t, store.PutGrant(ctx, original))
			claim := testOperationClaim(now)
			first, err := store.Claim(ctx, claim)
			require.NoError(t, err)
			require.True(t, first.Dispatch)
			require.NoError(t, store.Finish(ctx, OperationFinish{Binding: claim.Binding, AttemptID: claim.AttemptID,
				State: OperationUnknown, Result: nil}))
			reconcileAt := now.Add(time.Second)
			if expired {
				reconcileAt = original.ExpiresAt
			}
			resolution := OperationResolution{
				Binding: claim.Binding, ExpectedAttemptID: claim.AttemptID,
				State: OperationRetryAuthorized, Result: nil,
				ProofID: "authenticated downstream absence", Now: reconcileAt,
			}
			require.NoError(t, store.Resolve(ctx, resolution))
			fresh := original
			fresh.ID = "fresh-grant"
			fresh.IssuedAt = reconcileAt
			fresh.ExpiresAt = reconcileAt.Add(time.Hour)
			require.NoError(t, store.PutGrant(ctx, fresh))
			claim.AttemptID = "recovery"
			claim.Now = reconcileAt
			claim.LeaseUntil = reconcileAt.Add(time.Minute)
			// Act: valid fresh approval for the same binding cannot replace original grant.
			claim.GrantID = fresh.ID
			freshDecision, freshErr := store.Claim(ctx, claim)
			// Assert: no dispatch and no consumption of fresh approval.
			require.ErrorContains(t, freshErr, "approval_consumed")
			assert.False(t, freshDecision.Dispatch)
			assert.NotContains(t, store.Snapshot().Consumed, fresh.ID)
			assert.Equal(t, OperationRetryAuthorized, store.Snapshot().Records[operationKey(claim.Binding)].State)
			// Act: same original grant can recover only while still unexpired.
			claim.GrantID = original.ID
			originalDecision, originalErr := store.Claim(ctx, claim)
			if expired {
				require.ErrorContains(t, originalErr, "approval_expired")
				assert.False(t, originalDecision.Dispatch)
			} else {
				require.NoError(t, originalErr)
				require.True(t, originalDecision.Dispatch)
				// Old worker remains fenced after a new authorized claim.
				oldFinish := OperationFinish{
					Binding: claim.Binding, AttemptID: first.Record.AttemptID,
					State: OperationUnknown, Result: nil,
				}
				require.ErrorContains(t, store.Finish(ctx, oldFinish), "stale_attempt")
			}
		})
	}
}

func TestRecoveryProfileRefusesFreshGrantAfterOriginalExpires(t *testing.T) {
	// Arrange: actual handler has an uncertain post-dispatch outcome.
	ctx := context.Background()
	now := time.Date(2026, time.October, 6, 0, 0, 0, 0, time.UTC)
	store := NewMemoryOperationStore()
	grantID := ""
	effects := 0
	uncertain := errors.New("downstream result unavailable")
	tool, err := NewTool("write", "Write", func(context.Context, *RunEnv, struct{}) (string, error) {
		effects++
		return "", uncertain
	}, WithRequiresConfirmation())
	require.NoError(t, err)
	clock := func() time.Time { return now }
	profile, err := NewOperationProfile(OperationProfileConfig{
		Store:    store,
		Codec:    JSONResultCodec[string, string]{},
		Issuer:   "host",
		Clock:    clock,
		Lease:    time.Minute,
		MaxBytes: 0,
		Prepare: func(_ context.Context, call PreparedCall) (OperationIntent, error) {
			return OperationIntent{
				Namespace:         "writes",
				Scope:             "tenant",
				Subject:           "alice",
				OperationID:       "logical-write",
				PolicyFingerprint: "acl-v1",
				CanonicalDigest:   "credential-v1",
				CanonicalRules:    "v1",
				DownstreamKey:     "original-key",
				AttemptID:         call.Input.CallID,
				GrantID:           grantID,
				DisplayJSON:       []byte(`{"action":"write"}`),
			}, nil
		},
	})
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(tool).Build()
	require.NoError(t, err)
	call := ToolCall{ToolName: "write", Input: ToolInput{CallID: "initial", ArgsJSON: []byte(`{}`)},
		CallContext: NewCallContext("alice", "tenant")}
	yield := func(Chunk) error { return nil }
	pendingErr := reg.Execute(ctx, call, yield)
	var pending *PendingApprovalError
	require.ErrorAs(t, pendingErr, &pending)
	require.Zero(t, effects)
	original := ApprovalGrant{ID: "original", Issuer: "host", Binding: pending.Challenge.Binding, IssuedAt: now,
		ExpiresAt: now.Add(time.Hour), Rejected: false, AllowRecovery: true}
	require.NoError(t, store.PutGrant(ctx, original))
	grantID = original.ID
	dispatchErr := reg.Execute(ctx, call, yield)
	var outcome *OperationOutcomeError
	require.ErrorAs(t, dispatchErr, &outcome)
	require.ErrorIs(t, dispatchErr, uncertain)
	require.Equal(t, 1, effects)
	now = original.ExpiresAt
	require.NoError(t, ReconcileOperation(ctx, store, original.Binding, clock,
		func(_ context.Context, record OperationRecord) (ReconciliationDecision, error) {
			assert.Equal(t, original.ID, record.GrantID)
			return ReconciliationDecision{
				State:   OperationRetryAuthorized,
				Result:  nil,
				ProofID: "host verified downstream safety",
			}, nil
		}))
	fresh := original
	fresh.ID, fresh.IssuedAt, fresh.ExpiresAt = "fresh", now, now.Add(time.Hour)
	require.NoError(t, store.PutGrant(ctx, fresh))
	call.Input.CallID = "recovery"
	// Act: neither a fresh replacement grant nor the expired original permits dispatch.
	grantID = fresh.ID
	replacementErr := reg.Execute(ctx, call, yield)
	grantID = original.ID
	expiredErr := reg.Execute(ctx, call, yield)
	// Assert: physical effect counter remains unchanged through the real profile.
	require.ErrorContains(t, replacementErr, "approval_consumed")
	require.ErrorContains(t, expiredErr, "approval_expired")
	assert.Equal(t, 1, effects)
	record, found, err := store.Inspect(ctx, original.Binding)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, OperationRetryAuthorized, record.State)
	assert.Equal(t, "initial", record.AttemptID)
	assert.NotContains(t, store.Snapshot().Consumed, fresh.ID)
}
