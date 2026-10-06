package docker

import (
	"context"
	"encoding/binary"
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
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/internal/sandboxfs"
)

const (
	containerWorkspace          = "/workspace"
	cleanupTimeout              = 5 * time.Second
	logsTimeout                 = 5 * time.Second
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
	Info(context.Context) (system.Info, error)
	ContainerCreate(
		ctx context.Context,
		config *container.Config,
		hostConfig *container.HostConfig,
		networkingConfig *network.NetworkingConfig,
		platform *ocispec.Platform,
		containerName string,
	) (container.CreateResponse, error)
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
	client        dockerClient
	runtimes      map[string]Runtime
	languages     []string
	policy        Policy
	workspaceRoot string
}

// New creates a Docker-backed sandbox.
func New(opts ...Option) (*Sandbox, error) {
	o := options{
		runtimes:      defaultRuntimes(),
		policy:        DefaultPolicy(),
		client:        nil,
		workspaceRoot: "",
	}
	for _, opt := range opts {
		opt(&o)
	}

	if o.policy.MemoryBytes <= 0 || o.policy.CPUQuota <= 0 || o.policy.PIDs <= 0 || o.policy.TmpBytes <= 0 ||
		o.policy.OutputBytes <= 0 ||
		o.policy.InputBytes <= 0 || o.policy.MaxFiles <= 0 || o.policy.LogTimeout <= 0 {
		return nil, errors.New("docker sandbox: all policy bounds must be positive")
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
		if !strings.HasPrefix(localDaemonHost(cli), "unix://") {
			return nil, errors.New("docker sandbox: local unix daemon required for workspace bind")
		}
	}

	languages := make([]string, 0, len(o.runtimes))
	for language := range o.runtimes {
		languages = append(languages, language)
	}
	sort.Strings(languages)

	return &Sandbox{
		client:        cli,
		runtimes:      o.runtimes,
		languages:     languages,
		policy:        o.policy,
		workspaceRoot: o.workspaceRoot,
	}, nil
}

// SupportedLanguages returns a sorted copy of configured languages.
func (s *Sandbox) SupportedLanguages() []string {
	return append([]string(nil), s.languages...)
}

// Run executes code in an ephemeral Docker container.
//
//nolint:nonamedreturns // Deferred cleanup appends diagnostics while retaining the primary result.
func (s *Sandbox) Run(ctx context.Context, req exectool.RunRequest) (result exectool.RunResult, runErr error) {
	runtime, ok := s.runtimes[strings.TrimSpace(req.Language)]
	if !ok {
		return exectool.RunResult{}, fmt.Errorf("%w: %s", exectool.ErrUnsupportedLanguage, req.Language)
	}

	info, err := s.client.Info(ctx)
	if err != nil {
		return result, classifySetupError(ctx, err, "daemon capabilities")
	}
	if info.OSType != "linux" || !daemonSeccomp(info.SecurityOptions) || !info.MemoryLimit || !info.SwapLimit ||
		!info.CPUCfsPeriod ||
		!info.CPUCfsQuota ||
		!info.PidsLimit {
		return result, fmt.Errorf("%w: daemon cannot enforce mandatory cgroup policy", exectool.ErrSandboxFailure)
	}
	workspace, err := s.materializeWorkspace(ctx, req, runtime)
	if err != nil {
		return exectool.RunResult{}, err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		runErr = sandboxfs.WithCleanupError(
			runErr,
			"docker",
			"remove workspace",
			sandboxfs.RemoveWorkspace(cleanupCtx, workspace),
		)
	}()

	var cfg container.Config
	cfg.Image = runtime.Image
	cfg.Cmd = append([]string(nil), runtime.Command...)
	cfg.WorkingDir = containerWorkspace
	cfg.Env = append(encodeEnv(req.Env), "PYTHONDONTWRITEBYTECODE=1", "HOME=/tmp")
	cfg.User = "65534:65534"
	cfg.Entrypoint = []string{runtime.Command[0]}
	cfg.Cmd = append([]string(nil), runtime.Command[1:]...)

	created, err := s.client.ContainerCreate(ctx, &cfg, hostConfig(s.policy, workspace), nil, nil, "")
	if err != nil {
		return exectool.RunResult{}, classifySetupError(ctx, err, "create container")
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cleanupCancel()
		var rmOpts container.RemoveOptions
		rmOpts.Force = true
		runErr = sandboxfs.WithCleanupError(
			runErr,
			"docker",
			"remove container",
			s.client.ContainerRemove(cleanupCtx, created.ID, rmOpts),
		)
	}()

	var startOpts container.StartOptions
	if err = s.client.ContainerStart(ctx, created.ID, startOpts); err != nil {
		return exectool.RunResult{}, classifySetupError(ctx, err, "start container")
	}

	return s.collectExecution(ctx, created.ID)
}

func (s *Sandbox) collectExecution(ctx context.Context, containerID string) (exectool.RunResult, error) {
	executionCtx, cancelExecution := context.WithCancel(ctx)
	defer cancelExecution()
	type completion struct {
		code     int64
		duration time.Duration
		err      error
	}
	finished := make(chan completion, 1)
	go func() {
		code, duration, waitErr := s.waitForContainer(executionCtx, containerID)
		if waitErr != nil {
			cancelExecution()
		}
		finished <- completion{code, duration, waitErr}
	}()
	stdoutBuf, stderrBuf, logErr := s.collectContainerLogs(executionCtx, containerID)
	if logErr != nil {
		cancelExecution()
	}
	completed := <-finished
	if ctx.Err() != nil {
		return sandboxfs.FinalizeOrInterrupt(
			ctx,
			ctx.Err(),
			stdoutBuf,
			stderrBuf,
			int(completed.code),
			completed.duration,
			false,
		)
	}
	if completed.err != nil && !toolsy.IsContextInterrupt(completed.err) {
		return sandboxfs.FinalizeOrInterrupt(
			ctx,
			completed.err,
			stdoutBuf,
			stderrBuf,
			int(completed.code),
			completed.duration,
			false,
		)
	}
	if logErr != nil {
		return sandboxfs.FinalizeOrInterrupt(
			ctx,
			logErr,
			stdoutBuf,
			stderrBuf,
			int(completed.code),
			completed.duration,
			false,
		)
	}
	return sandboxfs.FinalizeOrInterrupt(
		ctx,
		completed.err,
		stdoutBuf,
		stderrBuf,
		int(completed.code),
		completed.duration,
		false,
	)
}

//nolint:nonamedreturns // Deferred cleanup preserves failures while removing partially prepared input.
func (s *Sandbox) materializeWorkspace(
	ctx context.Context,
	req exectool.RunRequest,
	runtime Runtime,
) (path string, runErr error) {
	if err := s.validateInput(req); err != nil {
		return "", err
	}
	workspace, err := os.MkdirTemp(s.workspaceRoot, "toolsy-docker-*")
	if err != nil {
		return "", classifySetupError(ctx, err, "create workspace")
	}

	defer func() {
		if runErr != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cancel()
			runErr = sandboxfs.WithCleanupError(
				runErr,
				"docker",
				"remove failed workspace",
				sandboxfs.RemoveWorkspace(cleanupCtx, workspace),
			)
		}
	}()
	canonicalFiles, err := sandboxfs.CanonicalizeFiles(req.Files, runtime.ScriptName)
	if err != nil {
		return "", classifySetupError(ctx, err, "validate files")
	}

	if err = sandboxfs.WriteWorkspace(workspace, canonicalFiles); err != nil {
		return "", classifySetupError(ctx, err, "materialize files")
	}
	if err = sandboxfs.WriteFile(workspace, runtime.ScriptName, []byte(req.Code)); err != nil {
		return "", classifySetupError(ctx, err, "write script")
	}

	if ctx.Err() != nil {
		return "", classifySetupError(ctx, ctx.Err(), "prepare workspace")
	}
	rootFS, err := os.OpenRoot(workspace)
	if err != nil {
		return "", err
	}
	defer func() { runErr = sandboxfs.WithCleanupError(runErr, "docker", "close workspace root", rootFS.Close()) }()
	if err := filepath.Walk(workspace, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		const readableFileMode = 0o444
		mode := os.FileMode(readableFileMode)
		if info.IsDir() {
			mode = 0o755
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		return rootFS.Chmod(rel, mode)
	}); err != nil {
		return "", err
	}
	return workspace, nil
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
	case waitErr, open := <-errCh:
		if !open || waitErr == nil {
			return 0, 0, fmt.Errorf("%w: missing wait response", exectool.ErrSandboxFailure)
		}
		return 0, 0, resolveContainerWaitError(runCtx, waitErr, killContainer)
	case status, open := <-statusCh:
		if !open {
			return 0, 0, fmt.Errorf("%w: missing container status", exectool.ErrSandboxFailure)
		}
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
		return nil, nil, collectionFailure(ctxErr)
	}
	limit := s.policy.LogTimeout
	if limit <= 0 {
		limit = logsTimeout
	}
	logsCtx, logsCancel := context.WithTimeout(ctx, limit)
	defer logsCancel()

	var logOpts container.LogsOptions
	logOpts.ShowStdout = true
	logOpts.ShowStderr = true
	logOpts.Follow = true

	logs, err := s.client.ContainerLogs(logsCtx, containerID, logOpts)
	if err != nil {
		return nil, nil, collectionFailure(err)
	}
	defer func() {
		_ = logs.Close()
	}()

	outBuf := sandboxfs.NewCappedBuffer("container stdout", s.outputLimit())
	errBuf := sandboxfs.NewCappedBuffer("container stderr", s.outputLimit())
	stopClose := context.AfterFunc(logsCtx, func() { _ = logs.Close() })
	defer stopClose()
	if demuxErr := demuxLogs(logs, outBuf, errBuf); demuxErr != nil {
		if logsCtx.Err() != nil {
			return outBuf, errBuf, collectionFailure(logsCtx.Err())
		}
		return outBuf, errBuf, fmt.Errorf("%w: demux logs: %w", exectool.ErrSandboxFailure, demuxErr)
	}
	if err := logsCtx.Err(); err != nil {
		return outBuf, errBuf, collectionFailure(err)
	}
	return outBuf, errBuf, nil
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

func (s *Sandbox) outputLimit() int {
	if s.policy.OutputBytes > 0 {
		return s.policy.OutputBytes
	}
	return defaultMaxContainerLogBytes
}

// demuxLogs rejects partial headers/payloads: Docker's StdCopy treats a short final header as EOF.
func demuxLogs(src io.Reader, stdout, stderr io.Writer) error {
	var header [8]byte
	for {
		n, err := io.ReadFull(src, header[:])
		if err == io.EOF && n == 0 {
			return nil
		}
		if err != nil {
			return fmt.Errorf("incomplete log frame: %w", err)
		}
		if header[1] != 0 || header[2] != 0 || header[3] != 0 {
			return errors.New("invalid log frame padding")
		}
		var dst io.Writer
		switch header[0] {
		case 1:
			dst = stdout
		case 2:
			dst = stderr
		default:
			return fmt.Errorf("invalid log stream %d", header[0])
		}
		size := int64(binary.BigEndian.Uint32(header[4:]))
		if _, err := io.CopyN(dst, src, size); err != nil {
			return fmt.Errorf("incomplete log payload: %w", err)
		}
	}
}

func daemonSeccomp(options []string) bool {
	for _, option := range options {
		if strings.HasPrefix(option, "name=seccomp") {
			return true
		}
	}
	return false
}

func collectionFailure(err error) error {
	if toolsy.IsContextInterrupt(err) {
		return fmt.Errorf("%w: log collection interrupted: %v", exectool.ErrSandboxFailure, err.Error())
	}
	return fmt.Errorf("%w: log collection: %w", exectool.ErrSandboxFailure, err)
}

func localDaemonHost(cli dockerClient) string {
	native, ok := cli.(*client.Client)
	if !ok {
		return ""
	}
	return native.DaemonHost()
}

func (s *Sandbox) validateInput(req exectool.RunRequest) error {
	if len(req.Files) > s.policy.MaxFiles {
		return fmt.Errorf("%w: workspace file count limit", exectool.ErrSandboxFailure)
	}
	total := len(req.Code)
	for name, data := range req.Files {
		if len(name) > s.policy.InputBytes-total {
			return fmt.Errorf("%w: workspace input limit", exectool.ErrSandboxFailure)
		}
		total += len(name)
		if len(data) > s.policy.InputBytes-total {
			return fmt.Errorf("%w: workspace input limit", exectool.ErrSandboxFailure)
		}
		total += len(data)
	}
	if total > s.policy.InputBytes {
		return fmt.Errorf("%w: workspace input limit", exectool.ErrSandboxFailure)
	}
	return nil
}
