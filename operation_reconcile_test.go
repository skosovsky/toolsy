package toolsy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func uncertainOperation(t *testing.T) (*MemoryOperationStore, OperationClaim) {
	t.Helper()
	store := NewMemoryOperationStore()
	claim := testOperationClaim(time.Now())
	claim.RequiresApproval = false
	claim.GrantID = ""
	decision, err := store.Claim(context.Background(), claim)
	require.NoError(t, err)
	require.True(t, decision.Dispatch)
	require.NoError(
		t,
		store.Finish(
			context.Background(),
			OperationFinish{Binding: claim.Binding, AttemptID: claim.AttemptID, State: OperationUnknown},
		),
	)
	return store, claim
}

func TestOperationInspectNeverClaimsOrMutates(t *testing.T) {
	// Arrange: an absent operation, then a live claim.
	ctx := context.Background()
	store := NewMemoryOperationStore()
	claim := testOperationClaim(time.Now())
	claim.RequiresApproval = false
	claim.GrantID = ""
	// Act.
	_, found, err := store.Inspect(ctx, claim.Binding)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, store.Snapshot().Records)
	decision, err := store.Claim(ctx, claim)
	require.NoError(t, err)
	require.True(t, decision.Dispatch)
	record, found, err := store.Inspect(ctx, claim.Binding)
	require.NoError(t, err)
	require.True(t, found)
	record.Attempts[0] = "changed"
	record.Binding.Scope = "foreign"
	again, found, err := store.Inspect(ctx, claim.Binding)
	// Assert: returned data is private; read never consumes or expires the claim.
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, claim.AttemptID, again.Attempts[0])
	assert.Equal(t, OperationInProgress, again.State)
	foreign := claim.Binding
	foreign.CanonicalDigest = "different"
	_, found, err = store.Inspect(ctx, foreign)
	require.ErrorContains(t, err, "binding_mismatch")
	assert.False(t, found)
}

func TestOperationReconcileUsesOriginalBindingAndPrivateSnapshot(t *testing.T) {
	// Arrange: callback deliberately mutates its input but returns proven outcome.
	store, claim := uncertainOperation(t)
	var queried int
	// Act.
	err := ReconcileOperation(context.Background(), store, claim.Binding, func() time.Time { return claim.Now },
		func(_ context.Context, record OperationRecord) (ReconciliationDecision, error) {
			queried++
			assert.Equal(t, claim.Binding.DownstreamKey, record.Binding.DownstreamKey)
			record.Binding.Scope = "foreign"
			record.AttemptID = "foreign"
			assert.Equal(t, "foreign", record.AttemptID)
			record.Attempts[0] = "foreign"
			return ReconciliationDecision{
				State:   OperationCompleted,
				Result:  []byte("verified result"),
				ProofID: "receipt",
			}, nil
		})
	// Assert: resolution cannot be redirected by callback snapshot edits.
	require.NoError(t, err)
	record, found, err := store.Inspect(context.Background(), claim.Binding)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, OperationCompleted, record.State)
	assert.Equal(t, claim.AttemptID, record.AttemptID)
	assert.Equal(t, claim.AttemptID, record.Attempts[0])
	assert.Equal(t, "receipt", record.ResolutionProofID)
	assert.Equal(t, 1, queried)
}

func TestOperationReconcileCallbackFailureDoesNotAuthorizeRetry(t *testing.T) {
	// Arrange.
	store, claim := uncertainOperation(t)
	failure := errors.New("external query unavailable")
	// Act.
	err := ReconcileOperation(context.Background(), store, claim.Binding, func() time.Time { return claim.Now },
		func(context.Context, OperationRecord) (ReconciliationDecision, error) {
			return ReconciliationDecision{}, failure
		})
	// Assert: callback failures are distinguishable from store/business outcomes.
	var reconciliationErr *ReconciliationError
	require.ErrorAs(t, err, &reconciliationErr)
	require.ErrorIs(t, err, failure)
	decision, err := store.Claim(context.Background(), claim)
	require.NoError(t, err)
	assert.False(t, decision.Dispatch)
	assert.Equal(t, OperationUnknown, decision.Record.State)
}

func TestOperationReconcileCannotCompleteNewerAttempt(t *testing.T) {
	// Arrange: another worker resolves and starts a new attempt during callback.
	store, claim := uncertainOperation(t)
	ctx := context.Background()
	// Act.
	err := ReconcileOperation(ctx, store, claim.Binding, func() time.Time { return claim.Now },
		func(context.Context, OperationRecord) (ReconciliationDecision, error) {
			require.NoError(
				t,
				store.Resolve(ctx, OperationResolution{Binding: claim.Binding, ExpectedAttemptID: claim.AttemptID,
					State: OperationRetryAuthorized, ProofID: "absence", Now: claim.Now}),
			)
			next := claim
			next.AttemptID = "next"
			decision, claimErr := store.Claim(ctx, next)
			require.NoError(t, claimErr)
			require.True(t, decision.Dispatch)
			return ReconciliationDecision{
				State:   OperationCompleted,
				Result:  []byte("old result"),
				ProofID: "late receipt",
			}, nil
		})
	// Assert: stale reconciliation cannot overwrite the newer worker.
	require.ErrorContains(t, err, "stale_attempt")
	record, found, err := store.Inspect(ctx, claim.Binding)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "next", record.AttemptID)
	assert.Equal(t, OperationInProgress, record.State)
}

func TestOperationReconcileRequiresUncertaintyAndEvidence(t *testing.T) {
	for _, mode := range []string{"absent", "in_progress", "missing_proof"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			ctx := context.Background()
			store := NewMemoryOperationStore()
			claim := testOperationClaim(time.Now())
			claim.RequiresApproval = false
			claim.GrantID = ""
			if mode != "absent" {
				_, err := store.Claim(ctx, claim)
				require.NoError(t, err)
			}
			if mode == "missing_proof" {
				require.NoError(
					t,
					store.Finish(
						ctx,
						OperationFinish{Binding: claim.Binding, AttemptID: claim.AttemptID, State: OperationUnknown},
					),
				)
			}
			var callbacks int
			// Act.
			err := ReconcileOperation(ctx, store, claim.Binding, func() time.Time { return claim.Now },
				func(context.Context, OperationRecord) (ReconciliationDecision, error) {
					callbacks++
					return ReconciliationDecision{State: OperationRetryAuthorized}, nil
				})
			// Assert.
			require.Error(t, err)
			if mode == "missing_proof" {
				assert.Equal(t, 1, callbacks)
				require.ErrorContains(t, err, "reconciliation_required")
			} else {
				assert.Zero(t, callbacks)
			}
		})
	}
}
