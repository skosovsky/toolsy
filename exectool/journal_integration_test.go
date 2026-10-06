//go:build linux || darwin

package exectool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/adapters/execution/filejournal"
)

func TestCollectionFailureAfterEffectRemainsUnknownAcrossJournalRestart(t *testing.T) {
	// Arrange: a real persisted receipt is committed before output collection fails.
	ctx := context.Background()
	directory := t.TempDir()
	receipt := filepath.Join(directory, "effect.txt")
	journal := filepath.Join(directory, "operations.json")
	store, err := filejournal.Open(journal, 0)
	require.NoError(t, err)
	now := time.Now()
	dispatches := 0
	collectionErr := errors.New("output collection disconnected")
	sandbox := &mockSandbox{
		languages: []string{"test"},
		runFn: func(context.Context, RunRequest) (RunResult, error) {
			dispatches++
			if writeErr := os.WriteFile(receipt, []byte("committed once"), 0o600); writeErr != nil {
				return RunResult{}, writeErr
			}
			return RunResult{}, errors.Join(ErrSandboxFailure, collectionErr)
		},
	}
	tool, err := New(sandbox, WithToolOptions(toolsy.WithRequiresConfirmation()))
	require.NoError(t, err)
	grantID := ""
	prepare := func(ctx context.Context, call toolsy.PreparedCall) (toolsy.OperationIntent, error) {
		if contextErr := ctx.Err(); contextErr != nil {
			return toolsy.OperationIntent{}, contextErr
		}
		digest := sha256.Sum256(call.Input.ArgsJSON)
		return toolsy.OperationIntent{
			Namespace: "sandbox", Subject: "host-user", Scope: "host-workspace", OperationID: "intent",
			AttemptID: call.Input.CallID, GrantID: grantID, PolicyFingerprint: "host-policy",
			CanonicalDigest: hex.EncodeToString(digest[:]), CanonicalRules: "prepared-json",
			DisplayJSON: call.Input.ArgsJSON,
		}, nil
	}
	newRegistry := func(journalStore *filejournal.Store) *toolsy.Registry {
		profile, profileErr := toolsy.NewOperationProfile(journalStore, prepare,
			toolsy.JSONResultCodec[RunResult, struct{}]{}, "host", func() time.Time { return now }, time.Minute, 0)
		require.NoError(t, profileErr)
		registry, buildErr := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(profile)).Add(tool).Build()
		require.NoError(t, buildErr)
		return registry
	}
	registry := newRegistry(store)
	call := toolsy.ToolCall{ToolName: "exec_code", Input: toolsy.ToolInput{
		CallID: "request", ArgsJSON: []byte(`{"language":"test","code":"external effect"}`),
	}}
	successfulResults := 0
	yield := func(chunk toolsy.Chunk) error {
		if chunk.Event == toolsy.EventResult && !chunk.IsError {
			successfulResults++
		}
		return nil
	}
	var pending *toolsy.PendingApprovalError
	require.ErrorAs(t, registry.Execute(ctx, call, yield), &pending)
	require.Zero(t, dispatches)
	require.NoError(t, store.PutGrant(ctx, toolsy.ApprovalGrant{
		ID: "grant", Issuer: "host", Binding: pending.Challenge.Binding,
		IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	}))
	grantID = "grant"
	call.Input.CallID = "dispatch"

	// Act: infrastructure failure follows a committed effect; then host reopens the durable store.
	err = registry.Execute(ctx, call, yield)
	var unknown *toolsy.OperationOutcomeError
	require.ErrorAs(t, err, &unknown)
	require.ErrorIs(t, err, collectionErr)
	toolError, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.False(t, toolError.Retryable)
	store, err = filejournal.Open(journal, 0)
	require.NoError(t, err)
	record, exists, inspectErr := store.Inspect(ctx, pending.Challenge.Binding)
	require.NoError(t, inspectErr)
	require.True(t, exists)
	require.Equal(t, toolsy.OperationUnknown, record.State)
	call.Input.CallID = "retry-after-restart"
	err = newRegistry(store).Execute(ctx, call, yield)

	// Assert: uncertainty requires host reconciliation; restart never grants a second dispatch.
	require.ErrorContains(t, err, string(toolsy.OperationUnknown))
	require.Equal(t, 1, dispatches)
	require.Zero(t, successfulResults)
	data, readErr := os.ReadFile(receipt)
	require.NoError(t, readErr)
	require.Equal(t, "committed once", string(data))
}
