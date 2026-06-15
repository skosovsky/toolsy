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

	"github.com/skosovsky/toolsy/textprocessor"
)

func quietStdioTransport(executable string, args []string, opts ...StdioTransportOption) *StdioTransport {
	all := append([]StdioTransportOption{WithLogger(slog.New(slog.DiscardHandler))}, opts...)
	return NewStdioTransport(executable, args, all...)
}

func TestStdioTransport_Start_CancelContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	transport := quietStdioTransport("sleep", []string{"30"}, WithStdioFirstLineTimeout(5*time.Second))
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	err := transport.Start(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, transport.Close())
}

func TestStdioTransport_StderrLongLineDoesNotBreakStart(t *testing.T) {
	t.Parallel()
	longLine := strings.Repeat("e", 70000)
	script := fmt.Sprintf(
		`echo '{"jsonrpc":"2.0","id":1,"result":null}' && printf '%%s\n' %q 1>&2 && sleep 30`,
		longLine,
	)
	transport := quietStdioTransport("/bin/sh", []string{"-c", script}, WithStdioFirstLineTimeout(5*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := transport.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, transport.Close())
}

func TestStdioTransport_StdoutExceedsMaxStreamBytes(t *testing.T) {
	t.Parallel()
	// Notification (no id) satisfies Start; valid JSON padding lines exceed stream cap without binary stdout noise.
	padding := strings.Repeat("x", 300)
	script := fmt.Sprintf(
		`echo '{"jsonrpc":"2.0","method":"notifications/initialized"}' && for i in 1 2 3 4 5 6 7 8 9 10; do echo '{"jsonrpc":"2.0","method":"notifications/pad","params":{"p":"%s"}}'; done && sleep 30`,
		padding,
	)
	transport := quietStdioTransport("/bin/sh", []string{"-c", script},
		WithStdioFirstLineTimeout(5*time.Second),
		WithStdioMaxStreamBytes(2048),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, transport.Start(ctx))
	t.Cleanup(func() { _ = transport.Close() })

	callCtx, callCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer callCancel()
	_, _, err := transport.Call(callCtx, "tools/list", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestStdioTransport_Call_CancelUnblocksPending(t *testing.T) {
	t.Parallel()
	script := `echo '{"jsonrpc":"2.0","method":"notifications/initialized"}' && sleep 30`
	transport := quietStdioTransport("/bin/sh", []string{"-c", script}, WithStdioFirstLineTimeout(5*time.Second))
	require.NoError(t, transport.Start(context.Background()))
	t.Cleanup(func() { _ = transport.Close() })

	callCtx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	_, _, err := transport.Call(callCtx, "tools/list", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestStdioTransport_CallAfterStartContextCanceled(t *testing.T) {
	t.Parallel()
	// Notification (no id) satisfies Start without colliding with the first Call's request id.
	script := `echo '{"jsonrpc":"2.0","method":"notifications/initialized"}' && sleep 30`
	ctx, cancel := context.WithCancel(context.Background())
	transport := quietStdioTransport("/bin/sh", []string{"-c", script}, WithStdioFirstLineTimeout(5*time.Second))
	require.NoError(t, transport.Start(ctx))
	cancel()
	t.Cleanup(func() { _ = transport.Close() })

	callCtx, callCancel := context.WithCancel(context.Background())
	callCancel()
	_, _, err := transport.Call(callCtx, "tools/list", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestStdioTransport_finishStdioCallResponse_CancelOverReadLimit(t *testing.T) {
	t.Parallel()
	transport := &stdioTransport{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := transport.finishStdioCallResponse(ctx, &callResult{Err: textprocessor.ErrReadLimitExceeded})
	require.ErrorIs(t, err, context.Canceled)
}

func TestStdioTransport_call_CancelOverStreamLimit(t *testing.T) {
	t.Parallel()
	script := `echo '{"jsonrpc":"2.0","method":"notifications/initialized"}' && sleep 30`
	transport := quietStdioTransport("/bin/sh", []string{"-c", script}, WithStdioFirstLineTimeout(5*time.Second))
	require.NoError(t, transport.Start(context.Background()))
	t.Cleanup(func() { _ = transport.Close() })

	transport.impl.streamMu.Lock()
	transport.impl.streamErr = textprocessor.ErrReadLimitExceeded
	transport.impl.streamMu.Unlock()

	callCtx, callCancel := context.WithCancel(context.Background())
	callCancel()
	_, _, err := transport.Call(callCtx, "tools/list", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestStdioTransport_finishStdioCallResponse_CancelOverStaleStream(t *testing.T) {
	t.Parallel()
	transport := &stdioTransport{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := transport.finishStdioCallResponse(ctx, &callResult{Err: errors.New("stdio stream closed")})
	require.ErrorIs(t, err, context.Canceled)
}

func TestStdioTransport_finishStdioCallResponse_LimitWithoutCancel(t *testing.T) {
	t.Parallel()
	transport := &stdioTransport{}
	_, err := transport.finishStdioCallResponse(
		context.Background(),
		&callResult{Err: textprocessor.ErrReadLimitExceeded},
	)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestStdioTransport_finishStdioCallResponse_InterruptInChainOverReadLimit(t *testing.T) {
	t.Parallel()
	transport := &stdioTransport{}
	composite := fmt.Errorf(
		"stream: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	_, err := transport.finishStdioCallResponse(context.Background(), &callResult{Err: composite})
	require.ErrorIs(t, err, context.Canceled)
}

func TestStdioTransport_streamLimitErr_InterruptOverReadLimit(t *testing.T) {
	t.Parallel()
	transport := &stdioTransport{}
	transport.streamErr = fmt.Errorf(
		"stream: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	err := transport.streamLimitErr()
	require.ErrorIs(t, err, context.Canceled)
}
