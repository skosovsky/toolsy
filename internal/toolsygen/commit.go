package toolsygen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FileRecoveryError records a failed filesystem operation and the owned path at failure.
// Later rollback may restore that path; failed restore/remove diagnostics require host inspection.
// CommitComplete is true only for backup disposal after all outputs were installed.
// Filesystem rollback is best effort, never a crash-atomic multi-file transaction.
type FileRecoveryError struct {
	Target         string
	Backup         string
	Action         string
	CommitComplete bool
	Cause          error
}

func (e *FileRecoveryError) Error() string {
	return fmt.Sprintf(
		"%s: %s (recovery=%q, commit_complete=%t): %v",
		e.Target,
		e.Action,
		e.Backup,
		e.CommitComplete,
		e.Cause,
	)
}
func (e *FileRecoveryError) Unwrap() error { return e.Cause }

type stagedFile struct {
	path, tempPath, backupPath     string
	exists, backupMoved, installed bool
}

func (g *generator) commitFilesWithRollback(ctx context.Context, files []generatedFile, mode fs.FileMode) error {
	staged, err := g.stageFilesForCommit(ctx, files, mode)
	if err != nil {
		return err
	}
	committed, err := g.finalizeStagedCommits(ctx, staged)
	if err != nil {
		return err
	}
	return g.removeBackupFiles(committed)
}

func (g *generator) stageFilesForCommit(
	ctx context.Context,
	files []generatedFile,
	mode fs.FileMode,
) ([]stagedFile, error) {
	staged := make([]stagedFile, 0, len(files))

	for _, file := range files {
		if err := checkContext(ctx); err != nil {
			return g.failedStaging(staged, err)
		}
		existing, readErr := g.readFileLimited(ctx, file.Path)
		if readErr == nil && bytes.Equal(existing, file.Content) {
			continue
		}
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return g.failedStaging(staged, fmt.Errorf("%s: read existing file: %w", file.Path, readErr))
		}
		tempPath, err := g.writeTempFile(filepath.Dir(file.Path), filepath.Base(file.Path)+".tmp-*", file.Content, mode)
		if err != nil {
			return g.failedStaging(staged, fmt.Errorf("%s: %w", file.Path, err))
		}
		staged = append(
			staged,
			stagedFile{
				path:        file.Path,
				tempPath:    tempPath,
				backupPath:  "",
				exists:      readErr == nil,
				backupMoved: false,
				installed:   false,
			},
		)
	}
	return staged, nil
}

func (g *generator) finalizeStagedCommits(
	ctx context.Context,
	staged []stagedFile,
) ([]stagedFile, error) {
	for i := range staged {
		if err := checkContext(ctx); err != nil {
			return g.failedFinalization(staged, err)
		}
		if err := g.commitOneStagedFile(ctx, &staged[i]); err != nil {
			return g.failedFinalization(staged, err)
		}
	}
	if err := checkContext(ctx); err != nil {
		return g.failedFinalization(staged, err)
	}
	return staged, nil
}
func (g *generator) failedStaging(staged []stagedFile, cause error) ([]stagedFile, error) {
	return nil, errors.Join(cause, g.cleanupStagedTemps(staged))
}
func (g *generator) failedFinalization(staged []stagedFile, cause error) ([]stagedFile, error) {
	rollbackErr := g.rollbackCommitted(staged)
	cleanupErr := g.cleanupStagedTemps(staged)
	return nil, errors.Join(cause, rollbackErr, cleanupErr)
}

func (g *generator) commitOneStagedFile(ctx context.Context, sf *stagedFile) error {
	if sf.exists {
		path, err := g.reserveTempPath(filepath.Dir(sf.path), filepath.Base(sf.path)+".bak-*")
		sf.backupPath = path
		if err != nil {
			return recoveryError(*sf, "reserve backup", err)
		}
		if err := checkContext(ctx); err != nil {
			return err
		}
		if err := g.fs.rename(sf.path, sf.backupPath); err != nil {
			return recoveryError(*sf, "backup existing file", err)
		}
		sf.backupMoved = true
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := g.fs.rename(sf.tempPath, sf.path); err != nil {
		return recoveryError(*sf, "install generated file", err)
	}
	sf.tempPath = ""
	sf.installed = true
	return nil
}
func recoveryError(sf stagedFile, action string, cause error) error {
	return &FileRecoveryError{
		Target:         sf.path,
		Backup:         sf.backupPath,
		Action:         action,
		CommitComplete: false,
		Cause:          cause,
	}
}
func (g *generator) rollbackCommitted(staged []stagedFile) error {
	var errs []error
	for i := len(staged) - 1; i >= 0; i-- {
		sf := &staged[i]
		if sf.installed {
			if err := g.fs.remove(sf.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, recoveryError(*sf, "remove installed file during rollback", err))
				continue
			}
			sf.installed = false
		}
		if sf.backupMoved {
			if err := g.fs.rename(sf.backupPath, sf.path); err != nil {
				errs = append(errs, recoveryError(*sf, "restore backup during rollback", err))
				continue
			}
			sf.backupMoved = false
			sf.backupPath = ""
		}
	}
	return errors.Join(errs...)
}

// cleanupStagedTemps never deletes a moved old-content backup, even after failed restore.
func (g *generator) cleanupStagedTemps(staged []stagedFile) error {
	var errs []error
	for _, sf := range staged {
		for _, path := range []string{sf.tempPath, unusedReservation(sf)} {
			if path == "" {
				continue
			}
			if err := g.fs.remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(
					errs,
					&FileRecoveryError{
						Target:         sf.path,
						Backup:         path,
						Action:         "cleanup owned staging artifact",
						CommitComplete: false,
						Cause:          err,
					},
				)
			}
		}
	}
	return errors.Join(errs...)
}
func unusedReservation(sf stagedFile) string {
	if sf.backupMoved {
		return ""
	}
	return sf.backupPath
}

// Disposal is after commit. Failures retain backups and report that new outputs remain installed.
func (g *generator) removeBackupFiles(committed []stagedFile) error {
	var errs []error
	for _, sf := range committed {
		if sf.backupPath == "" {
			continue
		}
		if err := g.fs.remove(sf.backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(
				errs,
				&FileRecoveryError{
					Target:         sf.path,
					Backup:         sf.backupPath,
					Action:         "post-commit backup cleanup",
					CommitComplete: true,
					Cause:          err,
				},
			)
		}
	}
	return errors.Join(errs...)
}
func (g *generator) writeTempFile(dir, pattern string, data []byte, mode fs.FileMode) (string, error) {
	tmp, err := g.fs.createTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	path := tmp.Name()
	var primary error
	if _, err := tmp.Write(data); err != nil {
		primary = fmt.Errorf("write temp file: %w", err)
	} else if err := tmp.Chmod(mode); err != nil {
		primary = fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		primary = errors.Join(primary, fmt.Errorf("close temp file: %w", err))
	}
	if primary == nil {
		return path, nil
	}
	if err := g.fs.remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		primary = errors.Join(
			primary,
			&FileRecoveryError{
				Target:         path,
				Backup:         "",
				Action:         "cleanup failed staging file",
				CommitComplete: false,
				Cause:          err,
			},
		)
	}
	return "", primary
}

func (g *generator) reserveTempPath(dir, pattern string) (string, error) {
	tmp, err := g.fs.createTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		return path, err
	}
	if err := g.fs.remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return path, err
	}
	return path, nil
}
