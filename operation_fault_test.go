package toolsy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type faultOperationStore struct {
	*MemoryOperationStore

	claimFailure         error
	finishFailure        error
	persistBeforeFailure bool
	afterClaim           func()
}

func (s *faultOperationStore) Claim(ctx context.Context, claim OperationClaim) (ClaimResult, error) {
	if s.claimFailure != nil {
		return ClaimResult{}, s.claimFailure
	}
	result, err := s.MemoryOperationStore.Claim(ctx, claim)
	if s.afterClaim != nil {
		s.afterClaim()
	}
	return result, err
}

func (s *faultOperationStore) Finish(ctx context.Context, finish OperationFinish) error {
	if s.finishFailure != nil {
		if s.persistBeforeFailure {
			if err := s.MemoryOperationStore.Finish(ctx, finish); err != nil {
				return err
			}
		}
		return s.finishFailure
	}
	return s.MemoryOperationStore.Finish(ctx, finish)
}

func testFaultProfile(t *testing.T, store OperationStore, now *time.Time) *OperationProfile {
	t.Helper()
	p, err := NewOperationProfile(OperationProfileConfig{
		Store: store,
		Prepare: func(context.Context, PreparedCall) (OperationIntent, error) {
			return OperationIntent{
				Namespace:         "writes",
				Scope:             "tenant",
				Subject:           "subject",
				OperationID:       "intent",
				AttemptID:         "attempt",
				PolicyFingerprint: "policy",
				CanonicalDigest:   "digest",
				CanonicalRules:    "rules",
				DisplayJSON:       []byte(`{}`),
			}, nil
		},
		Codec:    JSONResultCodec[string, string]{},
		Issuer:   "host",
		Clock:    func() time.Time { return *now },
		Lease:    time.Minute,
		MaxBytes: 0,
	})
	require.NoError(t, err)
	return p
}

func TestOperationFaultStoreFailureNeverAuthorizesDispatch(t *testing.T) {
	// Arrange: the atomic store is unavailable before reservation.
	now := time.Now()
	failure := errors.New("journal unavailable")
	store := &faultOperationStore{MemoryOperationStore: NewMemoryOperationStore(), claimFailure: failure}
	p := testFaultProfile(t, store, &now)
	var calls int
	// Act.
	err := p.ExecutePrepared(context.Background(), PreparedCall{Manifest: ToolManifest{Name: "write"}},
		func(func(Chunk) error) error { calls++; return nil }, func(Chunk) error { return nil })
	// Assert.
	require.ErrorIs(t, err, failure)
	var storeErr *OperationStoreError
	require.ErrorAs(t, err, &storeErr)
	assert.Equal(t, "claim", storeErr.Stage)
	assert.Zero(t, calls)
}

func TestOperationFaultCommitPersistenceFailureNoBlindRetry(t *testing.T) {
	for _, persisted := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "before_persistence", true: "after_persistence_before_ack"}[persisted],
			func(t *testing.T) {
				// Arrange: an external write succeeds but journal completion cannot be acknowledged.
				now := time.Now()
				store := &faultOperationStore{MemoryOperationStore: NewMemoryOperationStore(),
					finishFailure: errors.New("completion unavailable"), persistBeforeFailure: persisted}
				p := testFaultProfile(t, store, &now)
				call := PreparedCall{Manifest: ToolManifest{Name: "write"}}
				var commits, deliveries int
				invoke := func(yield func(Chunk) error) error {
					commits++
					return yield(
						Chunk{
							Event:       EventResult,
							Data:        []byte(`"written"`),
							MimeType:    MimeTypeJSON,
							TypedResult: "written",
						},
					)
				}
				// Act: fail persistence, then restore availability and repeat the same intent.
				err := p.ExecutePrepared(
					context.Background(),
					call,
					invoke,
					func(Chunk) error { deliveries++; return nil },
				)
				var unknown *OperationOutcomeError
				require.ErrorAs(t, err, &unknown)
				assert.Zero(t, deliveries)
				store.finishFailure = nil
				now = now.Add(2 * time.Minute)
				err = p.ExecutePrepared(
					context.Background(),
					call,
					invoke,
					func(Chunk) error { deliveries++; return nil },
				)
				// Assert: completion persisted before lost ack may be replayed, but no fresh write occurs.
				assert.Equal(t, 1, commits)
				if persisted {
					require.NoError(t, err)
					assert.Equal(t, 1, deliveries)
				} else {
					require.ErrorContains(t, err, string(OperationUnknown))
					assert.Zero(t, deliveries)
				}
			},
		)
	}
}

func TestOperationFaultCancellationAndPanicRemainUnknown(t *testing.T) {
	for _, scenario := range []string{"cancel", "panic", "invalid_terminal", "ignored_control_error"} {
		t.Run(scenario, func(t *testing.T) {
			// Arrange.
			now := time.Now()
			store := NewMemoryOperationStore()
			p := testFaultProfile(t, store, &now)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			call := PreparedCall{Manifest: ToolManifest{Name: "write"}}
			var commits, terminals int
			//nolint:unparam // Producer deliberately ignores callback failures to test sticky rejection.
			invoke := func(yield func(Chunk) error) error {
				commits++ // external write can precede cancellation/output failure.
				switch scenario {
				case "cancel":
					cancel()
				case "panic":
					panic("worker lost")
				case "invalid_terminal":
					_ = yield(Chunk{Event: EventResult, Data: []byte(`"a"`), MimeType: MimeTypeJSON})
					_ = yield(Chunk{Event: EventResult, Data: []byte(`"b"`), MimeType: MimeTypeJSON})
				case "ignored_control_error":
					_ = yield(Chunk{Event: EventControl, Control: &PauseSignal{Reason: "pause"}})
					_ = yield(Chunk{Event: EventResult, Data: []byte(`"must not complete"`), MimeType: MimeTypeJSON})
				}
				return nil
			}
			//nolint:unparam // The public Chunk consumer contract requires an error result.
			consume := func(c Chunk) error {
				if c.Event == EventResult {
					terminals++
				}
				return nil
			}
			// Act.
			if scenario == "panic" {
				require.Panics(t, func() { _ = p.ExecutePrepared(ctx, call, invoke, consume) })
			} else {
				err := p.ExecutePrepared(ctx, call, invoke, consume)
				var unknown *OperationOutcomeError
				require.ErrorAs(t, err, &unknown)
			}
			err := p.ExecutePrepared(context.Background(), call, invoke, consume)
			// Assert: cleanup uses an uncancelled context, and no outcome permits blind dispatch.
			require.ErrorContains(t, err, string(OperationUnknown))
			assert.Equal(t, 1, commits)
			assert.Zero(t, terminals)
		})
	}
}

func TestOperationFaultClaimDelayOrCancellationDoesNotDispatch(t *testing.T) {
	for _, scenario := range []string{"cancel", "lease_expired"} {
		t.Run(scenario, func(t *testing.T) {
			// Arrange: reservation succeeds, but its acknowledgement is delayed/cancelled.
			now := time.Now()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := &faultOperationStore{MemoryOperationStore: NewMemoryOperationStore()}
			store.afterClaim = func() {
				if scenario == "cancel" {
					cancel()
				} else {
					now = now.Add(2 * time.Minute)
				}
			}
			profile := testFaultProfile(t, store, &now)
			var calls int
			// Act.
			err := profile.ExecutePrepared(ctx, PreparedCall{Manifest: ToolManifest{Name: "write"}},
				func(func(Chunk) error) error { calls++; return nil }, func(Chunk) error { return nil })
			// Assert: claim is conservatively unknown, but a stale permission cannot dispatch.
			var unknown *OperationOutcomeError
			require.ErrorAs(t, err, &unknown)
			assert.Zero(t, calls)
			for _, record := range store.Snapshot().Records {
				assert.Equal(t, OperationUnknown, record.State)
			}
		})
	}
}

func TestOperationFaultApprovalExpiresWhileClaimAcknowledgementWaits(t *testing.T) {
	// Arrange: valid reservation, expired approval before dispatch, lease still live.
	now := time.Now()
	store := &faultOperationStore{MemoryOperationStore: NewMemoryOperationStore()}
	grantID := ""
	profile, err := NewOperationProfile(OperationProfileConfig{
		Store: store,
		Prepare: func(context.Context, PreparedCall) (OperationIntent, error) {
			return OperationIntent{
				Namespace:         "writes",
				Scope:             "tenant",
				Subject:           "subject",
				OperationID:       "intent",
				AttemptID:         "attempt",
				GrantID:           grantID,
				PolicyFingerprint: "policy",
				CanonicalDigest:   "digest",
				CanonicalRules:    "json",
				DisplayJSON:       []byte(`{}`),
			}, nil
		},
		Codec:    JSONResultCodec[string, string]{},
		Issuer:   "host",
		Clock:    func() time.Time { return now },
		Lease:    time.Hour,
		MaxBytes: 0,
	})
	require.NoError(t, err)
	call := PreparedCall{Manifest: ToolManifest{Name: "write", RequiresConfirmation: true}}
	var calls int
	invoke := func(func(Chunk) error) error { calls++; return nil }
	err = profile.ExecutePrepared(context.Background(), call, invoke, func(Chunk) error { return nil })
	var pending *PendingApprovalError
	require.ErrorAs(t, err, &pending)
	require.NoError(
		t,
		store.PutGrant(
			context.Background(),
			ApprovalGrant{ID: "grant", Issuer: "host", Binding: pending.Challenge.Binding,
				IssuedAt: now, ExpiresAt: now.Add(time.Minute)},
		),
	)
	grantID = "grant"
	store.afterClaim = func() { now = now.Add(2 * time.Minute) }
	// Act.
	err = profile.ExecutePrepared(context.Background(), call, invoke, func(Chunk) error { return nil })
	// Assert: stale reservation does not permit an expired confirmation.
	var decision *OperationError
	require.ErrorAs(t, err, &decision)
	assert.Equal(t, "approval_expired", decision.Kind)
	assert.Zero(t, calls)
}
