//go:build unix

package fstool

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFIFORejectedWithoutPeer(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(base, "fifo"), 0o600))
	var o options
	applyDefaults(&o)
	for _, operation := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"read", func(ctx context.Context) error { _, err := doReadFile(ctx, base, &o, "fifo"); return err }},
		{"write", func(ctx context.Context) error { _, err := doWriteFile(ctx, base, "fifo", "content"); return err }},
		{"list", func(ctx context.Context) error { _, err := doListDir(ctx, base, &o, "fifo"); return err }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- operation.run(ctx) }()
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(time.Second):
				cancel()
				t.Fatal("FIFO open blocked without a peer")
			}
		})
	}
}
