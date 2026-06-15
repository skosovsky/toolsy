package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/textprocessor"
)

type mockClient struct {
	createdConfig     *container.Config
	createdHostConfig *container.HostConfig
	copiedToContainer bool
	started           bool
	killed            bool
	removed           bool
	removeCtxErr      error
	removeDeadlineIn  time.Duration
	waitResponse      container.WaitResponse
	waitErr           error
	logs              []byte
}

func (m *mockClient) ContainerCreate(
	_ context.Context,
	config *container.Config,
	hostConfig *container.HostConfig,
	_ *network.NetworkingConfig,
	_ *ocispec.Platform,
	_ string,
) (container.CreateResponse, error) {
	m.createdConfig = config
	m.createdHostConfig = hostConfig
	return container.CreateResponse{ID: "abc123"}, nil
}

func (m *mockClient) CopyToContainer(
	_ context.Context,
	_ string,
	_ string,
	_ io.Reader,
	_ container.CopyToContainerOptions,
) error {
	m.copiedToContainer = true
	return nil
}

func (m *mockClient) ContainerStart(_ context.Context, _ string, _ container.StartOptions) error {
	m.started = true
	return nil
}

func (m *mockClient) ContainerWait(
	_ context.Context,
	_ string,
	_ container.WaitCondition,
) (<-chan container.WaitResponse, <-chan error) {
	statusCh := make(chan container.WaitResponse, 1)
	errCh := make(chan error, 1)
	if m.waitErr != nil {
		errCh <- m.waitErr
	} else {
		statusCh <- m.waitResponse
	}
	return statusCh, errCh
}

func (m *mockClient) ContainerLogs(_ context.Context, _ string, _ container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m.logs)), nil
}

func (m *mockClient) ContainerKill(_ context.Context, _ string, _ string) error {
	m.killed = true
	return nil
}

func (m *mockClient) ContainerRemove(ctx context.Context, _ string, _ container.RemoveOptions) error {
	m.removed = true
	m.removeCtxErr = ctx.Err()
	if deadline, ok := ctx.Deadline(); ok {
		m.removeDeadlineIn = time.Until(deadline)
	}
	return nil
}

func TestRunSuccess(t *testing.T) {
	client := &mockClient{
		waitResponse: container.WaitResponse{StatusCode: 0},
		logs:         muxLogs("hello", ""),
	}
	sb, err := New(WithClient(client))
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, "hello", res.Stdout)
	require.True(t, client.copiedToContainer)
	require.True(t, client.started)
	require.True(t, client.removed)
}

func TestRunReturnsNonZeroExitAsResult(t *testing.T) {
	client := &mockClient{
		waitResponse: container.WaitResponse{StatusCode: 3},
		logs:         muxLogs("", "boom"),
	}
	sb, err := New(WithClient(client))
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "bash",
		Code:     "exit 3",
	})
	require.NoError(t, err)
	require.Equal(t, 3, res.ExitCode)
	require.Equal(t, "boom", res.Stderr)
}

func TestRunRejectsUnsupportedLanguage(t *testing.T) {
	sb, err := New(WithClient(&mockClient{}))
	require.NoError(t, err)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "ruby",
		Code:     "puts 1",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrUnsupportedLanguage)
}

func TestRunRejectsReservedScriptNames(t *testing.T) {
	client := &mockClient{}
	sb, err := New(WithClient(client))
	require.NoError(t, err)

	testCases := []string{
		"main.py",
		"dir/../main.py",
	}

	for _, name := range testCases {
		t.Run(name, func(t *testing.T) {
			client.createdConfig = nil
			client.copiedToContainer = false

			_, err := sb.Run(context.Background(), exectool.RunRequest{
				Language: "python",
				Code:     "print(1)",
				Files:    map[string][]byte{name: []byte("collision")},
			})
			require.Error(t, err)
			require.ErrorIs(t, err, exectool.ErrSandboxFailure)
			require.Nil(t, client.createdConfig)
			require.False(t, client.copiedToContainer)
		})
	}
}

func TestRunKillsContainerOnTimeout(t *testing.T) {
	client := &timeoutClient{}
	sb, err := New(WithClient(client))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = sb.Run(ctx, exectool.RunRequest{
		Language: "python",
		Code:     "while True: pass",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)
	require.True(t, client.killed)
	require.True(t, client.removed)
}

func TestRunMapsErrChTimeoutToErrTimeout(t *testing.T) {
	client := &timeoutErrClient{}
	sb, err := New(WithClient(client))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = sb.Run(ctx, exectool.RunRequest{
		Language: "python",
		Code:     "while True: pass",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)
	require.True(t, client.killed)
	require.True(t, client.removed)
}

func TestRunReturnsTimeoutDuringSetup(t *testing.T) {
	testCases := []struct {
		name   string
		stage  string
		viaCtx bool
	}{
		{name: "create direct deadline", stage: "create", viaCtx: false},
		{name: "create expired ctx", stage: "create", viaCtx: true},
		{name: "copy direct deadline", stage: "copy", viaCtx: false},
		{name: "copy expired ctx", stage: "copy", viaCtx: true},
		{name: "start direct deadline", stage: "start", viaCtx: false},
		{name: "start expired ctx", stage: "start", viaCtx: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &setupTimeoutClient{stage: tc.stage, viaCtx: tc.viaCtx}
			sb, err := New(WithClient(client))
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			_, err = sb.Run(ctx, exectool.RunRequest{
				Language: "python",
				Code:     "print(1)",
			})
			require.Error(t, err)
			require.ErrorIs(t, err, exectool.ErrTimeout)
			if tc.stage != "create" {
				require.True(t, client.removed)
			}
		})
	}
}

func TestRunCreatesCleanupTimeoutAtRemoveTime(t *testing.T) {
	client := &delayedWaitClient{
		mockClient: mockClient{
			waitResponse: container.WaitResponse{StatusCode: 0},
			logs:         muxLogs("hello", ""),
		},
		delay: 60 * time.Millisecond,
	}
	sb, err := New(WithClient(client))
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
	})
	require.NoError(t, err)
	require.Equal(t, "hello", res.Stdout)
	require.True(t, client.removed)
	require.NoError(t, client.removeCtxErr)
	// Remove uses cleanupTimeout (5s): deadline must comfortably exceed the wait delay.
	require.Greater(t, client.removeDeadlineIn, 80*time.Millisecond)
}

func TestRunCollectsLogsAfterContextExpires(t *testing.T) {
	client := &delayedLogsClient{
		mockClient: mockClient{
			waitResponse: container.WaitResponse{StatusCode: 0},
			logs:         muxLogs("done", ""),
		},
		delay: 30 * time.Millisecond,
	}
	sb, err := New(WithClient(client))
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, "done", res.Stdout)
}

func TestRunDurationExcludesLogCollection(t *testing.T) {
	client := &durationClient{
		mockClient: mockClient{
			waitResponse: container.WaitResponse{StatusCode: 0},
			logs:         muxLogs("done", ""),
		},
		waitDelay: 30 * time.Millisecond,
		logsDelay: 60 * time.Millisecond,
	}
	sb, err := New(WithClient(client))
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
	})
	require.NoError(t, err)
	require.Greater(t, res.Duration, 20*time.Millisecond)
	require.Less(t, res.Duration, 70*time.Millisecond)
}

func TestNewAppliesImageMappingAndResourceOptions(t *testing.T) {
	client := &mockClient{
		waitResponse: container.WaitResponse{StatusCode: 0},
		logs:         muxLogs("", ""),
	}
	sb, err := New(
		WithClient(client),
		WithImageMapping(map[string]string{"python": "python:3.12-alpine"}),
		WithNetworkDisabled(),
		WithMemoryLimit(256*1024*1024),
	)
	require.NoError(t, err)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
	})
	require.NoError(t, err)
	require.Equal(t, "python:3.12-alpine", client.createdConfig.Image)
	require.Equal(t, container.NetworkMode("none"), client.createdHostConfig.NetworkMode)
	require.EqualValues(t, 256*1024*1024, client.createdHostConfig.Memory)
}

func TestReadArchiveFileClosesFileBeforeReturn(t *testing.T) {
	rc := &trackingReadCloser{Reader: bytes.NewReader([]byte("payload"))}

	data, err := readArchiveFile(context.Background(), func(name string) (io.ReadCloser, error) {
		require.Equal(t, "data.txt", name)
		return rc, nil
	}, "data.txt", 7, defaultMaxArchiveFileBytes)
	require.NoError(t, err)
	require.Equal(t, []byte("payload"), data)
	require.Equal(t, 1, rc.closeCalls)
}

func TestReadArchiveFileClosesFileOnReadError(t *testing.T) {
	rc := &trackingReadCloser{
		Reader: io.MultiReader(
			bytes.NewReader([]byte("payload")),
			errorReader{err: errors.New("boom")},
		),
	}

	_, err := readArchiveFile(context.Background(), func(string) (io.ReadCloser, error) {
		return rc, nil
	}, "data.txt", 7, defaultMaxArchiveFileBytes)
	require.Error(t, err)
	require.Equal(t, 1, rc.closeCalls)
}

func TestReadArchiveFile_ExceedsSizeLimit(t *testing.T) {
	rc := &trackingReadCloser{Reader: bytes.NewReader([]byte("payload"))}
	const maxBytes = 4

	_, err := readArchiveFile(context.Background(), func(string) (io.ReadCloser, error) {
		return rc, nil
	}, "big.bin", 100, maxBytes)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.Contains(t, err.Error(), "exceeds 4 byte limit")
	require.Equal(t, 0, rc.closeCalls)
}

func TestReadArchiveFile_ReadExceedsLimit(t *testing.T) {
	rc := &trackingReadCloser{Reader: bytes.NewReader(make([]byte, 20))}
	const maxBytes = 10

	_, err := readArchiveFile(context.Background(), func(string) (io.ReadCloser, error) {
		return rc, nil
	}, "big.bin", 7, maxBytes)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.Contains(t, err.Error(), "exceeds 10 byte limit")
	require.Equal(t, 1, rc.closeCalls)
}

func TestClassifySetupError_ArchiveReadLimitMapsValidation(t *testing.T) {
	capErr := fmt.Errorf(
		"%w: file data.txt exceeds %d byte limit: %w",
		exectool.ErrSandboxFailure,
		4,
		textprocessor.ErrReadLimitExceeded,
	)
	err := classifySetupError(context.Background(), capErr, "archive workspace")
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "data.txt")
}

func TestClassifySetupError_TimeoutOverReadLimit(t *testing.T) {
	t.Parallel()
	err := classifySetupError(
		context.Background(),
		fmt.Errorf("archive: %w", exectool.ErrTimeout),
		"archive workspace",
	)
	require.ErrorIs(t, err, exectool.ErrTimeout)
}

func TestClassifySetupError_CancelOverReadLimit(t *testing.T) {
	t.Parallel()
	capErr := fmt.Errorf(
		"%w: file data.txt exceeds %d byte limit: %w",
		exectool.ErrSandboxFailure,
		4,
		textprocessor.ErrReadLimitExceeded,
	)
	wrapped := fmt.Errorf("archive: %w", context.Canceled)
	inner := fmt.Errorf("setup: %w", errors.Join(wrapped, capErr))
	err := classifySetupError(context.Background(), inner, "archive workspace")
	require.ErrorIs(t, err, context.Canceled)
}

type timeoutClient struct {
	mockClient
}

func (m *timeoutClient) ContainerWait(
	ctx context.Context,
	_ string,
	_ container.WaitCondition,
) (<-chan container.WaitResponse, <-chan error) {
	statusCh := make(chan container.WaitResponse)
	errCh := make(chan error)
	go func() {
		<-ctx.Done()
	}()
	return statusCh, errCh
}

type timeoutErrClient struct {
	mockClient
}

func (m *timeoutErrClient) ContainerWait(
	ctx context.Context,
	_ string,
	_ container.WaitCondition,
) (<-chan container.WaitResponse, <-chan error) {
	statusCh := make(chan container.WaitResponse)
	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		errCh <- ctx.Err()
	}()
	return statusCh, errCh
}

type delayedWaitClient struct {
	mockClient

	delay time.Duration
}

type trackingReadCloser struct {
	io.Reader

	closeCalls int
}

func (r *trackingReadCloser) Close() error {
	r.closeCalls++
	return nil
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func (m *delayedWaitClient) ContainerWait(
	_ context.Context,
	_ string,
	_ container.WaitCondition,
) (<-chan container.WaitResponse, <-chan error) {
	statusCh := make(chan container.WaitResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		time.Sleep(m.delay)
		statusCh <- m.waitResponse
	}()
	return statusCh, errCh
}

type delayedLogsClient struct {
	mockClient

	delay time.Duration
}

func (m *delayedLogsClient) ContainerLogs(
	ctx context.Context,
	_ string,
	_ container.LogsOptions,
) (io.ReadCloser, error) {
	time.Sleep(m.delay)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(m.logs)), nil
}

type durationClient struct {
	mockClient

	waitDelay time.Duration
	logsDelay time.Duration
}

func (m *durationClient) ContainerWait(
	_ context.Context,
	_ string,
	_ container.WaitCondition,
) (<-chan container.WaitResponse, <-chan error) {
	statusCh := make(chan container.WaitResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		time.Sleep(m.waitDelay)
		statusCh <- m.waitResponse
	}()
	return statusCh, errCh
}

func (m *durationClient) ContainerLogs(_ context.Context, _ string, _ container.LogsOptions) (io.ReadCloser, error) {
	time.Sleep(m.logsDelay)
	return io.NopCloser(bytes.NewReader(m.logs)), nil
}

type setupTimeoutClient struct {
	mockClient

	stage  string
	viaCtx bool
}

func (m *setupTimeoutClient) timeoutErr(ctx context.Context) error {
	if m.viaCtx {
		<-ctx.Done()
		return ctx.Err()
	}
	return context.DeadlineExceeded
}

func (m *setupTimeoutClient) ContainerCreate(
	ctx context.Context,
	config *container.Config,
	hostConfig *container.HostConfig,
	networkingConfig *network.NetworkingConfig,
	platform *ocispec.Platform,
	containerName string,
) (container.CreateResponse, error) {
	if m.stage == "create" {
		return container.CreateResponse{}, m.timeoutErr(ctx)
	}
	return m.mockClient.ContainerCreate(ctx, config, hostConfig, networkingConfig, platform, containerName)
}

func (m *setupTimeoutClient) CopyToContainer(
	ctx context.Context,
	containerID string,
	dstPath string,
	content io.Reader,
	options container.CopyToContainerOptions,
) error {
	if m.stage == "copy" {
		return m.timeoutErr(ctx)
	}
	return m.mockClient.CopyToContainer(ctx, containerID, dstPath, content, options)
}

func (m *setupTimeoutClient) ContainerStart(
	ctx context.Context,
	containerID string,
	options container.StartOptions,
) error {
	if m.stage == "start" {
		return m.timeoutErr(ctx)
	}
	return m.mockClient.ContainerStart(ctx, containerID, options)
}

func TestCollectContainerLogs_ExceedsLimit(t *testing.T) {
	large := strings.Repeat("x", defaultMaxContainerLogBytes+1)
	s := &Sandbox{client: &mockClient{logs: muxLogs(large, "")}}
	outBuf, errBuf, err := s.collectContainerLogs(context.Background(), "abc123")
	require.Error(t, err)
	require.NotNil(t, outBuf)
	require.NotNil(t, errBuf)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.Contains(t, err.Error(), "stdout")
}

func TestDocker_Run_LogOverflowWithCanceledCtx_InterruptWins(t *testing.T) {
	large := strings.Repeat("x", defaultMaxContainerLogBytes+1)
	ctx, cancel := context.WithCancel(context.Background())
	client := &cancelOnLogsClient{
		mockClient: mockClient{
			waitResponse: container.WaitResponse{StatusCode: 0},
			logs:         muxLogs(large, ""),
		},
		runCtx: ctx,
		cancel: cancel,
	}
	sb, err := New(WithClient(client))
	require.NoError(t, err)

	_, err = sb.Run(ctx, exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

type cancelOnLogsClient struct {
	mockClient

	runCtx context.Context
	cancel context.CancelFunc
}

func (c *cancelOnLogsClient) ContainerLogs(
	ctx context.Context,
	containerID string,
	options container.LogsOptions,
) (io.ReadCloser, error) {
	c.cancel()
	return c.mockClient.ContainerLogs(ctx, containerID, options)
}

func TestArchiveWorkspace_ExceedsSingleFileBudget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "huge.bin")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(int64(defaultMaxArchiveFileBytes)+1))
	require.NoError(t, f.Close())

	_, err = archiveWorkspace(context.Background(), root)
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.Contains(t, err.Error(), "exceeds")
}

func TestArchiveWorkspace_ExceedsTotalBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("archive total budget test writes multi-file sparse payloads")
	}
	root := t.TempDir()
	const fileSize = int64(defaultMaxArchiveFileBytes)
	for i := range 5 {
		path := filepath.Join(root, fmt.Sprintf("part%d.bin", i))
		f, err := os.Create(path)
		require.NoError(t, err)
		require.NoError(t, f.Truncate(fileSize))
		require.NoError(t, f.Close())
	}
	_, err := archiveWorkspace(context.Background(), root)
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.Contains(t, err.Error(), "workspace archive exceeds")
}

func TestArchiveWorkspace_CancelDuringWalk(t *testing.T) {
	root := t.TempDir()
	for i := range 10 {
		path := filepath.Join(root, "file"+strconv.Itoa(i)+".txt")
		require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := archiveWorkspace(ctx, root)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestReadArchiveFile_CanceledBeforeSizeCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readArchiveFile(ctx, func(string) (io.ReadCloser, error) {
		return nil, errors.New("should not open")
	}, "big.bin", 2048, 1024)
	require.ErrorIs(t, err, context.Canceled)
}

func TestReadArchiveFile_CancelOverSizeCap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readArchiveFile(ctx, func(string) (io.ReadCloser, error) {
		return nil, errors.New("should not open")
	}, "big.bin", 2048, 1024)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestReadArchiveFile_InterruptInChainOverReadLimit(t *testing.T) {
	composite := fmt.Errorf(
		"read: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	_, err := readArchiveFile(context.Background(), func(string) (io.ReadCloser, error) {
		return io.NopCloser(&instantErrReader{err: composite}), nil
	}, "f.bin", 0, 1024)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, exectool.ErrSandboxFailure)
}

func TestReadArchiveFile_CanceledAfterSuccessfulRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	data := []byte("ok")
	_, err := readArchiveFile(ctx, func(string) (io.ReadCloser, error) {
		cancel()
		return io.NopCloser(strings.NewReader(string(data))), nil
	}, "f.bin", int64(len(data)), 1024)
	require.ErrorIs(t, err, context.Canceled)
}

type instantErrReader struct {
	err error
}

func (r *instantErrReader) Read([]byte) (int, error) {
	return 0, r.err
}

func muxLogs(stdout, stderr string) []byte {
	var buf bytes.Buffer
	if stdout != "" {
		_, _ = io.WriteString(stdcopy.NewStdWriter(&buf, stdcopy.Stdout), stdout)
	}
	if stderr != "" {
		_, _ = io.WriteString(stdcopy.NewStdWriter(&buf, stdcopy.Stderr), stderr)
	}
	return buf.Bytes()
}
