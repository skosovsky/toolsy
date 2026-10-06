//go:build darwin || linux

package filejournal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func journalBinding() toolsy.OperationBinding {
	return toolsy.OperationBinding{
		Namespace:           "writes",
		Scope:               "tenant",
		Subject:             "subject",
		OperationID:         "intent",
		Tool:                "write",
		ManifestFingerprint: "manifest",
		PolicyFingerprint:   "policy",
		CanonicalDigest:     "digest",
		CanonicalRules:      "rules",
	}
}

func journalClaim(now time.Time) toolsy.OperationClaim {
	return toolsy.OperationClaim{Binding: journalBinding(), AttemptID: "attempt", Issuer: "host", GrantID: "grant",
		RequiresApproval: true, Now: now, LeaseUntil: now.Add(time.Minute)}
}

func seedJournal(t *testing.T, path string, now time.Time) *Store {
	t.Helper()
	store, err := Open(path, 0)
	require.NoError(t, err)
	require.NoError(t, store.PutGrant(context.Background(), toolsy.ApprovalGrant{ID: "grant", Issuer: "host",
		Binding: journalBinding(), IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}))
	return store
}

func TestJournalRestartDoesNotRedispatchConsumedClaim(t *testing.T) {
	// Arrange: the worker disappears after a durable claim.
	ctx := context.Background()
	now := time.Now()
	path := filepath.Join(t.TempDir(), "journal.json")
	store := seedJournal(t, path, now)
	first, err := store.Claim(ctx, journalClaim(now))
	require.NoError(t, err)
	require.True(t, first.Dispatch)
	// Act: reopen as a different host worker after lease expiration.
	reopened, err := Open(path, 0)
	require.NoError(t, err)
	claim := journalClaim(now.Add(2 * time.Minute))
	claim.AttemptID = "recovered"
	result, err := reopened.Claim(ctx, claim)
	// Assert.
	require.NoError(t, err)
	assert.False(t, result.Dispatch)
	assert.Equal(t, toolsy.OperationUnknown, result.Record.State)
	require.ErrorContains(t, reopened.Finish(ctx, toolsy.OperationFinish{Binding: claim.Binding, AttemptID: "attempt",
		State: toolsy.OperationCompleted, Result: []byte("unverified")}), "stale_attempt")
}

func TestJournalConcurrentWorkers(t *testing.T) {
	// Arrange: independent handles to one durable journal.
	now := time.Now()
	path := filepath.Join(t.TempDir(), "journal.json")
	seedJournal(t, path, now)
	var count atomic.Int32
	var wg sync.WaitGroup
	// Act.
	for i := range 16 {
		wg.Go(func() {
			store, err := Open(path, 0)
			require.NoError(t, err)
			claim := journalClaim(now)
			claim.AttemptID = strconv.Itoa(i)
			result, err := store.Claim(context.Background(), claim)
			require.NoError(t, err)
			if result.Dispatch {
				count.Add(1)
			}
		})
	}
	wg.Wait()
	// Assert.
	assert.EqualValues(t, 1, count.Load())
}

func TestJournalInterprocessClaim(t *testing.T) {
	// Arrange: two independent processes, not an in-process mutex.
	path := filepath.Join(t.TempDir(), "journal.json")
	seedJournal(t, path, time.Now())
	binary, err := os.Executable()
	require.NoError(t, err)
	var count atomic.Int32
	var wg sync.WaitGroup
	// Act.
	for i := range 2 {
		wg.Go(func() {
			cmd := exec.CommandContext(t.Context(), binary, "-test.run=^TestJournalChild$")
			cmd.Env = append(os.Environ(), "TOOLSY_JOURNAL_CHILD="+path, "TOOLSY_JOURNAL_ATTEMPT="+strconv.Itoa(i))
			output, commandErr := cmd.CombinedOutput()
			require.NoError(t, commandErr, string(output))
			if strings.Contains(string(output), "CLAIM_DISPATCH") {
				count.Add(1)
			}
		})
	}
	wg.Wait()
	// Assert.
	assert.EqualValues(t, 1, count.Load())
}

func TestJournalChild(t *testing.T) {
	path := os.Getenv("TOOLSY_JOURNAL_CHILD")
	if path == "" {
		t.Skip("subprocess helper")
	}
	store, err := Open(path, 0)
	require.NoError(t, err)
	claim := journalClaim(time.Now())
	claim.AttemptID = os.Getenv("TOOLSY_JOURNAL_ATTEMPT")
	result, err := store.Claim(context.Background(), claim)
	require.NoError(t, err)
	if result.Dispatch {
		fmt.Println("CLAIM_DISPATCH")
	}
}

func TestJournalLimitFailsBeforeClaimPersistence(t *testing.T) {
	// Arrange: existing grant fits, adding a record exceeds the configured limit.
	now := time.Now()
	path := filepath.Join(t.TempDir(), "journal.json")
	seedJournal(t, path, now)
	info, err := os.Stat(path)
	require.NoError(t, err)
	limited, err := Open(path, info.Size())
	require.NoError(t, err)
	// Act.
	decision, err := limited.Claim(context.Background(), journalClaim(now))
	// Assert: failed transaction did not consume grant or authorize dispatch.
	require.ErrorIs(t, err, ErrLimitExceeded)
	assert.False(t, decision.Dispatch)
	store, err := Open(path, 0)
	require.NoError(t, err)
	decision, err = store.Claim(context.Background(), journalClaim(now))
	require.NoError(t, err)
	assert.True(t, decision.Dispatch)
}

const simulatedCrashExit = 23
const privateEffectMode = 0o600

func crashProfile(t *testing.T, store *Store, now time.Time, attempt string) *toolsy.OperationProfile {
	t.Helper()
	profile, err := toolsy.NewOperationProfile(
		toolsy.OperationProfileConfig{
			Store: store,
			Prepare: func(context.Context, toolsy.PreparedCall) (toolsy.OperationIntent, error) {
				return toolsy.OperationIntent{
					Namespace:         "writes",
					Scope:             "tenant",
					Subject:           "subject",
					OperationID:       "intent",
					AttemptID:         attempt,
					PolicyFingerprint: "policy",
					CanonicalDigest:   "digest",
					CanonicalRules:    "rules",
					DisplayJSON:       []byte(`{}`),
				}, nil
			},
			Codec:    toolsy.JSONResultCodec[string, string]{},
			Issuer:   "host",
			Clock:    func() time.Time { return now },
			Lease:    time.Minute,
			MaxBytes: 0,
		},
	)
	require.NoError(t, err)
	return profile
}

func TestJournalRemoteCommitCrashRecovery(t *testing.T) {
	// Arrange: a separate process commits an external effect then exits without
	// returning to the journal profile, so no Go defer can persist completion.
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "journal.json")
	effectPath := filepath.Join(dir, "external-effect")
	now := time.Now().UTC()
	binary, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), binary, "-test.run=^TestJournalCrashChild$")
	cmd.Env = append(os.Environ(), "TOOLSY_JOURNAL_CRASH="+path, "TOOLSY_JOURNAL_EFFECT="+effectPath,
		"TOOLSY_JOURNAL_CLOCK="+now.Format(time.RFC3339Nano))
	// Act: abrupt process loss between external commit and result persistence.
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, string(output))
	require.Equal(t, simulatedCrashExit, exitErr.ExitCode())
	effect, err := os.ReadFile(effectPath)
	require.NoError(t, err)
	require.Equal(t, "remote written", string(effect))
	store, err := Open(path, 0)
	require.NoError(t, err)
	call := toolsy.PreparedCall{Manifest: toolsy.ToolManifest{Name: "write"}}
	var redispatches int
	invoke := func(func(toolsy.Chunk) error) error { redispatches++; return nil }
	consume := func(toolsy.Chunk) error { return nil }
	err = crashProfile(t, store, now, "recover").ExecutePrepared(ctx, call, invoke, consume)
	require.ErrorContains(t, err, string(toolsy.OperationInProgress))
	now = now.Add(2 * time.Minute)
	err = crashProfile(t, store, now, "recover").ExecutePrepared(ctx, call, invoke, consume)
	require.ErrorContains(t, err, string(toolsy.OperationUnknown))
	// Trusted host reconciliation queries the external system and persists its
	// proven complete result. No model payload acts as a reconciliation decision.
	var unknown *toolsy.OperationStateError
	require.ErrorAs(t, err, &unknown)
	codec := toolsy.JSONResultCodec[string, string]{}
	raw, err := codec.EncodeResult(toolsy.Chunk{Event: toolsy.EventResult, Data: []byte(`"remote written"`),
		MimeType: toolsy.MimeTypeJSON, TypedResult: "remote written"})
	require.NoError(t, err)
	require.NoError(t, toolsy.ReconcileOperation(ctx, store, unknown.Record.Binding, func() time.Time { return now },
		func(context.Context, toolsy.OperationRecord) (toolsy.ReconciliationDecision, error) {
			return toolsy.ReconciliationDecision{
				State:   toolsy.OperationCompleted,
				Result:  raw,
				ProofID: "external receipt",
			}, nil
		}))
	var replay toolsy.Chunk
	require.NoError(
		t,
		crashProfile(
			t,
			store,
			now,
			"recover",
		).ExecutePrepared(ctx, call, invoke, func(c toolsy.Chunk) error { replay = c; return nil }),
	)
	// Assert: the remote effect is recovered, never blindly run a second time.
	assert.Zero(t, redispatches)
	assert.Equal(t, "remote written", replay.TypedResult)
	assert.Equal(t, toolsy.ReplaySourceOperation, replay.ToolEnvelope().Metadata[toolsy.ReplaySourceMetadata])
}

func TestJournalCrashChild(t *testing.T) {
	path := os.Getenv("TOOLSY_JOURNAL_CRASH")
	if path == "" {
		t.Skip("abrupt-process-loss helper")
	}
	now, err := time.Parse(time.RFC3339Nano, os.Getenv("TOOLSY_JOURNAL_CLOCK"))
	require.NoError(t, err)
	store, err := Open(path, 0)
	require.NoError(t, err)
	call := toolsy.PreparedCall{Manifest: toolsy.ToolManifest{Name: "write"}}
	err = crashProfile(
		t,
		store,
		now,
		"crashed",
	).ExecutePrepared(context.Background(), call, func(func(toolsy.Chunk) error) error {
		require.NoError(
			t,
			os.WriteFile(os.Getenv("TOOLSY_JOURNAL_EFFECT"), []byte("remote written"), privateEffectMode),
		)
		os.Exit(simulatedCrashExit) // deliberate process loss; no deferred profile cleanup.
		return nil
	}, func(toolsy.Chunk) error { return nil })
	require.NoError(t, err)
	t.Fatal("crash helper returned")
}

func TestJournalCorruptReverseLinkFailsBeforeDispatch(t *testing.T) {
	// Arrange: durable approval/claim, then a storage image missing its reservation.
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "journal.json")
	store := seedJournal(t, path, now)
	first, err := store.Claim(t.Context(), journalClaim(now))
	require.NoError(t, err)
	require.True(t, first.Dispatch)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var snapshot toolsy.OperationSnapshot
	require.NoError(t, json.Unmarshal(raw, &snapshot))
	delete(snapshot.Consumed, "grant")
	corrupt, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, corrupt, privateEffectMode))
	reopened, err := Open(path, 0)
	require.NoError(t, err)
	claim := journalClaim(now.Add(2 * time.Minute))
	claim.AttemptID = "recovery"
	// Act.
	result, err := reopened.Claim(t.Context(), claim)
	_, found, inspectErr := reopened.Inspect(t.Context(), claim.Binding)
	// Assert: corrupt image never returns permission or a trusted record, nor rewrites bytes.
	var claimCause, inspectCause *toolsy.OperationError
	require.ErrorAs(t, err, &claimCause)
	require.ErrorAs(t, inspectErr, &inspectCause)
	assert.Equal(t, "corrupt_snapshot", claimCause.Kind)
	assert.Equal(t, "corrupt_snapshot", inspectCause.Kind)
	assert.False(t, result.Dispatch)
	assert.False(t, found)
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, corrupt, after)
}

func TestJournalRejectsUnusedGrantBeforeDispatch(t *testing.T) {
	// Arrange: unapproved claim carries a grant it must not consume.
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "journal.json")
	store := seedJournal(t, path, now)
	before, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	claim := journalClaim(now)
	claim.RequiresApproval = false
	// Act.
	result, err := store.Claim(t.Context(), claim)
	// Assert: no effect permission and no invalid persisted image.
	require.ErrorContains(t, err, "invalid_claim")
	assert.False(t, result.Dispatch)
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
	claim.GrantID = ""
	result, err = store.Claim(t.Context(), claim)
	require.NoError(t, err)
	assert.True(t, result.Dispatch)
	require.NoError(t, store.Finish(t.Context(), toolsy.OperationFinish{Binding: claim.Binding,
		AttemptID: claim.AttemptID, State: toolsy.OperationCompleted, Result: []byte("done")}))
	reopened, openErr := Open(path, 0)
	require.NoError(t, openErr)
	record, found, inspectErr := reopened.Inspect(t.Context(), claim.Binding)
	require.NoError(t, inspectErr)
	assert.True(t, found)
	assert.Equal(t, toolsy.OperationCompleted, record.State)
}
