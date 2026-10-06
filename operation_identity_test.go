package toolsy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationNamespaceScopeAndNewIntentAreIndependent(t *testing.T) {
	// Arrange: identical inputs; identity comes exclusively from the host.
	now := time.Now()
	store := NewMemoryOperationStore()
	intent := OperationIntent{
		Namespace:         "writes",
		Scope:             "tenant-a",
		Subject:           "alice",
		OperationID:       "intent-a",
		AttemptID:         "attempt",
		PolicyFingerprint: "policy",
		CanonicalDigest:   "digest",
		CanonicalRules:    "json",
		DisplayJSON:       []byte(`{}`),
	}
	profile, err := NewOperationProfile(
		OperationProfileConfig{
			Store:    store,
			Prepare:  func(context.Context, PreparedCall) (OperationIntent, error) { return intent, nil },
			Codec:    JSONResultCodec[string, string]{},
			Issuer:   "host",
			Clock:    func() time.Time { return now },
			Lease:    time.Minute,
			MaxBytes: 0,
		},
	)
	require.NoError(t, err)
	call := PreparedCall{Manifest: ToolManifest{Name: "write"}, Input: ToolInput{ArgsJSON: []byte(`{}`)}}
	var calls int
	invoke := func(yield func(Chunk) error) error {
		calls++
		return yield(
			Chunk{Event: EventResult, Data: []byte(`"written"`), MimeType: MimeTypeJSON, TypedResult: "written"},
		)
	}
	consume := func(Chunk) error { return nil }
	// Act: repeat delivery, a new intentional action, another tenant and namespace.
	require.NoError(t, profile.ExecutePrepared(context.Background(), call, invoke, consume))
	require.NoError(t, profile.ExecutePrepared(context.Background(), call, invoke, consume))
	intent.OperationID = "intent-b"
	require.NoError(t, profile.ExecutePrepared(context.Background(), call, invoke, consume))
	intent.Scope = "tenant-b"
	require.NoError(t, profile.ExecutePrepared(context.Background(), call, invoke, consume))
	intent.Namespace = "another-host"
	require.NoError(t, profile.ExecutePrepared(context.Background(), call, invoke, consume))
	// Assert: same-input new intent/scopes execute independently; repeat does not.
	assert.Equal(t, 4, calls)
	assert.Len(t, store.Snapshot().Records, 4)
}

func TestOperationDownstreamIdempotencyKeepsOriginalKeyOnAuthorizedRetry(t *testing.T) {
	// Arrange: this fake downstream explicitly guarantees deduplication by key.
	// That guarantee belongs to the remote service, not to the local journal.
	now := time.Now()
	store := NewMemoryOperationStore()
	remote := map[string]string{}
	var commits, dispatches int
	var observedKeys []string
	attempt := "first"
	profile, err := NewOperationProfile(OperationProfileConfig{
		Store: store,
		Prepare: func(_ context.Context, call PreparedCall) (OperationIntent, error) {
			return OperationIntent{
				Namespace:         "writes",
				Scope:             call.Context.Scope.(string),
				Subject:           call.Context.Subject.(string),
				OperationID:       "intent",
				AttemptID:         attempt,
				DownstreamKey:     call.Context.Values["downstream_key"].(string),
				PolicyFingerprint: "policy",
				CanonicalDigest:   "digest",
				CanonicalRules:    "json",
				DisplayJSON:       []byte(`{}`),
			}, nil
		},
		Codec:    JSONResultCodec[string, string]{},
		Issuer:   "host",
		Clock:    func() time.Time { return now },
		Lease:    time.Minute,
		MaxBytes: 0,
	})
	require.NoError(t, err)
	tool, err := NewTool("write", "Write", func(_ context.Context, env *RunEnv, _ struct{}) (string, error) {
		dispatches++
		key := env.CallContext().Values["downstream_key"].(string)
		observedKeys = append(observedKeys, key)
		if value, found := remote[key]; found {
			return value, nil
		}
		commits++
		remote[key] = "remote receipt"
		return "", errors.New("response lost after commit")
	})
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(tool).Build()
	require.NoError(t, err)
	call := ToolCall{
		ToolName:    "write",
		Input:       ToolInput{ArgsJSON: []byte(`{}`)},
		CallContext: NewCallContext("alice", "tenant", WithCallValue("downstream_key", "host-original-key")),
	}
	// Act: unknown first outcome. Only a verified downstream contract authorizes retry.
	err = reg.Execute(context.Background(), call, func(Chunk) error { return nil })
	var unknown *OperationOutcomeError
	require.ErrorAs(t, err, &unknown)
	require.NoError(t, ReconcileOperation(context.Background(), store, unknown.Binding, func() time.Time { return now },
		func(_ context.Context, record OperationRecord) (ReconciliationDecision, error) {
			assert.Equal(t, "host-original-key", record.Binding.DownstreamKey)
			return ReconciliationDecision{
				State:   OperationRetryAuthorized,
				ProofID: "verified remote idempotency contract",
			}, nil
		}))
	attempt = "second"
	var result Chunk
	require.NoError(t, reg.Execute(context.Background(), call, func(c Chunk) error { result = c; return nil }))
	// Assert: two authorized attempts, one remote write, same downstream key.
	assert.Equal(t, 2, dispatches)
	assert.Equal(t, 1, commits)
	assert.Equal(t, []string{"host-original-key", "host-original-key"}, observedKeys)
	assert.Equal(t, []byte(`"remote receipt"`), result.Data)
}
