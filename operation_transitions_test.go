package toolsy

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type operationTransitionFixture struct {
	States      []OperationState      `json:"states"`
	Transitions []operationTransition `json:"transitions"`
	Invariants  []string              `json:"invariants"`
}

type operationTransition struct {
	From     string `json:"from"`
	Event    string `json:"event"`
	To       string `json:"to"`
	Dispatch bool   `json:"dispatch"`
}

func TestOperationNormativeTransitions(t *testing.T) {
	raw, err := os.ReadFile("testdata/execution/operation-transitions.json")
	require.NoError(t, err)
	var fixture operationTransitionFixture
	require.NoError(t, json.Unmarshal(raw, &fixture))
	assert.ElementsMatch(
		t,
		[]OperationState{
			OperationInProgress,
			OperationCompleted,
			OperationUnknown,
			OperationRejected,
			OperationRetryAuthorized,
		},
		fixture.States,
	)
	for _, transition := range fixture.Transitions {
		t.Run(transition.From+"/"+transition.Event, func(t *testing.T) {
			// Arrange: an independent state image for each normative transition.
			now := time.Now()
			store := seedTransitionStore(t, transition, now)
			claim := testOperationClaim(now)
			claim.AttemptID = "next"
			// Act.
			dispatch := applyNormativeTransition(t, store, claim, transition.Event)
			// Assert: state and dispatch permission match the normative artifact.
			assert.Equal(t, transition.Dispatch, dispatch)
			state := "absent"
			if record, exists := store.Snapshot().Records[operationKey(claim.Binding)]; exists {
				state = string(record.State)
			}
			assert.Equal(t, transition.To, state)
			_, restoreErr := RestoreOperationStore(store.Snapshot())
			require.NoError(t, restoreErr)
		})
	}
}

func seedTransitionStore(t *testing.T, transition operationTransition, now time.Time) *MemoryOperationStore {
	t.Helper()
	ctx := context.Background()
	store := NewMemoryOperationStore()
	if transition.Event != "approval_missing" {
		require.NoError(t, store.PutGrant(ctx, testApprovalGrant(now)))
	}
	if transition.From == "absent" {
		return store
	}
	claim := testOperationClaim(now)
	decision, err := store.Claim(ctx, claim)
	require.NoError(t, err)
	require.True(t, decision.Dispatch)
	switch OperationState(transition.From) {
	case OperationInProgress:
	case OperationCompleted:
		require.NoError(
			t,
			store.Finish(
				ctx,
				OperationFinish{
					Binding:   claim.Binding,
					AttemptID: claim.AttemptID,
					State:     OperationCompleted,
					Result:    []byte("result"),
				},
			),
		)
	case OperationUnknown, OperationRetryAuthorized, OperationRejected:
		require.NoError(
			t,
			store.Finish(
				ctx,
				OperationFinish{Binding: claim.Binding, AttemptID: claim.AttemptID, State: OperationUnknown},
			),
		)
		if transition.From != string(OperationUnknown) {
			require.NoError(
				t,
				store.Resolve(ctx, OperationResolution{Binding: claim.Binding, ExpectedAttemptID: claim.AttemptID,
					State: OperationState(transition.From), ProofID: "trusted fixture proof", Now: now}),
			)
		}
	default:
		t.Fatalf("unimplemented fixture state %q", transition.From)
	}
	return store
}

func applyNormativeTransition(t *testing.T, store *MemoryOperationStore, claim OperationClaim, event string) bool {
	t.Helper()
	ctx := context.Background()
	switch event {
	case "atomic_authorized_claim",
		"concurrent_claim",
		"unverified_retry",
		"authorized_replay",
		"repeat_delivery",
		"lease_expired",
		"approval_missing":
		if event == "lease_expired" {
			claim.Now = claim.LeaseUntil
			claim.LeaseUntil = claim.Now.Add(time.Minute)
		}
		decision, err := store.Claim(ctx, claim)
		if event == "approval_missing" {
			require.ErrorContains(t, err, "approval_required")
		} else {
			require.NoError(t, err)
		}
		return decision.Dispatch
	case "fenced_success", "execution_or_validation_failure":
		finish := OperationFinish{Binding: claim.Binding, AttemptID: "attempt", State: OperationUnknown}
		if event == "fenced_success" {
			finish.State = OperationCompleted
			finish.Result = []byte("result")
		}
		require.NoError(t, store.Finish(ctx, finish))
	case "trusted_reconciled_success", "trusted_retry_proof", "trusted_rejected":
		resolution := OperationResolution{
			Binding:           claim.Binding,
			ExpectedAttemptID: "attempt",
			ProofID:           "trusted",
			Now:               claim.Now,
		}
		switch event {
		case "trusted_reconciled_success":
			resolution.State = OperationCompleted
			resolution.Result = []byte("result")
		case "trusted_retry_proof":
			resolution.State = OperationRetryAuthorized
		case "trusted_rejected":
			resolution.State = OperationRejected
		}
		require.NoError(t, store.Resolve(ctx, resolution))
	default:
		t.Fatalf("unimplemented fixture event %q", event)
	}
	return false
}
