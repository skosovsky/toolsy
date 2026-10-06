package fstool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

type listArgs struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type entryInfo struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

type listResult struct {
	Path       string      `json:"path"`
	Entries    []entryInfo `json:"entries"`
	NextOffset int64       `json:"next_offset"`
	HasMore    bool        `json:"has_more"`
}

type readArgs struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset,omitempty"`
	Length int    `json:"length,omitempty"`
}

type readResult struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	NextOffset int64  `json:"next_offset"`
	HasMore    bool   `json:"has_more"`
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
	if o.maxBytes < 0 || o.maxSourceBytes < 0 || int64(o.maxSourceBytes) == math.MaxInt64 || o.maxEntries < 0 ||
		o.maxScanEntries < 0 ||
		o.maxNameBytes < 0 ||
		o.maxEntries >= o.maxScanEntries {
		return nil, errors.New("toolkit/fstool: invalid limits")
	}
	baseDir, err = filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}

	listTool, err := toolsy.NewTool[listArgs, listResult](
		o.listDirName,
		o.listDirDesc,
		func(ctx context.Context, _ *toolsy.RunEnv, args listArgs) (listResult, error) {
			return listDirectory(ctx, baseDir, &o, args)
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
			return readFile(ctx, baseDir, &o, args)
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
				if len(args.Content) > o.maxSourceBytes {
					return statusResult{}, toolsy.NewValidationError("write content exceeds source limit")
				}
				result := statusResult{Status: "Success"}
				if err := checkWire(result, o.maxBytes); err != nil {
					return statusResult{}, err
				}
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

func checkWire(value any, limit int) error {
	data, err := json.Marshal(value)
	if err != nil {
		return toolsy.NewInternalError(err)
	}
	if len(data) > limit {
		return toolsy.NewValidationError(fmt.Sprintf("JSON result exceeds %d bytes", limit))
	}
	return nil
}

func openRoot(ctx context.Context, baseDir, path string) (*os.Root, string, error) {
	if err := toolsy.ToolkitContextError(ctx, "toolkit/fstool"); err != nil {
		return nil, "", err
	}
	name, err := relativePath(path)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return nil, "", toolsy.NewInternalError(err)
	}
	return root, name, nil
}

func doListDir(ctx context.Context, baseDir string, o *options, path string) (listResult, error) {
	return listDirectory(ctx, baseDir, o, listArgs{Path: path, Offset: 0, Limit: 0})
}

const (
	directoryScanBatch = 128
	directoryMode      = 0o750
	fileMode           = 0o600
)

func listDirectory(ctx context.Context, baseDir string, o *options, args listArgs) (listResult, error) {
	if err := toolsy.ToolkitContextError(ctx, "toolkit/fstool: list"); err != nil {
		return listResult{}, err
	}
	if args.Offset < 0 || args.Offset >= int64(o.maxScanEntries) || args.Limit < 0 || args.Limit > o.maxEntries {
		return listResult{}, toolsy.NewValidationError("invalid directory range")
	}
	limit := args.Limit
	if limit == 0 {
		limit = o.maxEntries
	}
	if args.Offset > int64(o.maxScanEntries)-int64(limit)-1 {
		return listResult{}, toolsy.NewValidationError("directory range exceeds scan limit")
	}
	root, name, err := openRoot(ctx, baseDir, args.Path)
	if err != nil {
		return listResult{}, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.OpenFile(name, os.O_RDONLY|nonblockFlag, 0)
	if err != nil {
		return listResult{}, toolsy.NewValidationError("directory inaccessible: " + err.Error())
	}
	defer func() { _ = f.Close() }()
	stat, err := f.Stat()
	if err != nil {
		return listResult{}, toolsy.NewInternalError(err)
	}
	if !stat.IsDir() {
		return listResult{}, toolsy.NewValidationError("path is not a directory")
	}
	if err = skipDirectory(ctx, f, args.Offset); err != nil {
		return listResult{}, err
	}
	entries, err := f.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return listResult{}, toolsy.NewInternalError(err)
	}
	more := len(entries) > limit
	entries = entries[:min(len(entries), limit)]
	result := listResult{
		Path:       args.Path,
		Entries:    make([]entryInfo, 0, len(entries)),
		HasMore:    more,
		NextOffset: args.Offset + int64(len(entries)),
	}
	for _, entry := range entries {
		if err = toolsy.ToolkitContextError(ctx, "toolkit/fstool: list"); err != nil {
			return listResult{}, err
		}
		if len(entry.Name()) > o.maxNameBytes || !utf8.ValidString(entry.Name()) {
			return listResult{}, toolsy.NewValidationError("entry name exceeds limit or is not UTF-8")
		}
		info, infoErr := entry.Info() // os.Root ReadDir pins fd-relative metadata before constructing DirEntry.
		if infoErr != nil {
			return listResult{}, toolsy.NewInternalError(infoErr)
		}
		result.Entries = append(result.Entries, entryInfo{Name: entry.Name(), IsDir: entry.IsDir(), Size: info.Size()})
	}
	return result, checkWire(result, o.maxBytes)
}

func doReadFile(ctx context.Context, baseDir string, o *options, path string) (readResult, error) {
	return readFile(ctx, baseDir, o, readArgs{Path: path, Offset: 0, Length: 0})
}

func readFile(ctx context.Context, baseDir string, o *options, args readArgs) (readResult, error) {
	if err := toolsy.ToolkitContextError(ctx, "toolkit/fstool: read"); err != nil {
		return readResult{}, err
	}
	if args.Offset < 0 || args.Length < 0 || args.Length > o.maxSourceBytes {
		return readResult{}, toolsy.NewValidationError("invalid byte range")
	}
	limit := args.Length
	if limit == 0 {
		limit = o.maxSourceBytes
	}
	root, name, err := openRoot(ctx, baseDir, args.Path)
	if err != nil {
		return readResult{}, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.OpenFile(name, os.O_RDONLY|nonblockFlag, 0)
	if err != nil {
		return readResult{}, toolsy.NewValidationError("file inaccessible: " + err.Error())
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return readResult{}, toolsy.NewInternalError(err)
	}
	if !info.Mode().IsRegular() {
		return readResult{}, toolsy.NewValidationError("path is not a regular file")
	}
	if args.Offset > info.Size() {
		return readResult{}, toolsy.NewValidationError("offset exceeds file size")
	}
	if args.Length == 0 && info.Size()-args.Offset > int64(limit) {
		return readResult{}, toolsy.NewValidationError("file exceeds source limit; specify a byte range")
	}
	if _, err = f.Seek(args.Offset, io.SeekStart); err != nil {
		return readResult{}, toolsy.NewInternalError(err)
	}
	reader := io.Reader(f)
	if args.Length > 0 {
		reader = io.LimitReader(f, int64(limit))
	}
	content, err := readFileLimited(ctx, reader, limit)
	if err != nil {
		return readResult{}, err
	}
	if !utf8.ValidString(content) {
		return readResult{}, toolsy.NewValidationError("byte range is not valid UTF-8")
	}
	next := args.Offset + int64(len(content))
	result := readResult{Path: args.Path, Content: content, NextOffset: next, HasMore: next < info.Size()}
	return result, checkWire(result, o.maxBytes)
}

func doWriteFile(ctx context.Context, baseDir, path, content string) (statusResult, error) {
	root, name, err := openRoot(ctx, baseDir, path)
	if err != nil {
		return statusResult{}, err
	}
	defer func() { _ = root.Close() }()
	if err = root.MkdirAll(filepath.Dir(name), directoryMode); err != nil {
		return statusResult{}, toolsy.NewValidationError("parent inaccessible: " + err.Error())
	}
	if err = toolsy.ToolkitContextError(ctx, "toolkit/fstool: write"); err != nil {
		return statusResult{}, err
	}
	// Open without truncation: reject special files before changing content.
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|nonblockFlag, fileMode)
	if err != nil {
		return statusResult{}, toolsy.NewValidationError("file inaccessible: " + err.Error())
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return statusResult{}, toolsy.NewInternalError(err)
	}
	if !info.Mode().IsRegular() {
		return statusResult{}, toolsy.NewValidationError("path is not a regular file")
	}
	if err = f.Truncate(0); err != nil {
		return statusResult{}, toolsy.NewInternalError(err)
	}
	written, err := io.WriteString(f, content)
	if err == nil && written != len(content) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return statusResult{}, toolsy.NewInternalError(err)
	}
	return statusResult{Status: "Success"}, nil
}

// readFileLimited reads at most maxBytes and fails closed on overflow.
func readFileLimited(ctx context.Context, r io.Reader, maxBytes int) (string, error) {
	data, err := textprocessor.ReadLimitedBytes(ctx, r, maxBytes)
	if mapped := toolsy.MapToolkitReadError(ctx, err, "toolkit/fstool: read", maxBytes, "file", ""); mapped != nil {
		return "", mapped
	}
	if err != nil {
		return "", toolsy.NewInternalError(err)
	}
	return string(data), nil
}

func skipDirectory(ctx context.Context, f *os.File, offset int64) error {
	var err error
	for skipped := int64(0); skipped < offset; {
		if err = toolsy.ToolkitContextError(ctx, "toolkit/fstool: list"); err != nil {
			return err
		}
		n := min(int64(directoryScanBatch), offset-skipped)
		batch, readErr := f.ReadDir(int(n))
		skipped += int64(len(batch))
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return toolsy.NewInternalError(readErr)
		}
		if len(batch) < int(n) {
			return toolsy.NewValidationError("offset exceeds directory size")
		}
	}
	return nil
}
