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

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/internal/sandboxfs"
)

const (
	containerWorkspace = "/workspace"
	cleanupTimeout     = 5 * time.Second
	logsTimeout        = 5 * time.Second
)

func classifySetupError(runCtx context.Context, err error, op string) error {
	if runCtx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
		return exectool.ErrTimeout
	}
	if runCtx.Err() != nil {
		return runCtx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
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

	workspace, archive, err := s.materializeWorkspace(req, runtime)
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
		return exectool.RunResult{}, err
	}

	stdout, stderr, err := s.collectContainerLogs(created.ID)
	if err != nil {
		return exectool.RunResult{}, err
	}

	return exectool.RunResult{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: int(exitCode),
		Duration: duration,
	}, nil
}

func (s *Sandbox) materializeWorkspace(
	req exectool.RunRequest,
	runtime Runtime,
) (string, []byte, error) {
	workspace, err := os.MkdirTemp("", "toolsy-docker-*")
	if err != nil {
		return "", nil, fmt.Errorf("%w: create workspace: %w", exectool.ErrSandboxFailure, err)
	}

	canonicalFiles, err := sandboxfs.CanonicalizeFiles(req.Files, runtime.ScriptName)
	if err != nil {
		_ = os.RemoveAll(workspace)
		return "", nil, fmt.Errorf("%w: validate files: %w", exectool.ErrSandboxFailure, err)
	}

	if err = sandboxfs.WriteWorkspace(workspace, canonicalFiles); err != nil {
		_ = os.RemoveAll(workspace)
		return "", nil, fmt.Errorf("%w: materialize files: %w", exectool.ErrSandboxFailure, err)
	}
	if err = sandboxfs.WriteFile(workspace, runtime.ScriptName, []byte(req.Code)); err != nil {
		_ = os.RemoveAll(workspace)
		return "", nil, fmt.Errorf("%w: write script: %w", exectool.ErrSandboxFailure, err)
	}

	archive, err := archiveWorkspace(workspace)
	if err != nil {
		_ = os.RemoveAll(workspace)
		return "", nil, fmt.Errorf("%w: archive workspace: %w", exectool.ErrSandboxFailure, err)
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

func (s *Sandbox) collectContainerLogs(containerID string) (string, string, error) {
	logsCtx, logsCancel := context.WithTimeout(context.Background(), logsTimeout)
	defer logsCancel()

	var logOpts container.LogsOptions
	logOpts.ShowStdout = true
	logOpts.ShowStderr = true

	logs, err := s.client.ContainerLogs(logsCtx, containerID, logOpts)
	if err != nil {
		return "", "", fmt.Errorf("%w: read logs: %w", exectool.ErrSandboxFailure, err)
	}
	defer func() {
		_ = logs.Close()
	}()

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	if _, demuxErr := stdcopy.StdCopy(&outBuf, &errBuf, logs); demuxErr != nil {
		return "", "", fmt.Errorf("%w: demux logs: %w", exectool.ErrSandboxFailure, demuxErr)
	}
	return outBuf.String(), errBuf.String(), nil
}

func archiveWorkspace(root string) ([]byte, error) {
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
		if info.IsDir() {
			return nil
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

		data, err := readArchiveFile(func(name string) (io.ReadCloser, error) {
			return rootFS.Open(name)
		}, name)
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

func readArchiveFile(open func(name string) (io.ReadCloser, error), name string) ([]byte, error) {
	file, err := open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	return io.ReadAll(file)
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
