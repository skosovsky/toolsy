package toolsy

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrentApprovedResumesDispatchOnce(t *testing.T) {
	// Arrange: keep the winner in its handler while all other approved resumes arrive.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	now := time.Now()
	store := NewMemoryOperationStore()
	grantID := ""
	entered, release := make(chan struct{}, 8), make(chan struct{})
	var releaseOnce sync.Once
	releaseWorkers := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseWorkers)
	var handlers, policies atomic.Int32
	tool, err := NewTypedTool(
		TypedToolSpec[NoSubject, NoScope, struct{}, string, string]{
			Name:        "write",
			Description: "Write",
			Options:     []ToolOption{WithRequiresConfirmation()},
			Policy: func(context.Context, TypedPolicyRequest[NoSubject, NoScope, struct{}]) Decision {
				policies.Add(1)
				return AllowDecision()
			},
			Handler: func(ctx context.Context, _ TypedCallContext[NoSubject, NoScope], _ *RunEnv, _ ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
				handlers.Add(1)
				entered <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					return ToolResult[string, string]{}, ctx.Err()
				}
				return NewToolResult[string, string]("written"), nil
			},
		},
	)
	require.NoError(t, err)
	profile, err := NewOperationProfile(OperationProfileConfig{
		Store: store,
		Prepare: func(_ context.Context, call PreparedCall) (OperationIntent, error) {
			return OperationIntent{
				Namespace:         "workers",
				Subject:           "host",
				Scope:             "tenant",
				OperationID:       "intent",
				AttemptID:         call.Input.CallID,
				GrantID:           grantID,
				PolicyFingerprint: "policy",
				CanonicalDigest:   "host dependency",
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
	reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(tool).Build()
	require.NoError(t, err)
	call := ToolCall{ToolName: "write", Input: ToolInput{CallID: "challenge", ArgsJSON: []byte(`{}`)}}
	err = reg.Execute(ctx, call, func(Chunk) error { return nil })
	var pending *PendingApprovalError
	require.ErrorAs(t, err, &pending)
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
	results := make(chan error, 8)
	// Act: eight independent delivery attempts share one logical operation/grant.
	for worker := range 8 {
		go func() {
			workerCall := call
			workerCall.Input.CallID = "worker-" + strconv.Itoa(worker)
			results <- reg.Execute(ctx, workerCall, func(Chunk) error { return nil })
		}()
	}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("no handler entered before test deadline")
	}
	for range 7 {
		var state *OperationStateError
		workerErr := <-results
		require.ErrorAs(t, workerErr, &state)
		assert.Equal(t, OperationInProgress, state.Record.State)
	}
	releaseWorkers()
	require.NoError(t, <-results)
	call.Input.CallID = "replay"
	require.NoError(t, reg.Execute(ctx, call, func(c Chunk) error {
		assert.Equal(t, ReplaySourceOperation, c.ToolEnvelope().Metadata[ReplaySourceMetadata])
		return nil
	}))
	// Assert: all current policies ran, exactly one handler dispatched, grant consumed once.
	assert.Equal(t, int32(1), handlers.Load())
	assert.Equal(t, int32(10), policies.Load())
	assert.Len(t, store.Snapshot().Consumed, 1)
}
