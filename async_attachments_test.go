package toolsy

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAsAsyncTool_BackgroundDoesNotInheritSyncEnvAttachments(t *testing.T) {
	var (
		bgCount int
		done    sync.WaitGroup
	)
	done.Add(1)

	base := minTool{
		manifest: ToolManifest{Name: "probe", Parameters: map[string]any{"type": "object"}},
		execute: func(_ context.Context, run *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			bgCount = len(run.Attachments())
			done.Done()
			return nil
		},
	}
	wrapped := AsAsyncTool(base)

	syncEnv := NewRunEnv()
	syncEnv = syncEnv.cloneForExecute([]Attachment{{MimeType: MimeTypePNG, Data: []byte{1}}}, nil)

	err := wrapped.Execute(
		context.Background(),
		syncEnv,
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.Zero(t, bgCount, "background must not inherit sync invocation attachments when input has none")
}

func TestAsAsyncTool_BackgroundGetsAttachmentsFromToolInput(t *testing.T) {
	var (
		bgCount int
		done    sync.WaitGroup
	)
	done.Add(1)

	base, err := NewTool(
		"att_probe",
		"probe",
		func(_ context.Context, run *RunEnv, _ struct{}) (struct{}, error) {
			bgCount = len(run.Attachments())
			done.Done()
			return struct{}{}, nil
		},
	)
	require.NoError(t, err)

	wrapped := AsAsyncTool(base)
	err = wrapped.Execute(
		context.Background(),
		NewRunEnv(),
		ToolInput{
			ArgsJSON:    []byte(`{}`),
			Attachments: []Attachment{{MimeType: MimeTypeJSON, Data: []byte(`"x"`)}},
		},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.Equal(t, 1, bgCount)
}
