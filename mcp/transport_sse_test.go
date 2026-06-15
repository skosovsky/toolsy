package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func TestSSETransport_resolvePostURL_ValidateRemoteURL_BlocksPrivateIP(t *testing.T) {
	t.Parallel()
	impl := &sseTransportImpl{
		initialURL:      "http://example.com/sse",
		allowPrivateIPs: false,
	}
	resolved, err := impl.resolvePostURL("http://127.0.0.1:8080/rpc")
	require.NoError(t, err)
	err = httptool.ValidateRemoteURL(context.Background(), resolved.String(), impl.allowPrivateIPs)
	require.Error(t, err)
}

func TestSSETransport_resolvePostURL_AllowPrivateIPs(t *testing.T) {
	t.Parallel()
	impl := &sseTransportImpl{
		initialURL:      "http://example.com/sse",
		allowPrivateIPs: true,
	}
	resolved, err := impl.resolvePostURL("http://127.0.0.1:8080/rpc")
	require.NoError(t, err)
	err = httptool.ValidateRemoteURL(context.Background(), resolved.String(), impl.allowPrivateIPs)
	require.NoError(t, err)
}

func TestSSETransport_ValidateInitialURL_BlocksPrivateIP(t *testing.T) {
	t.Parallel()
	err := httptool.ValidateRemoteURL(context.Background(), "http://127.0.0.1/sse", false)
	require.Error(t, err)
}

func TestSSETransport_notify_Non2xxStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	impl := &sseTransportImpl{
		initialURL:      srv.URL,
		client:          srv.Client(),
		allowPrivateIPs: true,
		ready:           make(chan struct{}),
		readerDone:      make(chan struct{}),
	}
	impl.postURL = srv.URL + "/messages"
	close(impl.ready)

	err := impl.notify(context.Background(), "notifications/test", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}

func TestSSETransport_call_Non2xxPOSTStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "messages") {
			http.Error(w, "fail", http.StatusBadRequest)
			return
		}
		http.Error(w, "unexpected", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	impl := &sseTransportImpl{
		initialURL:      srv.URL,
		client:          srv.Client(),
		allowPrivateIPs: true,
		ready:           make(chan struct{}),
		readerDone:      make(chan struct{}),
	}
	impl.postURL = srv.URL + "/messages"
	close(impl.ready)

	_, _, err := impl.call(context.Background(), "tools/list", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "400")
}

func TestSSETransport_Start_Non2xxGET(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	tr := NewSSETransport(srv.URL, WithSSEAllowPrivateIPs(true), WithSSEHTTPClient(srv.Client()))
	err := tr.Start(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}

func TestSSETransport_Start_Accepts204GET(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	tr := NewSSETransport(srv.URL, WithSSEAllowPrivateIPs(true), WithSSEHTTPClient(srv.Client()))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := tr.Start(ctx)
	if err != nil {
		require.NotContains(t, err.Error(), "status 204")
	}
	_ = tr.Close()
}

func TestSSETransport_Start_CancelBeforeEndpoint(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, ": keepalive\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	tr := NewSSETransport(srv.URL, WithSSEAllowPrivateIPs(true), WithSSEHTTPClient(srv.Client()))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	err := tr.Start(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, tr.Close())
}

func TestSSETransport_Call_CancelUnblocksPending(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: endpoint\ndata: /messages\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	tr := NewSSETransport(srv.URL, WithSSEAllowPrivateIPs(true), WithSSEHTTPClient(srv.Client()))
	require.NoError(t, tr.Start(context.Background()))
	t.Cleanup(func() { _ = tr.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	_, _, err := tr.Call(ctx, "tools/list", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSSETransport_ExceedsMaxStreamBytes(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: endpoint\ndata: /messages\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		for range 50 {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", strings.Repeat("x", 200))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)

	tr := NewSSETransport(srv.URL,
		WithSSEAllowPrivateIPs(true),
		WithSSEHTTPClient(srv.Client()),
		WithSSEMaxStreamBytes(2048),
	)
	require.NoError(t, tr.Start(context.Background()))
	t.Cleanup(func() { _ = tr.Close() })

	callCtx, callCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer callCancel()
	_, _, err := tr.Call(callCtx, "tools/list", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestSSETransport_finishSSECallResponse_CancelOverReadLimit(t *testing.T) {
	t.Parallel()
	impl := &sseTransportImpl{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := impl.finishSSECallResponse(ctx, &sseCallResult{Err: textprocessor.ErrReadLimitExceeded})
	require.ErrorIs(t, err, context.Canceled)
}

func TestSSETransport_finishSSECallResponse_CancelOverStaleStream(t *testing.T) {
	t.Parallel()
	impl := &sseTransportImpl{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := impl.finishSSECallResponse(ctx, &sseCallResult{Err: errors.New("sse stream closed")})
	require.ErrorIs(t, err, context.Canceled)
}

func TestSSETransport_finishSSECallResponse_LimitWithoutCancel(t *testing.T) {
	t.Parallel()
	impl := &sseTransportImpl{}
	_, err := impl.finishSSECallResponse(context.Background(), &sseCallResult{Err: textprocessor.ErrReadLimitExceeded})
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestSSETransport_finishSSECallResponse_InterruptInChainOverReadLimit(t *testing.T) {
	t.Parallel()
	impl := &sseTransportImpl{}
	composite := fmt.Errorf(
		"stream: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	_, err := impl.finishSSECallResponse(context.Background(), &sseCallResult{Err: composite})
	require.ErrorIs(t, err, context.Canceled)
}

func TestSSETransport_streamLimitErr_InterruptOverReadLimit(t *testing.T) {
	t.Parallel()
	impl := &sseTransportImpl{}
	impl.streamErr = fmt.Errorf(
		"stream: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	err := impl.streamLimitErr()
	require.ErrorIs(t, err, context.Canceled)
}

func TestSSETransport_getPostURL_StreamLimitBlocksEndpoint(t *testing.T) {
	t.Parallel()
	impl := &sseTransportImpl{ready: make(chan struct{})}
	close(impl.ready)
	impl.postURL = "http://example.com/messages"
	impl.streamErr = textprocessor.ErrReadLimitExceeded
	_, err := impl.getPostURL(context.Background())
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestSSETransport_getPostURL_CancelOverStreamLimit(t *testing.T) {
	t.Parallel()
	impl := &sseTransportImpl{ready: make(chan struct{})}
	close(impl.ready)
	impl.postURL = "http://example.com/messages"
	impl.streamErr = textprocessor.ErrReadLimitExceeded
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := impl.getPostURL(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSSETransport_call_CancelOverStreamLimit(t *testing.T) {
	t.Parallel()
	impl := &sseTransportImpl{ready: make(chan struct{})}
	close(impl.ready)
	impl.postURL = "http://example.com/messages"
	impl.streamErr = textprocessor.ErrReadLimitExceeded
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := impl.call(ctx, "tools/list", nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSSETransport_waitSSECallResponse_DeadlineOverReadLimit(t *testing.T) {
	t.Parallel()
	impl := &sseTransportImpl{
		callResponseTimeout: time.Second,
	}
	impl.streamErr = textprocessor.ErrReadLimitExceeded
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Millisecond))
	defer cancel()
	ch := make(chan *sseCallResult)
	_, err := impl.waitSSECallResponse(ctx, "tools/list", ch)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}
