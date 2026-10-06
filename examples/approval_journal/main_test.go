package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrdinaryHostPendingApproveRestartReplay(t *testing.T) {
	// Arrange: host-owned local journal and receipt, no remote calls or agent loop.
	cfg := config{Directory: t.TempDir(), OperationID: "intent", Note: "ordinary", Approve: false}
	var output bytes.Buffer
	// Act/Assert: pending alone cannot create an external receipt.
	require.NoError(t, run(context.Background(), cfg, &output))
	assert.Contains(t, output.String(), "pending action=")
	_, err := os.Stat(filepath.Join(cfg.Directory, "effects.jsonl"))
	require.ErrorIs(t, err, os.ErrNotExist)
	cfg.Approve = true
	require.NoError(t, run(context.Background(), cfg, &output))
	first, err := os.ReadFile(filepath.Join(cfg.Directory, "effects.jsonl"))
	require.NoError(t, err)
	// Act: create fresh host/store/session objects, repeating the same logical intent.
	output.Reset()
	require.NoError(t, run(context.Background(), cfg, &output))
	second, err := os.ReadFile(filepath.Join(cfg.Directory, "effects.jsonl"))
	require.NoError(t, err)
	// Assert: restart replays complete output, never issues a new approval/effect.
	assert.Equal(t, first, second)
	assert.Contains(t, output.String(), "replay=completed_operation")
	assert.NotContains(t, output.String(), "pending action=")
	// Act/Assert: changing args under the old intent conflicts; a new intent is explicit.
	cfg.Note = "changed"
	require.Error(t, run(context.Background(), cfg, &output))
	cfg.OperationID = "new-intent"
	require.NoError(t, run(context.Background(), cfg, &output))
	third, err := os.ReadFile(filepath.Join(cfg.Directory, "effects.jsonl"))
	require.NoError(t, err)
	assert.Len(t, bytes.Split(bytes.TrimSpace(third), []byte("\n")), 2)
}
