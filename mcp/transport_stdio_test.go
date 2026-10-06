package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

func quietStdioTransport(executable string, args []string, opts ...StdioTransportOption) *StdioTransport {
	all := append([]StdioTransportOption{WithStdioLogger(slog.New(slog.DiscardHandler))}, opts...)
	return NewStdioTransport(executable, args, all...)
}

func TestStdioTransport_Start_CancelContext(t *testing.T) {
	t.Parallel()
	// Arrange. Start no longer waits for an unsolicited first line.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transport := quietStdioTransport("sleep", []string{"30"})
	// Act.
	err := transport.Start(ctx)
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, transport.cmd, "pre-cancelled Start must not create a process")
	require.NoError(t, transport.Close())
}

func TestStdioTransport_StderrLongLineDoesNotBreakStart(t *testing.T) {
	t.Parallel()
	// Arrange. Reply only after receiving a request and writing the 70KB stderr line.
	longLine := strings.Repeat("e", 70000)
	script := fmt.Sprintf(
		`IFS= read -r request; printf '%%s\n' %q 1>&2; echo '{"jsonrpc":"2.0","id":1,"result":{}}'; sleep 30`, longLine,
	)
	transport := quietStdioTransport("/bin/sh", []string{"-c", script})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, transport.Start(ctx))
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	pending, err := transport.PrepareRequest(ctx, "server/discover", migrationDiscoveryParams())
	require.NoError(t, err)
	// Act.
	require.NoError(t, pending.Deliver())
	result, err := pending.Await(ctx)
	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(result))
}

func TestStdioTransport_StdoutExceedsMaxStreamBytes(t *testing.T) {
	t.Parallel()
	// Arrange. Each frame fits; only their cumulative bytes exceed the cap.
	padding := strings.Repeat("x", 300)
	script := fmt.Sprintf(
		`IFS= read -r request; for i in 1 2 3 4 5 6 7 8 9 10; do echo '{"jsonrpc":"2.0","method":"notifications/pad","params":{"p":"%s"}}'; done; sleep 30`,
		padding,
	)
	transport := quietStdioTransport(
		"/bin/sh",
		[]string{"-c", script},
		WithStdioLimits(TransportLimits{MaxFrameBytes: 2048, MaxLifetimeBytes: 2048}),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, transport.Start(ctx))
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	pending, err := transport.PrepareRequest(ctx, "server/discover", migrationDiscoveryParams())
	require.NoError(t, err)
	// Act.
	require.NoError(t, pending.Deliver())
	_, err = pending.Await(ctx)
	// Assert.
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestStdioTransport_PreparedCancellationUnblocksPending(t *testing.T) {
	t.Parallel()
	// Arrange.
	transport := quietStdioTransport("/bin/sh", []string{"-c", `IFS= read -r request; sleep 30`})
	require.NoError(t, transport.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pending, err := transport.PrepareRequest(ctx, "server/discover", migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	delivery := pending.(DeliveryPendingRequest)
	select {
	case <-delivery.DeliveryDone():
	case <-ctx.Done():
		t.Fatal("request was not delivered before cancellation")
	}
	require.True(t, delivery.WasSent())
	// Act.
	cancel()
	_, err = pending.Await(ctx)
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
}

func TestStdioTransport_PrepareAfterStartContextCanceled(t *testing.T) {
	t.Parallel()
	// Arrange. Start's context must not become the process lifetime.
	script := `IFS= read -r request; echo '{"jsonrpc":"2.0","id":1,"result":{}}'; sleep 30`
	startCtx, cancelStart := context.WithCancel(context.Background())
	defer cancelStart()
	transport := quietStdioTransport("/bin/sh", []string{"-c", script})
	require.NoError(t, transport.Start(startCtx))
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Act.
	cancelStart()
	pending, err := transport.PrepareRequest(ctx, "server/discover", migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	result, err := pending.Await(ctx)
	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(result))
	cancelledCtx, cancelCall := context.WithCancel(context.Background())
	cancelCall()
	pending, err = transport.PrepareRequest(cancelledCtx, "server/discover", migrationDiscoveryParams())
	require.Nil(t, pending)
	require.ErrorIs(t, err, context.Canceled)
}

// Removed transport response helpers are tested at the current client error boundary.
func TestStdioTransport_CallerCancellationOverReadLimit(t *testing.T) {
	t.Parallel()
	// Arrange.
	client := &Client{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act.
	err := client.mapCallReadLimitFor(ctx, textprocessor.ErrReadLimitExceeded, "stdio response")
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
}

func TestStdioTransport_PreCancelledPrepareOverTerminalReadLimit(t *testing.T) {
	t.Parallel()
	// Arrange. A stale terminal fault must not mask an already cancelled caller.
	transport := quietStdioTransport("sleep", []string{"30"})
	require.NoError(t, transport.Start(context.Background()))
	require.NoError(t, transport.closeWithCause(textprocessor.ErrReadLimitExceeded))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act.
	pending, err := transport.PrepareRequest(ctx, "server/discover", migrationDiscoveryParams())
	// Assert.
	require.Nil(t, pending)
	require.ErrorIs(t, err, context.Canceled)
}

func TestStdioTransport_CallerCancellationOverStaleStream(t *testing.T) {
	t.Parallel()
	// Arrange.
	client := &Client{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act.
	err := client.mapCallReadLimitFor(ctx, errors.New("stdio stream closed"), "stdio response")
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
}

func TestStdioTransport_ReadLimitWithoutCancellation(t *testing.T) {
	t.Parallel()
	// Arrange.
	client := &Client{}
	// Act.
	err := client.mapCallReadLimitFor(context.Background(), textprocessor.ErrReadLimitExceeded, "stdio response")
	// Assert.
	require.ErrorIs(t, err, toolsy.ErrValidation)
	var validation *toolsy.ToolError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, toolsy.CodeValidationFailed, validation.Code)
	require.Equal(t, "stdio response exceeds 16777216 byte limit", validation.Reason)
}

func TestStdioTransport_InterruptInChainOverReadLimit(t *testing.T) {
	t.Parallel()
	// Arrange.
	client := &Client{}
	composite := fmt.Errorf("stream: %w", errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded))
	// Act.
	err := client.mapCallReadLimitFor(context.Background(), composite, "stdio response")
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Same(t, composite, err, "interrupt must not be reclassified as a read-limit failure")
}

func TestStdioTransport_TerminalInterruptOverReadLimit(t *testing.T) {
	t.Parallel()
	// Arrange.
	transport := quietStdioTransport("sleep", []string{"30"})
	require.NoError(t, transport.Start(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pending, err := transport.PrepareRequest(ctx, "server/discover", migrationDiscoveryParams())
	require.NoError(t, err)
	composite := fmt.Errorf("stream: %w", errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded))
	// Act.
	require.NoError(t, transport.closeWithCause(composite))
	_, err = pending.Await(ctx)
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}
