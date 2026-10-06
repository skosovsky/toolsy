package sandboxfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/skosovsky/toolsy/exectool"
)

const (
	maxCleanupDirectoryDepth = 128
	cleanupDirectoryPageSize = 64
)

// RemoveWorkspace removes an owned workspace without following its child
// symlinks. Directory reads are paged and cancellation is checked between OS
// calls; an individual blocked filesystem syscall cannot be interrupted.
// The owner must stop guest access before cleanup and supply a fresh deadline.
func RemoveWorkspace(ctx context.Context, workspace string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(workspace)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: cleanup workspace must remain a directory, not a symlink", exectool.ErrSandboxFailure)
	}
	root, err := os.OpenRoot(workspace)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	removeErr := removeWorkspaceChildren(ctx, root, ".", 0)
	closeErr := root.Close()
	if err = errors.Join(removeErr, closeErr); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return os.Remove(workspace)
}

func removeWorkspaceChildren(ctx context.Context, root *os.Root, name string, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > maxCleanupDirectoryDepth {
		return fmt.Errorf("%w: workspace cleanup nesting exceeds 128 directories", exectool.ErrSandboxFailure)
	}
	directory, err := root.Open(name)
	if err != nil {
		return err
	}
	err = removeWorkspaceEntries(ctx, root, directory, name, depth)
	return errors.Join(err, directory.Close())
}

func removeWorkspaceEntries(ctx context.Context, root *os.Root, directory *os.File, name string, depth int) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := directory.ReadDir(cleanupDirectoryPageSize)
		for _, entry := range entries {
			if err := removeWorkspaceEntry(ctx, root, path.Join(name, entry.Name()), entry, depth); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func removeWorkspaceEntry(ctx context.Context, root *os.Root, child string, entry os.DirEntry, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry.IsDir() {
		if err := removeWorkspaceChildren(ctx, root, child, depth+1); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return root.Remove(child)
}
