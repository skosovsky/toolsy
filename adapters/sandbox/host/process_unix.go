//go:build unix

package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const groupObservationInterval = 10 * time.Millisecond

func newRuntimeCommand(_ context.Context, command string, args ...string) *exec.Cmd {
	//nolint:noctx // The owned cancellation watcher joins before the group leader is reaped.
	return exec.Command(command, args...)
}

type streamCollection struct {
	err     error
	stopErr error
}

// runRuntime launches the guest directly; a separate unreaped anchor pins its PGID.
//
//nolint:funlen // Every owned process, pipe and watcher is joined before returning.
func runRuntime(ctx context.Context, cmd *exec.Cmd) (int, error, error) {
	if err := ctx.Err(); err != nil {
		return 0, err, nil
	}
	if cmd.Err != nil {
		return 0, cmd.Err, nil
	}
	holdRead, holdWrite, err := os.Pipe()
	if err != nil {
		return 0, err, nil
	}
	defer holdRead.Close()
	defer holdWrite.Close()
	//nolint:noctx // Cancellation is owned by the joined watcher; no signal may follow anchor.Wait.
	anchor := exec.Command("/bin/sh", "-c", "IFS= read -r hold")
	anchor.Stdin, anchor.Dir, anchor.Env = holdRead, cmd.Dir, cmd.Env
	var anchorAttr syscall.SysProcAttr
	anchorAttr.Setpgid = true
	anchor.SysProcAttr = &anchorAttr
	if err := anchor.Start(); err != nil {
		return 0, err, nil
	}
	_ = holdRead.Close()
	var guestAttr syscall.SysProcAttr
	guestAttr.Setpgid = true
	guestAttr.Pgid = anchor.Process.Pid
	cmd.SysProcAttr = &guestAttr
	stdoutWriter, stderrWriter := cmd.Stdout, cmd.Stderr
	outRead, outWrite, outErr := os.Pipe()
	if outErr != nil {
		stopErr := stopAnchor(anchor)
		return 0, outErr, stopErr
	}
	defer outRead.Close()
	defer outWrite.Close()
	errRead, errWrite, errErr := os.Pipe()
	if errErr != nil {
		stopErr := stopAnchor(anchor)
		return 0, errErr, stopErr
	}
	defer errRead.Close()
	defer errWrite.Close()
	cmd.Stdout, cmd.Stderr = outWrite, errWrite
	if err := cmd.Start(); err != nil {
		stopErr := stopAnchor(anchor)
		return 0, err, stopErr
	}
	_ = outWrite.Close()
	_ = errWrite.Close()
	var owner ownedProcessGroup
	owner.anchor, owner.guest = anchor, cmd
	collected := make(chan streamCollection, 2)
	collect := func(reader *os.File, writer io.Writer) {
		_, copyErr := io.Copy(writer, reader)
		var stopErr error
		if copyErr != nil {
			stopErr = owner.stop()
		}
		collected <- streamCollection{err: copyErr, stopErr: stopErr}
	}
	go collect(outRead, stdoutWriter)
	go collect(errRead, stderrWriter)
	stopWatch, watchDone := make(chan struct{}), make(chan struct{})
	var watchStopErr error
	go func() {
		defer close(watchDone)
		select {
		case <-ctx.Done():
			watchStopErr = owner.stop()
		case <-stopWatch:
		}
	}()
	// File writers prevent Cmd.Wait from waiting on inherited child pipe descriptors.
	waitErr := cmd.Wait()
	exitCode := 0
	if exitErr, ok := errors.AsType[*exec.ExitError](waitErr); ok {
		exitCode = exitErr.ExitCode()
		waitErr = nil
	}
	stopErr := errors.Join(owner.stop(), owner.sweep())
	timer := time.NewTimer(cleanupTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(groupObservationInterval)
	defer ticker.Stop()
	for received := 0; received < 2; {
		select {
		case item := <-collected:
			received++
			waitErr = errors.Join(waitErr, item.err)
			stopErr = errors.Join(stopErr, item.stopErr)
		case <-ticker.C:
			stopErr = errors.Join(stopErr, owner.sweep())
		case <-timer.C:
			_ = outRead.Close()
			_ = errRead.Close()
			waitErr = errors.Join(waitErr, exec.ErrWaitDelay)
		}
	}
	stopErr = errors.Join(stopErr, owner.sweep())
	close(stopWatch)
	<-watchDone
	stopErr = errors.Join(stopErr, watchStopErr)
	// Collectors and watcher can no longer signal the group; only now reap its anchor.
	anchorErr := anchor.Wait()
	if !expectedAnchorStop(anchorErr) {
		stopErr = errors.Join(stopErr, anchorErr)
	}
	if stopErr == nil {
		stopErr = confirmGroupStopped(anchor.Process.Pid)
	}
	return exitCode, waitErr, stopErr
}

func stopAnchor(anchor *exec.Cmd) error {
	stopErr := stopOwnedWork(anchor, nil, killOwnedGroup)
	waitErr := anchor.Wait()
	if !expectedAnchorStop(waitErr) {
		stopErr = errors.Join(stopErr, waitErr)
	}
	if stopErr == nil {
		stopErr = confirmGroupStopped(anchor.Process.Pid)
	}
	return stopErr
}

func killOwnedGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return fmt.Errorf("kill owned process group: %w", err)
}

func expectedAnchorStop(err error) bool {
	if err == nil {
		return true
	}
	exitErr, ok := errors.AsType[*exec.ExitError](err)
	if !ok {
		return false
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && status.Signal() == syscall.SIGKILL
}

func confirmGroupStopped(group int) error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	ticker := time.NewTicker(groupObservationInterval)
	defer ticker.Stop()
	for {
		err := syscall.Kill(-group, 0) // Observation only: never signal a reaped/reused group.
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return fmt.Errorf("observe owned process group: %w", err)
		}
		// Darwin may transiently report EPERM for dying group members. It is
		// not confirmation; keep observing until ESRCH or the cleanup deadline.
		select {
		case <-ctx.Done():
			return fmt.Errorf("owned group still exists: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// A failed group signal still stops owned direct children; failures retain the
// group diagnostic and cannot be mistaken for confirmed descendant cleanup.
func stopOwnedWork(anchor, guest *exec.Cmd, stopGroup func(*exec.Cmd) error) error {
	err := stopGroup(anchor)
	if err == nil {
		return nil
	}
	for _, cmd := range []*exec.Cmd{guest, anchor} {
		if cmd == nil || cmd.Process == nil {
			continue
		}
		killErr := cmd.Process.Kill()
		if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			err = errors.Join(err, killErr)
		}
	}
	return err
}

// A successful SIGKILL is sticky: repeated signals to dying/zombie groups can
// return EPERM on Darwin. All concurrent completion paths share this owner.
type ownedProcessGroup struct {
	mu      sync.Mutex
	anchor  *exec.Cmd
	guest   *exec.Cmd
	stopped bool
	stopErr error
}

func (o *ownedProcessGroup) stop() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.stopped {
		o.stopErr = stopOwnedWork(o.anchor, o.guest, killOwnedGroup)
		o.stopped = true
	}
	return o.stopErr
}

// The guest may fork concurrently with the first group signal. Its Wait has now
// completed, but the unreaped anchor still pins the PGID. A bounded collection
// phase can sweep newly born descendants safely. EPERM for already dying members
// is not confirmation: confirmGroupStopped must still observe ESRCH after reap.
func (o *ownedProcessGroup) sweep() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.stopped || o.stopErr != nil {
		return o.stopErr
	}
	err := killOwnedGroup(o.anchor)
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
