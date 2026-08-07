package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTask34ProcessTransport(mode string) *StdioTransport {
	return NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestTask34StdioHelperProcess", "--", mode},
	)
}

func prepareTask34ProcessRequest(t *testing.T, transport *StdioTransport) PreparedRequest {
	t.Helper()
	pending, err := transport.PrepareRequest(t.Context(), MethodServerDiscover, DiscoverParams{
		Meta: requestMetaForContract(),
	})
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	return pending
}

func TestTask34StdioCrashUnblocksWaiterAndReapsProcess(t *testing.T) {
	// Arrange.
	transport := newTask34ProcessTransport("crash")
	require.NoError(t, transport.Start(t.Context()))
	pending := prepareTask34ProcessRequest(t, transport)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	// Act.
	_, err := pending.Await(ctx)

	// Assert.
	var crash *TransportCrashError
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &crash)
	require.ErrorAs(t, err, &exitErr)
	requireClosedBefore(t, transport.processDone, "stdio child was not reaped")
	require.NoError(t, transport.Close())
	requireClosedBefore(t, transport.writerDone, "stdio writer leaked")
	requireClosedBefore(t, transport.readerDone, "stdio reader leaked")
	requireClosedBefore(t, transport.stderrDone, "stdio stderr forwarder leaked")
}

func TestTask34StdioConcurrentCloseKillsDescendantProcessTree(t *testing.T) {
	// Arrange.
	transport := newTask34ProcessTransport("descendant")
	require.NoError(t, transport.Start(t.Context()))
	pending := prepareTask34ProcessRequest(t, transport)
	result, err := pending.Await(t.Context())
	require.NoError(t, err)
	var child struct {
		PID int `json:"pid"`
	}
	require.NoError(t, json.Unmarshal(result, &child))
	require.Positive(t, child.PID)
	require.True(t, processExists(child.PID))
	start := make(chan struct{})
	errorsByCaller := make(chan error, 16)
	var callers sync.WaitGroup
	callers.Add(cap(errorsByCaller))

	// Act.
	for range cap(errorsByCaller) {
		go func() {
			defer callers.Done()
			<-start
			errorsByCaller <- transport.Close()
		}()
	}
	close(start)
	callers.Wait()
	close(errorsByCaller)

	// Assert.
	for closeErr := range errorsByCaller {
		require.NoError(t, closeErr)
	}
	require.Eventually(t, func() bool { return !processExists(child.PID) }, 3*time.Second, 10*time.Millisecond)
	requireClosedBefore(t, transport.processDone, "stdio child was not reaped")
	requireClosedBefore(t, transport.writerDone, "stdio writer leaked")
	requireClosedBefore(t, transport.readerDone, "stdio reader leaked")
	requireClosedBefore(t, transport.stderrDone, "stdio stderr forwarder leaked")
}

func requireClosedBefore(t *testing.T, channel <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}

func TestTask34StdioHelperProcess(_ *testing.T) {
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		return
	}
	mode := os.Args[separator+1]
	if mode == "long-lived-descendant" {
		time.Sleep(30 * time.Second)
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var request Request
	if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
		os.Exit(3)
	}
	switch mode {
	case "crash":
		os.Exit(7)
	case "descendant":
		child := newLongLivedDescendant()
		if child == nil || child.Start() != nil {
			os.Exit(8)
		}
		response := Response{
			JSONRPC: JSONRPCVersion,
			ID:      request.ID,
			Result:  fmt.Appendf(nil, `{"resultType":"complete","pid":%d}`, child.Process.Pid),
		}
		encoded, err := json.Marshal(response)
		if err != nil {
			os.Exit(9)
		}
		_, _ = fmt.Fprintln(os.Stdout, string(encoded))
		time.Sleep(30 * time.Second)
	default:
		os.Exit(10)
	}
}
