package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/internal/sandboxfs"
	"github.com/skosovsky/toolsy/textprocessor"
)

const (
	containerWorkspace          = "/workspace"
	cleanupTimeout              = 5 * time.Second
	logsTimeout                 = 5 * time.Second
	defaultMaxArchiveFileBytes  = 64 * 1024 * 1024
	defaultMaxArchiveTotalBytes = 256 * 1024 * 1024
	defaultMaxContainerLogBytes = sandboxfs.DefaultMaxSandboxOutputBytes
)

func classifySetupError(runCtx context.Context, err error, op string) error {
	if runCtx.Err() == context.DeadlineExceeded ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, exectool.ErrTimeout) {
		return exectool.ErrTimeout
	}
	if runCtx.Err() != nil {
		return runCtx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if mapped := toolsy.MapSandboxReadLimitError(err); mapped != nil {
		return mapped
	}
	return fmt.Errorf("%w: %s: %w", exectool.ErrSandboxFailure, op, err)
}

type dockerClient interface {
	ContainerCreate(
		ctx context.Context,
		config *container.Config,
		hostConfig *container.HostConfig,
		networkingConfig *network.NetworkingConfig,
		platform *ocispec.Platform,
		containerName string,
	) (container.CreateResponse, error)
	CopyToContainer(
		ctx context.Context,
		containerID, dstPath string,
		content io.Reader,
		options container.CopyToContainerOptions,
	) error
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerWait(
		ctx context.Context,
		containerID string,
		condition container.WaitCondition,
	) (<-chan container.WaitResponse, <-chan error)
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	ContainerKill(ctx context.Context, containerID, signal string) error
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
}

// Sandbox executes code in ephemeral Docker containers.
type Sandbox struct {
	client          dockerClient
	runtimes        map[string]Runtime
	languages       []string
	networkDisabled bool
	memoryLimit     int64
}

// New creates a Docker-backed sandbox.
func New(opts ...Option) (*Sandbox, error) {
	o := options{
		runtimes:        defaultRuntimes(),
		networkDisabled: false,
		memoryLimit:     0,
		client:          nil,
	}
	for _, opt := range opts {
		opt(&o)
	}

	for language, runtime := range o.runtimes {
		if strings.TrimSpace(runtime.Image) == "" {
			return nil, fmt.Errorf("docker sandbox: runtime %q image must be non-empty", language)
		}
		if len(runtime.Command) == 0 {
			return nil, fmt.Errorf("docker sandbox: runtime %q command must be non-empty", language)
		}
		if strings.TrimSpace(runtime.ScriptName) == "" {
			return nil, fmt.Errorf("docker sandbox: runtime %q script name must be non-empty", language)
		}
	}

	cli := o.client
	if cli == nil {
		var err error
		cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return nil, fmt.Errorf("docker sandbox: create client: %w", err)
		}
	}

	languages := make([]string, 0, len(o.runtimes))
	for language := range o.runtimes {
		languages = append(languages, language)
	}
	sort.Strings(languages)

	return &Sandbox{
		client:          cli,
		runtimes:        o.runtimes,
		languages:       languages,
		networkDisabled: o.networkDisabled,
		memoryLimit:     o.memoryLimit,
	}, nil
}

// SupportedLanguages returns a sorted copy of configured languages.
func (s *Sandbox) SupportedLanguages() []string {
	return append([]string(nil), s.languages...)
}

// Run executes code in an ephemeral Docker container.
func (s *Sandbox) Run(ctx context.Context, req exectool.RunRequest) (exectool.RunResult, error) {
	runtime, ok := s.runtimes[strings.TrimSpace(req.Language)]
	if !ok {
		return exectool.RunResult{}, fmt.Errorf("%w: %s", exectool.ErrUnsupportedLanguage, req.Language)
	}

	workspace, archive, err := s.materializeWorkspace(ctx, req, runtime)
	if err != nil {
		return exectool.RunResult{}, err
	}
	defer func() {
		_ = os.RemoveAll(workspace)
	}()

	var cfg container.Config
	cfg.Image = runtime.Image
	cfg.Cmd = append([]string(nil), runtime.Command...)
	cfg.WorkingDir = containerWorkspace
	cfg.Env = encodeEnv(req.Env)

	created, err := s.client.ContainerCreate(ctx, &cfg, hostConfig(s.networkDisabled, s.memoryLimit), nil, nil, "")
	if err != nil {
		return exectool.RunResult{}, classifySetupError(ctx, err, "create container")
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cleanupCancel()
		var rmOpts container.RemoveOptions
		rmOpts.Force = true
		_ = s.client.ContainerRemove(cleanupCtx, created.ID, rmOpts)
	}()

	var copyOpts container.CopyToContainerOptions
	if err = s.client.CopyToContainer(
		ctx,
		created.ID,
		containerWorkspace,
		bytes.NewReader(archive),
		copyOpts,
	); err != nil {
		return exectool.RunResult{}, classifySetupError(ctx, err, "copy workspace")
	}
	var startOpts container.StartOptions
	if err = s.client.ContainerStart(ctx, created.ID, startOpts); err != nil {
		return exectool.RunResult{}, classifySetupError(ctx, err, "start container")
	}

	exitCode, duration, err := s.waitForContainer(ctx, created.ID)
	if err != nil {
		return exectool.RunResult{}, classifySetupError(ctx, err, "wait container")
	}

	stdoutBuf, stderrBuf, logErr := s.collectContainerLogs(ctx, created.ID)
	return sandboxfs.FinalizeOrInterrupt(ctx, logErr, stdoutBuf, stderrBuf, int(exitCode), duration, true, false)
}

func (s *Sandbox) materializeWorkspace(
	ctx context.Context,
	req exectool.RunRequest,
	runtime Runtime,
) (string, []byte, error) {
	workspace, err := os.MkdirTemp("", "toolsy-docker-*")
	if err != nil {
		return "", nil, classifySetupError(ctx, err, "create workspace")
	}

	canonicalFiles, err := sandboxfs.CanonicalizeFiles(req.Files, runtime.ScriptName)
	if err != nil {
		_ = os.RemoveAll(workspace)
		return "", nil, classifySetupError(ctx, err, "validate files")
	}

	if err = sandboxfs.WriteWorkspace(workspace, canonicalFiles); err != nil {
		_ = os.RemoveAll(workspace)
		return "", nil, classifySetupError(ctx, err, "materialize files")
	}
	if err = sandboxfs.WriteFile(workspace, runtime.ScriptName, []byte(req.Code)); err != nil {
		_ = os.RemoveAll(workspace)
		return "", nil, classifySetupError(ctx, err, "write script")
	}

	archive, err := archiveWorkspace(ctx, workspace)
	if err != nil {
		_ = os.RemoveAll(workspace)
		return "", nil, classifySetupError(ctx, err, "archive workspace")
	}
	return workspace, archive, nil
}

func (s *Sandbox) waitForContainer(
	runCtx context.Context,
	containerID string,
) (int64, time.Duration, error) {
	statusCh, errCh := s.client.ContainerWait(runCtx, containerID, container.WaitConditionNotRunning)
	start := time.Now()
	killContainer := func() {
		killCtx, killCancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer killCancel()
		_ = s.client.ContainerKill(killCtx, containerID, "SIGKILL")
	}

	select {
	case waitErr := <-errCh:
		if waitErr == nil {
			return 0, time.Since(start), nil
		}
		return 0, 0, resolveContainerWaitError(runCtx, waitErr, killContainer)
	case status := <-statusCh:
		if status.Error != nil && status.Error.Message != "" {
			return 0, 0, fmt.Errorf(
				"%w: wait container: %s",
				exectool.ErrSandboxFailure,
				status.Error.Message,
			)
		}
		return status.StatusCode, time.Since(start), nil
	case <-runCtx.Done():
		killContainer()
		if runCtx.Err() == context.DeadlineExceeded {
			return 0, 0, exectool.ErrTimeout
		}
		return 0, 0, runCtx.Err()
	}
}

func resolveContainerWaitError(runCtx context.Context, waitErr error, kill func()) error {
	if runCtx.Err() != nil {
		kill()
		if runCtx.Err() == context.DeadlineExceeded {
			return exectool.ErrTimeout
		}
		return runCtx.Err()
	}
	if errors.Is(waitErr, context.DeadlineExceeded) {
		kill()
		return exectool.ErrTimeout
	}
	if errors.Is(waitErr, context.Canceled) {
		kill()
		return waitErr
	}
	return fmt.Errorf("%w: wait container: %w", exectool.ErrSandboxFailure, waitErr)
}

func (s *Sandbox) collectContainerLogs(
	ctx context.Context,
	containerID string,
) (*sandboxfs.CappedBuffer, *sandboxfs.CappedBuffer, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, nil, ctxErr
	}
	logsCtx, logsCancel := context.WithTimeout(ctx, logsTimeout)
	defer logsCancel()

	var logOpts container.LogsOptions
	logOpts.ShowStdout = true
	logOpts.ShowStderr = true

	logs, err := s.client.ContainerLogs(logsCtx, containerID, logOpts)
	if err != nil {
		return nil, nil, classifySetupError(ctx, err, "read logs")
	}
	defer func() {
		_ = logs.Close()
	}()

	outBuf := sandboxfs.NewCappedBuffer("container stdout", defaultMaxContainerLogBytes)
	errBuf := sandboxfs.NewCappedBuffer("container stderr", defaultMaxContainerLogBytes)
	if _, demuxErr := stdcopy.StdCopy(outBuf, errBuf, logs); demuxErr != nil {
		return outBuf, errBuf, fmt.Errorf("%w: demux logs: %w", exectool.ErrSandboxFailure, demuxErr)
	}
	return outBuf, errBuf, nil
}

//nolint:gocognit // walk callback: ctx, total budget, per-file read
func archiveWorkspace(ctx context.Context, root string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		_ = tw.Close()
		return nil, err
	}
	defer func() {
		_ = rootFS.Close()
	}()

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if walkCtxErr := ctx.Err(); walkCtxErr != nil {
			return walkCtxErr
		}
		if info.IsDir() {
			return nil
		}
		if buf.Len() > defaultMaxArchiveTotalBytes {
			return fmt.Errorf(
				"%w: workspace archive exceeds %d byte limit: %w",
				exectool.ErrSandboxFailure,
				defaultMaxArchiveTotalBytes,
				textprocessor.ErrReadLimitExceeded,
			)
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name, err := sandboxfs.NormalizeRelativePath(rel)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = name
		if werr := tw.WriteHeader(header); werr != nil {
			return werr
		}

		data, err := readArchiveFile(ctx, func(name string) (io.ReadCloser, error) {
			return rootFS.Open(name)
		}, name, info.Size(), defaultMaxArchiveFileBytes)
		if err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		_ = tw.Close()
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readArchiveFile(
	ctx context.Context,
	open func(name string) (io.ReadCloser, error),
	name string,
	fileSize int64,
	maxBytes int,
) ([]byte, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if maxBytes > 0 && fileSize > int64(maxBytes) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf(
			"%w: file %s size %d exceeds %d byte limit: %w",
			exectool.ErrSandboxFailure,
			name, fileSize, maxBytes, textprocessor.ErrReadLimitExceeded,
		)
	}
	file, err := open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	data, err := textprocessor.ReadLimitedBytes(ctx, file, maxBytes)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if toolsy.IsContextInterrupt(err) {
		return nil, err
	}
	if textprocessor.IsReadLimitExceeded(err) {
		return nil, fmt.Errorf(
			"%w: file %s exceeds %d byte limit: %w",
			exectool.ErrSandboxFailure,
			name, maxBytes, textprocessor.ErrReadLimitExceeded,
		)
	}
	return data, err
}

func encodeEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	sort.Strings(out)
	return out
}
