package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

func TestTransportLimitsRejectNegativeAndNilOptionsBeforeStart(t *testing.T) {
	for _, limits := range []TransportLimits{{MaxFrameBytes: -1}, {MaxQueueBytes: -1}, {MaxInFlight: -1}, {MaxLifetimeBytes: -1}} {
		// Arrange.
		stdio := NewStdioTransport("must-not-run", nil, WithStdioLimits(limits))
		http := NewStreamableHTTPTransport("https://must-not-resolve.invalid", WithStreamableHTTPLimits(limits))
		// Act/Assert.
		require.ErrorContains(t, stdio.Start(t.Context()), "negative")
		require.ErrorContains(t, http.Start(t.Context()), "negative")
		require.NoError(t, stdio.Close())
		require.NoError(t, http.Close())
	}
	require.ErrorContains(t, NewStdioTransport("unused", nil, nil).Start(t.Context()), "nil stdio option")
	require.ErrorContains(
		t,
		NewStreamableHTTPTransport("https://unused.invalid", nil).Start(t.Context()),
		"nil HTTP option",
	)
}

func TestSSEFrameBudgetIsIndependentOfOptionalLifetime(t *testing.T) {
	for _, lifetime := range []int{0, 2048} {
		t.Run(strconv.Itoa(lifetime), func(t *testing.T) {
			// Arrange: many individually small frames exceed only the optional lifetime budget.
			peer := newRPCPeer(t.Context(), nil, func(context.Context, []byte) error { return nil })
			defer peer.close(ErrTransportClosed)
			pending, _, err := peer.beginRequest("test", struct{}{})
			require.NoError(t, err)
			transport := &StreamableHTTPTransport{
				peer:          peer,
				maxFrameBytes: 512,
				limits:        TransportLimits{MaxLifetimeBytes: lifetime},
			}
			stream := strings.Repeat(
				":"+strings.Repeat("x", 100)+"\n\n",
				100,
			) + fmt.Sprintf(
				"data: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{}}\n\n",
				pending.ID(),
			)
			// Act.
			err = transport.consumeRequestSSE(
				t.Context(),
				strings.NewReader(stream),
				&sseRequestProvenance{requestID: pending.ID(), method: "test"},
			)
			// Assert.
			if lifetime == 0 {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
			}
		})
	}
}

func TestSSEFrameBudgetCountsMultipleDataLinesAndDelimiters(t *testing.T) {
	// Arrange: neither line alone exceeds the frame budget.
	transport := &StreamableHTTPTransport{maxFrameBytes: 512}
	stream := "data: " + strings.Repeat("x", 250) + "\r\ndata: " + strings.Repeat("x", 250) + "\r\n\r\n"
	// Act.
	err := transport.consumeRequestSSE(t.Context(), strings.NewReader(stream), &sseRequestProvenance{})
	// Assert: fail on resource bounds before interpreting the incomplete JSON value.
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestStdioTrafficCanExceedFrameBudgetAcrossMessages(t *testing.T) {
	// Arrange: a long-lived child emits many valid bounded notifications.
	script := `IFS= read -r request; i=0; while [ "$i" -lt 100 ]; do printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/vendor/pad","params":{"padding":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}}'; i=$((i+1)); done; printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{}}'; sleep 30`
	transport := quietStdioTransport(
		"/bin/sh",
		[]string{"-c", script},
		WithStdioLimits(TransportLimits{MaxFrameBytes: 512}),
	)
	require.NoError(t, transport.Start(t.Context()))
	defer transport.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	pending, err := transport.PrepareRequest(ctx, "test", migrationDiscoveryParams())
	require.NoError(t, err)
	// Act.
	require.NoError(t, pending.Deliver())
	raw, err := pending.Await(ctx)
	// Assert: no cumulative default cap ends the stream.
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(raw))
}

func TestPendingRequestBudgetReleasesOnTerminalPaths(t *testing.T) {
	for _, byteBound := range []bool{false, true} {
		t.Run(strconv.FormatBool(byteBound), func(t *testing.T) {
			// Arrange.
			peer := newRPCPeer(t.Context(), nil, func(context.Context, []byte) error { return nil })
			defer peer.close(ErrTransportClosed)
			peer.limits.MaxInFlight = 1
			if byteBound {
				peer.limits.MaxInFlight = 10
				peer.limits.MaxQueueBytes = 100
			}
			first, _, err := peer.beginRequest("first", struct{}{})
			require.NoError(t, err)
			// Act: a second request exceeds either count or retained byte capacity.
			refused, _, err := peer.beginRequest("second", struct{}{})
			// Assert.
			require.ErrorIs(t, err, ErrTransportLimitExceeded)
			require.Nil(t, refused)
			require.Len(t, peer.pending, 1)
			peer.failPending(first, context.Canceled)
			require.Zero(t, peer.pendingBytes)
			next, _, err := peer.beginRequest("next", struct{}{})
			require.NoError(t, err)
			require.True(t, next.CancelPending())
			require.Zero(t, peer.pendingBytes)
		})
	}
}

func TestTransportRegressionStdioCRLFFrameBudget(t *testing.T) {
	// Arrange: scanner accepts CRLF JSON lines but raw frame exceeds budget by one.
	body := `{"jsonrpc":"2.0","method":"notifications/vendor/pad","params":{"x":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}}`
	tr := quietStdioTransport(
		"/bin/sh",
		[]string{"-c", "printf '%s\\r\\n' '" + body + "'; sleep 30"},
		WithStdioLimits(TransportLimits{MaxFrameBytes: len(body) + 1}),
	)
	delivered := make(chan struct{}, 1)
	tr.OnNotification("notifications/vendor/pad", func(json.RawMessage) { delivered <- struct{}{} })
	require.NoError(t, tr.Start(t.Context()))
	defer tr.Close()
	// Act/Assert: do not dispatch raw over-budget frame.
	select {
	case <-delivered:
		t.Fatal("over-budget CRLF frame dispatched")
	case <-time.After(250 * time.Millisecond):
	}
}

func TestTransportRegressionStdioOutgoingNotificationTypedLimit(t *testing.T) {
	// Arrange.
	tr := quietStdioTransport(
		"/bin/sh",
		[]string{"-c", "sleep 30"},
		WithStdioLimits(TransportLimits{MaxFrameBytes: 128}),
	)
	require.NoError(t, tr.Start(t.Context()))
	defer tr.Close()
	// Act.
	e := tr.Notify(t.Context(), "notifications/vendor/pad", map[string]any{"pad": strings.Repeat("x", 256)})
	// Assert.
	require.ErrorIs(t, e, textprocessor.ErrReadLimitExceeded)
}

func TestTransportAdmissionLimitIdentity(t *testing.T) {
	// Arrange.
	limits := TransportLimits{MaxInFlight: 1, MaxQueueBytes: 10}
	// Act.
	countErr := retainedWorkLimit(1, 0, 1, limits)
	byteErr := retainedWorkLimit(0, 9, 2, limits)
	// Assert.
	for _, err := range []error{countErr, byteErr} {
		require.ErrorIs(t, err, ErrTransportLimitExceeded)
		var limitErr *TransportLimitError
		require.ErrorAs(t, err, &limitErr)
		require.Positive(t, limitErr.Limit)
	}
	require.NoError(t, retainedWorkLimit(0, 9, 1, limits))
}
