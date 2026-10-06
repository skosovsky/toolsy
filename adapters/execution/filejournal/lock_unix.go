//go:build darwin || linux

package filejournal

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

const lockPollInterval = 10 * time.Millisecond
const privateLockMode = 0o600

func platformSupported() error { return nil }

func lockJournal(ctx context.Context, path string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// O_NOFOLLOW refuses a substituted lock symlink. Closing releases flock.
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, privateLockMode)
	if err != nil {
		return nil, err
	}
	if fd < 0 {
		return nil, errors.New("filejournal: invalid lock descriptor")
	}
	f := os.NewFile(uintptr(fd), path) // #nosec G115 -- successful OS descriptor is nonnegative and fits uintptr.
	ticker := time.NewTicker(lockPollInterval)
	defer ticker.Stop()
	for {
		err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return nil, errors.Join(err, f.Close())
		}
		select {
		case <-ctx.Done():
			return nil, errors.Join(ctx.Err(), f.Close())
		case <-ticker.C:
		}
	}
}
