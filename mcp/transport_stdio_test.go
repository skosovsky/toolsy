package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()
	_, _, err := transport.Call(callCtx, "tools/list", nil)
	require.Error(t, err)
}
