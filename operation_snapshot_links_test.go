package toolsy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func approvedSnapshot(t *testing.T) OperationSnapshot {
	t.Helper()
	now := time.Now().UTC().Add(-2 * time.Hour)
	store := NewMemoryOperationStore()
	binding := testOperationBinding()
	require.NoError(t, store.PutGrant(t.Context(), ApprovalGrant{ID: "grant", Issuer: "host", Binding: binding,
		IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), Rejected: false, AllowRecovery: true}))
	result, err := store.Claim(t.Context(), OperationClaim{Binding: binding, AttemptID: "attempt", GrantID: "grant",
		Issuer: "host", RequiresApproval: true, Now: now, LeaseUntil: now.Add(time.Minute)})
	require.NoError(t, err)
	require.True(t, result.Dispatch)
	return store.Snapshot()
}

func TestRestoreOperationSnapshotReverseLinks(t *testing.T) {
	cases := map[string]func(OperationSnapshot){
		"missing reservation":           func(s OperationSnapshot) { delete(s.Consumed, "grant") },
		"missing grant and reservation": func(s OperationSnapshot) { delete(s.Consumed, "grant"); delete(s.Grants, "grant") },
		"grant binding mismatch without reservation": func(s OperationSnapshot) {
			delete(s.Consumed, "grant")
			g := s.Grants["grant"]
			g.Binding.Subject = "other"
			s.Grants["grant"] = g
		},
		"expiry mismatch without reservation": func(s OperationSnapshot) {
			delete(s.Consumed, "grant")
			g := s.Grants["grant"]
			g.ExpiresAt = g.ExpiresAt.Add(time.Minute)
			s.Grants["grant"] = g
		},
		"grantless approval expiry": func(s OperationSnapshot) {
			delete(s.Consumed, "grant")
			for key, r := range s.Records {
				r.GrantID = ""
				s.Records[key] = r
			}
		},
		"reservation wrong key": func(s OperationSnapshot) { s.Consumed["grant"] = "other" },
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			// Arrange: a complete real atomic claim, then a corrupt trusted storage image.
			snapshot := approvedSnapshot(t)
			corrupt(snapshot)
			// Act.
			restored, err := RestoreOperationStore(snapshot)
			// Assert: no partially authorized store is returned.
			require.ErrorContains(t, err, "corrupt_snapshot")
			assert.Nil(t, restored)
		})
	}
}

func TestRestoreOperationSnapshotValidLinks(t *testing.T) {
	for _, approved := range []bool{false, true} {
		for _, state := range []OperationState{OperationInProgress, OperationUnknown, OperationCompleted, OperationRejected, OperationRetryAuthorized} {
			t.Run(
				string(state)+"/"+map[bool]string{false: "unapproved", true: "approved"}[approved],
				func(t *testing.T) {
					// Arrange: real transitions under a historical clock whose approval has since expired.
					store := snapshotTransitionStore(t, approved, state)
					// Act.
					restored, err := RestoreOperationStore(store.Snapshot())
					// Assert: original authority and all legitimate states survive restart.
					require.NoError(t, err)
					record, found, inspectErr := restored.Inspect(context.Background(), testOperationBinding())
					require.NoError(t, inspectErr)
					assert.True(t, found)
					assert.Equal(t, state, record.State)
					if approved {
						assert.True(t, record.ApprovalExpiresAt.Before(time.Now()))
					}
				},
			)
		}
	}
}

func snapshotTransitionStore(t *testing.T, approved bool, state OperationState) *MemoryOperationStore {
	t.Helper()
	now := time.Now().UTC().Add(-2 * time.Hour)
	claim := testOperationClaim(now)
	store := NewMemoryOperationStore()
	if approved {
		require.NoError(t, store.PutGrant(t.Context(), testApprovalGrant(now)))
	} else {
		claim.RequiresApproval = false
		claim.GrantID = ""
	}
	result, err := store.Claim(t.Context(), claim)
	require.NoError(t, err)
	require.True(t, result.Dispatch)
	if state == OperationInProgress {
		return store
	}
	finish := OperationFinish{Binding: claim.Binding, AttemptID: claim.AttemptID, State: OperationUnknown, Result: nil}
	if state == OperationCompleted {
		finish.State = state
		finish.Result = []byte("done")
	}
	require.NoError(t, store.Finish(t.Context(), finish))
	if state == OperationRejected || state == OperationRetryAuthorized {
		require.NoError(t, store.Resolve(t.Context(), OperationResolution{
			Binding:           claim.Binding,
			ExpectedAttemptID: claim.AttemptID,
			State:             state,
			Result:            nil,
			ProofID:           "trusted-proof",
			Now:               now.Add(time.Minute),
		}))
	}
	return store
}

func setSnapshotState(snapshot OperationSnapshot, state OperationState, approved bool) {
	for key, record := range snapshot.Records {
		if !approved {
			record.GrantID = ""
			record.ApprovalExpiresAt = time.Time{}
			delete(snapshot.Consumed, "grant")
		}
		record.State = state
		if state == OperationCompleted {
			record.Result = []byte("result")
		}
		if state == OperationRejected || state == OperationRetryAuthorized {
			record.ResolutionProofID = "proof"
			record.ResolvedAt = time.Now().UTC()
		}
		snapshot.Records[key] = record
	}
}

func TestOperationClaimRejectsInconsistentApprovalMode(t *testing.T) {
	// Arrange: an unused grant identifier cannot create a grant-bearing unapproved record.
	claim := testOperationClaim(time.Now().UTC())
	claim.RequiresApproval = false
	empty := NewMemoryOperationStore()
	// Act.
	result, err := empty.Claim(t.Context(), claim)
	// Assert: rejection precedes dispatch and mutation.
	require.ErrorContains(t, err, "invalid_claim")
	assert.False(t, result.Dispatch)
	assert.Empty(t, empty.Snapshot().Records)

	// Arrange: an approved unknown operation is explicitly reconciled for recovery.
	snapshot := approvedSnapshot(t)
	setSnapshotState(snapshot, OperationRetryAuthorized, true)
	approved, restoreErr := RestoreOperationStore(snapshot)
	require.NoError(t, restoreErr)
	claim.GrantID = ""
	claim.AttemptID = "downgrade"
	before := approved.Snapshot()
	// Act: host attempts to remove approval mode from the same logical operation.
	result, err = approved.Claim(t.Context(), claim)
	// Assert: consumed reservation and original authority survive refusal.
	require.ErrorContains(t, err, "binding_mismatch")
	assert.False(t, result.Dispatch)
	assert.Equal(t, before, approved.Snapshot())
	_, restoreErr = RestoreOperationStore(approved.Snapshot())
	require.NoError(t, restoreErr)
}
