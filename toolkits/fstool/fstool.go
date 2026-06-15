package fstool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

type listArgs struct {
	Path string `json:"path"`
}

type entryInfo struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

type listResult struct {
	Entries []entryInfo `json:"entries"`
}

type readArgs struct {
	Path string `json:"path"`
}

type readResult struct {
	Content string `json:"content"`
}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type statusResult struct {
	Status string `json:"status"`
}

// AsTools returns filesystem tools (list_dir, read_file, and optionally write_file) bound to baseDir.
// baseDir must exist and be a directory. Options customize limits and tool names.
func AsTools(baseDir string, opts ...Option) ([]toolsy.Tool, error) {
	info, err := os.Stat(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("toolkit/fstool: base dir does not exist: %w", err)
		}
		return nil, fmt.Errorf("toolkit/fstool: base dir: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("toolkit/fstool: base dir is not a directory")
	}

	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)

	listTool, err := toolsy.NewTool[listArgs, listResult](
		o.listDirName,
		o.listDirDesc,
		func(ctx context.Context, _ *toolsy.RunEnv, args listArgs) (listResult, error) {
			return doListDir(ctx, baseDir, &o, args.Path)
		},
		toolsy.WithReadOnly(),
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/fstool: build list_dir tool: %w", err)
	}

	readTool, err := toolsy.NewTool[readArgs, readResult](
		o.readFileName,
		o.readFileDesc,
		func(ctx context.Context, _ *toolsy.RunEnv, args readArgs) (readResult, error) {
			return doReadFile(ctx, baseDir, &o, args.Path)
		},
		toolsy.WithReadOnly(),
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/fstool: build read_file tool: %w", err)
	}

	tools := []toolsy.Tool{listTool, readTool}

	if !o.readOnly {
		writeTool, err := toolsy.NewTool[writeArgs, statusResult](
			o.writeFileName,
			o.writeFileDesc,
			func(ctx context.Context, _ *toolsy.RunEnv, args writeArgs) (statusResult, error) {
				return doWriteFile(ctx, baseDir, args.Path, args.Content)
			},
			toolsy.WithDangerous(),
			toolsy.WithRequiresConfirmation(),
		)
		if err != nil {
			return nil, fmt.Errorf("toolkit/fstool: build write_file tool: %w", err)
		}
		tools = append(tools, writeTool)
	}

	return tools, nil
}

func doListDir(ctx context.Context, baseDir string, _ *options, path string) (listResult, error) {
	if ie := toolsy.ToolkitContextError(ctx, "toolkit/fstool: list dir"); ie != nil {
		return listResult{}, ie
	}
	resolved, err := sanitizePath(baseDir, path)
	if err != nil {
		return listResult{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return listResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/fstool: stat: %w", err))
	}
	if !info.IsDir() {
		return listResult{}, toolsy.NewValidationError("path is not a directory")
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return listResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/fstool: read dir: %w", err))
	}
	infos := make([]entryInfo, 0, len(entries))
	for _, e := range entries {
		if ie := toolsy.ToolkitContextError(ctx, "toolkit/fstool: list dir entries"); ie != nil {
			return listResult{}, ie
		}
		ei := entryInfo{Name: e.Name(), IsDir: e.IsDir(), Size: 0}
		if !e.IsDir() {
			if fi, infoErr := e.Info(); infoErr == nil {
				ei.Size = fi.Size()
			}
		}
		infos = append(infos, ei)
	}
	return listResult{Entries: infos}, nil
}

func doReadFile(ctx context.Context, baseDir string, o *options, path string) (readResult, error) {
	if ie := toolsy.ToolkitContextError(ctx, "toolkit/fstool: read file"); ie != nil {
		return readResult{}, ie
	}
	resolved, err := sanitizePath(baseDir, path)
	if err != nil {
		return readResult{}, err
	}
	f, err := os.Open(resolved) // #nosec G304 -- path validated by sanitizePath
	if err != nil {
		return readResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/fstool: open file: %w", err))
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return readResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/fstool: stat file: %w", err))
	}
	if info.IsDir() {
		return readResult{}, toolsy.NewValidationError("path is a directory, not a file")
	}
	contentCap := readContentByteCap(o.maxBytes)
	if info.Size() > int64(contentCap) {
		return readResult{}, toolsy.MapToolkitCapError(ctx, "toolkit/fstool: stat size", contentCap, "file", "")
	}
	content, err := readFileLimited(ctx, f, contentCap)
	if err != nil {
		return readResult{}, err
	}
	return readResult{Content: content}, nil
}

func doWriteFile(ctx context.Context, baseDir, path, content string) (statusResult, error) {
	if ie := toolsy.ToolkitContextError(ctx, "toolkit/fstool: write file"); ie != nil {
		return statusResult{}, ie
	}
	target, err := sanitizePathForWrite(baseDir, path)
	if err != nil {
		return statusResult{}, err
	}
	parent := filepath.Dir(target)
	if mkdirErr := os.MkdirAll(parent, 0o750); mkdirErr != nil {
		return statusResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/fstool: mkdir: %w", mkdirErr))
	}
	// Post-creation symlink check: resolve parent and ensure still under sandbox
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return statusResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/fstool: resolve parent: %w", err))
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return statusResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/fstool: base dir: %w", err))
	}
	baseCanon, err := filepath.EvalSymlinks(baseAbs)
	if err != nil {
		return statusResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/fstool: base dir: %w", err))
	}
	if uerr := pathUnderBase(baseCanon, resolvedParent); uerr != nil {
		return statusResult{}, uerr
	}
	finalPath := filepath.Join(resolvedParent, filepath.Base(target))
	// If target already exists and is a symlink, ensure it does not point outside sandbox
	if symErr := checkWriteTargetSymlinkWithinSandbox(baseCanon, finalPath); symErr != nil {
		return statusResult{}, symErr
	}
	if writeErr := os.WriteFile(finalPath, []byte(content), 0o600); writeErr != nil {
		return statusResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/fstool: write file: %w", writeErr))
	}
	return statusResult{Status: "Success"}, nil
}

// checkWriteTargetSymlinkWithinSandbox returns nil if finalPath is absent, not a symlink, or resolves inside baseCanon.
func checkWriteTargetSymlinkWithinSandbox(baseCanon, finalPath string) error {
	fi, statErr := os.Lstat(finalPath)
	if statErr != nil {
		// Match prior behavior: symlink check only runs when Lstat succeeds (path exists).
		return nil //nolint:nilerr // intentional: ignore Lstat errors (not found, permission, etc.)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	linkDest, err := os.Readlink(finalPath)
	if err != nil {
		return toolsy.NewInternalError(fmt.Errorf("toolkit/fstool: read target symlink: %w", err))
	}
	checkPath := linkDest
	if !filepath.IsAbs(checkPath) {
		checkPath = filepath.Join(filepath.Dir(finalPath), checkPath)
	}
	checkPath = filepath.Clean(checkPath)
	if resolved, evalErr := filepath.EvalSymlinks(finalPath); evalErr == nil {
		checkPath = resolved
	}
	return pathUnderBase(baseCanon, checkPath)
}

// readFileLimited reads at most maxBytes from r (fail-closed); respects ctx cancellation.
func readFileLimited(ctx context.Context, r io.Reader, maxBytes int) (string, error) {
	data, err := textprocessor.ReadLimitedBytes(ctx, r, maxBytes)
	if mapped := toolsy.MapToolkitReadError(ctx, err, "toolkit/fstool: read", maxBytes, "file", ""); mapped != nil {
		return "", mapped
	}
	if err != nil {
		return "", toolsy.NewInternalError(fmt.Errorf("toolkit/fstool: read: %w", err))
	}
	return string(data), nil
}
