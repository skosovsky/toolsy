package toolsy

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testOperationBinding() OperationBinding {
	return OperationBinding{Namespace: "writes", Scope: "tenant", Subject: "user", OperationID: "intent",
		Tool: "write", ManifestFingerprint: "manifest", ViewFingerprint: "view", PolicyFingerprint: "policy",
		CanonicalDigest: "digest", CanonicalRules: "canonical", DownstreamKey: "downstream"}
}

func testOperationClaim(now time.Time) OperationClaim {
	return OperationClaim{Binding: testOperationBinding(), AttemptID: "attempt", GrantID: "grant", Issuer: "host",
		RequiresApproval: true, Now: now, LeaseUntil: now.Add(time.Minute)}
}

func testApprovalGrant(now time.Time) ApprovalGrant {
	return ApprovalGrant{ID: "grant", Issuer: "host", Binding: testOperationBinding(), IssuedAt: now,
		ExpiresAt: now.Add(time.Hour), AllowRecovery: true}
}

func TestOperationStoreAtomicGrantClaim(t *testing.T) {
	// Arrange: two workers resume the same approved intent.
	ctx := context.Background()
	now := time.Now()
	store := NewMemoryOperationStore()
	require.NoError(t, store.PutGrant(ctx, testApprovalGrant(now)))
	var dispatches atomic.Int32
	var wg sync.WaitGroup
	// Act.
	for i := range 16 {
		wg.Go(func() {
			claim := testOperationClaim(now)
			claim.AttemptID = string(rune('a' + i))
			decision, err := store.Claim(ctx, claim)
			require.NoError(t, err)
			if decision.Dispatch {
				dispatches.Add(1)
			}
		})
	}
	wg.Wait()
	// Assert.
	assert.EqualValues(t, 1, dispatches.Load())
	assert.Len(t, store.Snapshot().Consumed, 1)
}

func TestApprovalBindsEveryField(t *testing.T) {
	fields := reflect.TypeFor[OperationBinding]()
	for i := range fields.NumField() {
		t.Run(fields.Field(i).Name, func(t *testing.T) {
			// Arrange.
			now := time.Now()
			store := NewMemoryOperationStore()
			require.NoError(t, store.PutGrant(context.Background(), testApprovalGrant(now)))
			claim := testOperationClaim(now)
			field := reflect.ValueOf(&claim.Binding).Elem().Field(i)
			field.SetString(field.String() + "changed")
			// Act.
			decision, err := store.Claim(context.Background(), claim)
			// Assert: changed namespaces/scopes/IDs cannot reuse this grant either.
			require.ErrorContains(t, err, "binding_mismatch")
			assert.False(t, decision.Dispatch)
			assert.Empty(t, store.Snapshot().Consumed)
		})
	}
}

func TestOperationRecoveryRequiresReconciliationAndFencesOldAttempt(t *testing.T) {
	// Arrange: crash after claiming; restored journal has consumed approval.
	ctx := context.Background()
	now := time.Now()
	store := NewMemoryOperationStore()
	require.NoError(t, store.PutGrant(ctx, testApprovalGrant(now)))
	claim := testOperationClaim(now)
	first, err := store.Claim(ctx, claim)
	require.NoError(t, err)
	require.True(t, first.Dispatch)
	store, err = RestoreOperationStore(store.Snapshot())
	require.NoError(t, err)
	claim.Now = now.Add(2 * time.Minute)
	claim.LeaseUntil = claim.Now.Add(time.Minute)
	claim.AttemptID = "recovery"
	// Act: expired lease is unknown, not retry permission.
	unknown, err := store.Claim(ctx, claim)
	require.NoError(t, err)
	// Assert.
	assert.False(t, unknown.Dispatch)
	assert.Equal(t, OperationUnknown, unknown.Record.State)
	require.ErrorContains(t, store.Finish(ctx, OperationFinish{Binding: claim.Binding, AttemptID: "attempt",
		State: OperationCompleted, Result: []byte("result")}), "stale_attempt")
	resolution := OperationResolution{
		Binding:           claim.Binding,
		ExpectedAttemptID: "attempt",
		State:             OperationRetryAuthorized,
		Now:               claim.Now,
	}
	require.ErrorContains(t, store.Resolve(ctx, resolution), "reconciliation_required")
	resolution.ProofID = "trusted remote absence check"
	require.NoError(t, store.Resolve(ctx, resolution))
	retry, err := store.Claim(ctx, claim)
	require.NoError(t, err)
	assert.True(t, retry.Dispatch)
	require.ErrorContains(t, store.Finish(ctx, OperationFinish{Binding: claim.Binding, AttemptID: "attempt",
		State: OperationUnknown}), "stale_attempt")
	require.NoError(t, store.Finish(ctx, OperationFinish{Binding: claim.Binding, AttemptID: "recovery",
		State: OperationCompleted, Result: []byte("result")}))
	completed, err := store.Claim(ctx, claim)
	require.NoError(t, err)
	assert.False(t, completed.Dispatch)
	assert.Equal(t, []byte("result"), completed.Record.Result)
}

func TestApprovalExpiredRejectedAndRecoveryPolicy(t *testing.T) {
	for _, kind := range []string{"approval_expired", "approval_rejected", "approval_consumed"} {
		t.Run(kind, func(t *testing.T) {
			// Arrange.
			ctx := context.Background()
			now := time.Now()
			store := NewMemoryOperationStore()
			grant := testApprovalGrant(now)
			grant.Rejected = kind == "approval_rejected"
			grant.AllowRecovery = false
			require.NoError(t, store.PutGrant(ctx, grant))
			claim := testOperationClaim(now)
			if kind == "approval_expired" {
				claim.Now = grant.ExpiresAt
				claim.LeaseUntil = claim.Now.Add(time.Minute)
			}
			if kind == "approval_consumed" {
				_, err := store.Claim(ctx, claim)
				require.NoError(t, err)
				require.NoError(
					t,
					store.Finish(
						ctx,
						OperationFinish{Binding: claim.Binding, AttemptID: claim.AttemptID, State: OperationUnknown},
					),
				)
				require.NoError(
					t,
					store.Resolve(ctx, OperationResolution{Binding: claim.Binding, ExpectedAttemptID: claim.AttemptID,
						State: OperationRetryAuthorized, ProofID: "verified", Now: now}),
				)
				claim.AttemptID = "retry"
			}
			// Act.
			decision, err := store.Claim(ctx, claim)
			// Assert.
			require.ErrorContains(t, err, kind)
			assert.False(t, decision.Dispatch)
		})
	}
}

func TestOperationAttemptIdentityCannotBeRecycled(t *testing.T) {
	// Arrange: two explicitly reconciled attempts of the same logical operation.
	ctx := context.Background()
	now := time.Now()
	store := NewMemoryOperationStore()
	claim := testOperationClaim(now)
	claim.RequiresApproval = false
	claim.GrantID = ""
	for _, attempt := range []string{"worker-a", "worker-b"} {
		claim.AttemptID = attempt
		decision, err := store.Claim(ctx, claim)
		require.NoError(t, err)
		require.True(t, decision.Dispatch)
		require.NoError(
			t,
			store.Finish(ctx, OperationFinish{Binding: claim.Binding, AttemptID: attempt, State: OperationUnknown}),
		)
		require.NoError(t, store.Resolve(ctx, OperationResolution{Binding: claim.Binding, ExpectedAttemptID: attempt,
			State: OperationRetryAuthorized, ProofID: "verified absence", Now: now}))
	}
	store, err := RestoreOperationStore(store.Snapshot())
	require.NoError(t, err)
	// Act: recycle the first ID while its original worker may still be running.
	claim.AttemptID = "worker-a"
	decision, err := store.Claim(ctx, claim)
	// Assert: a stale worker can never become the current fencing identity again.
	require.ErrorContains(t, err, "stale_attempt")
	assert.False(t, decision.Dispatch)
	require.ErrorContains(
		t,
		store.Finish(
			ctx,
			OperationFinish{
				Binding:   claim.Binding,
				AttemptID: "worker-a",
				State:     OperationCompleted,
				Result:    []byte("stale result"),
			},
		),
		"stale_attempt",
	)
	claim.AttemptID = "worker-c"
	decision, err = store.Claim(ctx, claim)
	require.NoError(t, err)
	assert.True(t, decision.Dispatch)
	assert.Equal(t, []string{"worker-a", "worker-b", "worker-c"}, decision.Record.Attempts)
	assert.Equal(t, "verified absence", decision.Record.ResolutionProofID)
}
