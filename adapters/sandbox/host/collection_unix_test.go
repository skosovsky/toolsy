//go:build unix

package host

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

var errCollectionWriter = errors.New("collection refused")

type rejectCollectionWriter struct{}

func (rejectCollectionWriter) Write([]byte) (int, error) { return 0, errCollectionWriter }
func TestCollectionWriterFailureStopsForkingGuest(t *testing.T) {
	// Arrange: cleanup every known child even if a regression leaves the group alive.
	pidfile := filepath.Join(t.TempDir(), "child")
	second := filepath.Join(t.TempDir(), "second")
	t.Cleanup(func() {
		for _, f := range []string{pidfile, second} {
			b, _ := os.ReadFile(f)
			p, _ := strconv.Atoi(string(b))
			if p > 0 {
				_ = syscall.Kill(p, syscall.SIGKILL)
			}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := newRuntimeCommand(
		ctx,
		"/bin/sh",
		"-c",
		"/bin/sleep 30 & printf '%s' \"$!\" > \"$PIDFILE\"; printf output; /bin/sh -c 'printf %s \"$$\" > \"$SECOND\"; exec /bin/sleep 30'",
	)
	cmd.Env = []string{"PIDFILE=" + pidfile, "SECOND=" + second}
	cmd.Stdout = rejectCollectionWriter{}
	cmd.Stderr = &bytes.Buffer{}
	// Act.
	start := time.Now()
	_, err, cleanupErr := runRuntime(ctx, cmd)
	// Assert.
	if !errors.Is(err, errCollectionWriter) || cleanupErr != nil {
		t.Fatalf("execution=%v cleanup=%v", err, cleanupErr)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("collection failure did not stop group promptly")
	}
	b, e := os.ReadFile(pidfile)
	if e != nil {
		t.Fatal(e)
	}
	p, e := strconv.Atoi(string(b))
	if e != nil {
		t.Fatal(e)
	}
	if !errors.Is(syscall.Kill(p, 0), syscall.ESRCH) {
		t.Fatal("child survived")
	}
}
