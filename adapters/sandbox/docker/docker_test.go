package docker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
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
		Timeout:  time.Second,
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
		Timeout:  time.Second,
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
		Timeout:  time.Second,
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
				Timeout:  time.Second,
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

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "while True: pass",
		Timeout:  20 * time.Millisecond,
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

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "while True: pass",
		Timeout:  20 * time.Millisecond,
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

			_, err = sb.Run(context.Background(), exectool.RunRequest{
				Language: "python",
				Code:     "print(1)",
				Timeout:  20 * time.Millisecond,
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
		Timeout:  time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, "hello", res.Stdout)
	require.True(t, client.removed)
	require.NoError(t, client.removeCtxErr)
	// Remove uses cleanupTimeout (5s): deadline must comfortably exceed the wait delay.
	require.Greater(t, client.removeDeadlineIn, 80*time.Millisecond)
}

func TestRunCollectsLogsAfterRunContextExpires(t *testing.T) {
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
		Timeout:  20 * time.Millisecond,
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
		Timeout:  time.Second,
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
		Timeout:  time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, "python:3.12-alpine", client.createdConfig.Image)
	require.Equal(t, container.NetworkMode("none"), client.createdHostConfig.NetworkMode)
	require.EqualValues(t, 256*1024*1024, client.createdHostConfig.Memory)
}

func TestReadArchiveFileClosesFileBeforeReturn(t *testing.T) {
	rc := &trackingReadCloser{Reader: bytes.NewReader([]byte("payload"))}

	data, err := readArchiveFile(func(name string) (io.ReadCloser, error) {
		require.Equal(t, "data.txt", name)
		return rc, nil
	}, "data.txt")
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

	_, err := readArchiveFile(func(string) (io.ReadCloser, error) {
		return rc, nil
	}, "data.txt")
	require.Error(t, err)
	require.Equal(t, 1, rc.closeCalls)
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
