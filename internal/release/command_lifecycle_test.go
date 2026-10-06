//go:build darwin || linux

package release

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCommandCancellationBoundsIgnoringProcessGroup(t *testing.T) {
	// Arrange: the command and its descendant ignore the first termination signal.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	// Act.
	_, err := (commands{env: nil, private: false}).run(ctx, t.TempDir(), "sh", "-c", "trap '' TERM; sleep 30 & wait")
	// Assert: pipe wait is bounded and final SIGKILL terminates the owned group.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline cause lost: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("cancellation took %s", elapsed)
	}
}
