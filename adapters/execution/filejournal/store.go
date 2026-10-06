// Package filejournal provides a local durable operation/approval journal.
// It is a host-selected adapter, not a distributed transaction coordinator.
package filejournal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/skosovsky/toolsy"
)

const defaultLimit = 8 << 20

// ErrLimitExceeded is a host-visible quota failure, never dispatch permission.
var ErrLimitExceeded = errors.New("filejournal: journal limit exceeded")

// Store owns one journal file and its persistent advisory lock. The host owns
// the trusted local directory, access controls, encryption, quotas and retention.
// Network filesystems and untrusted/writable-by-others directories are unsupported.
type Store struct {
	path     string
	maxBytes int64
}

// Open uses an absolute filename in an existing host-owned directory. A zero
// limit selects a bounded default. Missing files are created on the first write.
func Open(path string, maxBytes int64) (*Store, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || maxBytes < 0 {
		return nil, errors.New("filejournal: absolute clean filename and nonnegative limit required")
	}
	if err := platformSupported(); err != nil {
		return nil, err
	}
	dir, err := os.Lstat(filepath.Dir(path)) // #nosec G703 -- host-owned absolute clean path, not tool/model input.
	if err != nil {
		return nil, err
	}
	if !dir.IsDir() || dir.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("filejournal: trusted directory required")
	}
	if maxBytes == 0 {
		maxBytes = defaultLimit
	}
	return &Store{path: path, maxBytes: maxBytes}, nil
}

func (s *Store) PutGrant(ctx context.Context, grant toolsy.ApprovalGrant) error {
	return s.transact(ctx, func(memory *toolsy.MemoryOperationStore) error { return memory.PutGrant(ctx, grant) })
}

func (s *Store) Inspect(
	ctx context.Context,
	binding toolsy.OperationBinding,
) (toolsy.OperationRecord, bool, error) {
	var record toolsy.OperationRecord
	var found bool
	readErr := func() (err error) {
		lock, err := lockJournal(ctx, s.path+".lock")
		if err != nil {
			return &toolsy.OperationStoreError{Stage: "inspect_lock", Cause: err}
		}
		defer func() {
			if closeErr := lock.Close(); closeErr != nil {
				err = errors.Join(err, &toolsy.OperationStoreError{Stage: "inspect_unlock", Cause: closeErr})
			}
		}()
		memory, err := s.load()
		if err != nil {
			return &toolsy.OperationStoreError{Stage: "inspect_read", Cause: err}
		}
		record, found, err = memory.Inspect(ctx, binding)
		return err
	}()
	if readErr != nil {
		return toolsy.OperationRecord{}, false, readErr
	}
	return record, found, nil
}

func (s *Store) Claim(ctx context.Context, claim toolsy.OperationClaim) (toolsy.ClaimResult, error) {
	var result toolsy.ClaimResult
	err := s.transact(ctx, func(memory *toolsy.MemoryOperationStore) error {
		var claimErr error
		result, claimErr = memory.Claim(ctx, claim)
		return claimErr
	})
	if err != nil {
		return toolsy.ClaimResult{}, err
	} // never dispatch on uncertain persistence.
	return result, nil
}

func (s *Store) Finish(ctx context.Context, finish toolsy.OperationFinish) error {
	return s.transact(ctx, func(memory *toolsy.MemoryOperationStore) error { return memory.Finish(ctx, finish) })
}

func (s *Store) Resolve(ctx context.Context, resolution toolsy.OperationResolution) error {
	return s.transact(ctx, func(memory *toolsy.MemoryOperationStore) error { return memory.Resolve(ctx, resolution) })
}

func (s *Store) transact(ctx context.Context, change func(*toolsy.MemoryOperationStore) error) (err error) {
	defer func() {
		var decision *toolsy.OperationError
		if err != nil && !errors.As(err, &decision) {
			err = &toolsy.OperationStoreError{Stage: "file_transaction", Cause: err}
		}
	}()
	lock, err := lockJournal(ctx, s.path+".lock")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	memory, err := s.load()
	if err != nil {
		return err
	}
	if err = change(memory); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	raw, err := json.Marshal(memory.Snapshot())
	if err != nil {
		return err
	}
	if int64(len(raw)) > s.maxBytes {
		return ErrLimitExceeded
	}
	return s.persist(raw)
}

func (s *Store) load() (*toolsy.MemoryOperationStore, error) {
	info, err := os.Lstat(s.path) // #nosec G703 -- constructor validates trusted host path; regular-file check below.
	if errors.Is(err, os.ErrNotExist) {
		return toolsy.NewMemoryOperationStore(), nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > s.maxBytes {
		return nil, errors.New("filejournal: invalid journal file or limit exceeded")
	}
	f, err := os.Open(s.path) // #nosec G703 -- trusted local directory, regular file checked above.
	if err != nil {
		return nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, s.maxBytes+1))
	closeErr := f.Close()
	if err = errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	if int64(len(raw)) > s.maxBytes {
		return nil, ErrLimitExceeded
	}
	var snapshot toolsy.OperationSnapshot
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return nil, fmt.Errorf("filejournal: corrupt journal: %w", err)
	}
	return toolsy.RestoreOperationStore(snapshot)
}

func (s *Store) persist(raw []byte) (err error) {
	dir := filepath.Dir(s.path)
	f, err := os.CreateTemp(dir, ".toolsy-journal-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() {
		removeErr := os.Remove(name) // #nosec G703 -- generated temporary filename, not caller data.
		if !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()
	_, writeErr := f.Write(raw)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	if err = os.Rename(name, s.path); err != nil { // #nosec G703 -- validated host paths.
		return err
	}
	directory, err := os.Open(
		dir,
	) // #nosec G703 -- constructor-validated host-owned directory, needed for durability sync.
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}
