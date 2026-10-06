package docker

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/system"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
)

func TestSecurePolicyRequest(t *testing.T) {
	// Arrange.
	client := &mockClient{}
	sb, err := New(WithClient(client))
	require.NoError(t, err)
	// Act.
	_, err = sb.Run(context.Background(), exectool.RunRequest{Language: "python", Code: "print(1)"})
	// Assert.
	require.NoError(t, err)
	hc := client.createdHostConfig
	require.True(t, hc.ReadonlyRootfs)
	require.Equal(t, container.NetworkMode("none"), hc.NetworkMode)
	require.Equal(t, []string{"ALL"}, []string(hc.CapDrop))
	require.Contains(t, hc.SecurityOpt, "no-new-privileges:true")
	require.Positive(t, hc.Memory)
	require.Equal(t, hc.Memory, hc.MemorySwap)
	require.Positive(t, hc.CPUQuota)
	require.Positive(t, *hc.PidsLimit)
	require.Equal(t, "65534:65534", client.createdConfig.User)
	require.Contains(t, hc.Binds[0], ":/workspace:ro")
	require.Contains(t, hc.Tmpfs["/tmp"], "size=33554432")
	require.EqualValues(t, 33554432, hc.ShmSize)
	require.Equal(t, container.LogConfig{Type: "json-file"}, hc.LogConfig)
}
func TestPolicyRefusesMissingBoundsAndCapabilities(t *testing.T) {
	// Arrange / Act / Assert.
	_, err := New(WithMemoryLimit(0))
	require.Error(t, err)
	client := &unsupportedClient{}
	sb, err := New(WithClient(client))
	require.NoError(t, err)
	_, err = sb.Run(context.Background(), exectool.RunRequest{Language: "python"})
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.Nil(t, client.createdConfig)
}

type unsupportedClient struct{ mockClient }

func (*unsupportedClient) Info(context.Context) (system.Info, error) { return system.Info{}, nil }

func TestLogCollectionFailureNeverSucceeds(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      []byte
		transport error
	}{
		{name: "transport", transport: errors.New("logs disconnected")},
		{name: "short header", body: []byte{1, 0, 0}},
		{name: "short payload", body: []byte{1, 0, 0, 0, 0, 0, 0, 3, 'x'}},
		{name: "bad stream", body: []byte{7, 0, 0, 0, 0, 0, 0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			client := &logFailureClient{logs: tc.body, logErr: tc.transport}
			sb, err := New(WithClient(client))
			require.NoError(t, err)
			// Act.
			_, err = sb.Run(context.Background(), exectool.RunRequest{Language: "python"})
			// Assert.
			require.ErrorIs(t, err, exectool.ErrSandboxFailure)
			require.True(t, client.removed)
		})
	}
}

type logFailureClient struct {
	mockClient

	logErr error
}

func (c *logFailureClient) ContainerLogs(context.Context, string, container.LogsOptions) (io.ReadCloser, error) {
	if c.logErr != nil {
		return nil, c.logErr
	}
	return io.NopCloser(strings.NewReader(string(c.logs))), nil
}

func TestCleanupFailurePreservesGuestResultAndPrimary(t *testing.T) {
	for _, transport := range []error{nil, errors.New("lost logs")} {
		// Arrange.
		client := &cleanupFailureClient{
			waitResponse: container.WaitResponse{StatusCode: 7}, logs: muxLogs("ok", ""),
			logErr: transport,
		}
		sb, err := New(WithClient(client))
		require.NoError(t, err)
		// Act.
		result, err := sb.Run(context.Background(), exectool.RunRequest{Language: "python"})
		// Assert.
		require.ErrorIs(t, err, exectool.ErrSandboxCleanup)
		if transport == nil {
			require.Equal(t, 7, result.ExitCode)
			require.Equal(t, "ok", result.Stdout)
		} else {
			require.ErrorIs(t, err, transport)
		}
	}
}

type cleanupFailureClient struct{ logFailureClient }

func (*cleanupFailureClient) ContainerRemove(context.Context, string, container.RemoveOptions) error {
	return errors.New("remove refused")
}

func TestOwnedLogBodyClosedOnCancellation(t *testing.T) {
	// Arrange.
	body := &blockedLogBody{done: make(chan struct{})}
	client := &blockingLogClient{body: body}
	sb, err := New(WithClient(client))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	// Act.
	_, err = sb.Run(ctx, exectool.RunRequest{Language: "python"})
	// Assert.
	require.ErrorIs(t, err, exectool.ErrTimeout)
	select {
	case <-body.done:
	default:
		t.Fatal("owned body was not closed")
	}
}

type blockedLogBody struct {
	done chan struct{}
	once sync.Once
}

func (b *blockedLogBody) Read([]byte) (int, error) { <-b.done; return 0, io.ErrClosedPipe }
func (b *blockedLogBody) Close() error             { b.once.Do(func() { close(b.done) }); return nil }

type blockingLogClient struct {
	mockClient

	body *blockedLogBody
}

func (c *blockingLogClient) ContainerLogs(context.Context, string, container.LogsOptions) (io.ReadCloser, error) {
	return c.body, nil
}

func TestOutputOverflowDoesNotLeakInternalCancellation(t *testing.T) {
	// Arrange.
	p := DefaultPolicy()
	p.OutputBytes = 10
	client := &timeoutClient{logs: muxLogs(strings.Repeat("x", 20), "")}
	sb, err := New(WithClient(client), WithPolicy(p))
	require.NoError(t, err)
	// Act.
	_, err = sb.Run(context.Background(), exectool.RunRequest{Language: "python"})
	// Assert.
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.NotErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, context.DeadlineExceeded)
	require.True(t, client.killed)
}
func TestCollectionDeadlineRemainsInfrastructureFailure(t *testing.T) {
	// Arrange.
	client := &logFailureClient{logErr: context.DeadlineExceeded}
	sb, err := New(WithClient(client))
	require.NoError(t, err)
	// Act.
	_, err = sb.Run(context.Background(), exectool.RunRequest{Language: "python"})
	// Assert.
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.NotErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, exectool.ErrTimeout)
}

func TestActualLogDeadlineClosesBodyAndRemainsInfrastructure(t *testing.T) {
	// Arrange.
	p := DefaultPolicy()
	p.LogTimeout = 20 * time.Millisecond
	body := &blockedLogBody{done: make(chan struct{})}
	client := &blockingLogClient{body: body}
	sb, err := New(WithClient(client), WithPolicy(p))
	require.NoError(t, err)
	// Act.
	_, err = sb.Run(context.Background(), exectool.RunRequest{Language: "python"})
	// Assert.
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.NotErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, exectool.ErrTimeout)
	select {
	case <-body.done:
	default:
		t.Fatal("owned body not closed")
	}
}
