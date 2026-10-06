package agents

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

type cancellationCredentials struct{ failure error }

func (c cancellationCredentials) GetAuth(_ context.Context, name string) (string, error) {
	if name == cancelTaskAuthToolName {
		return "", c.failure
	}
	return "", nil
}

func TestCancellationDiagnosticsStagesAndParentCauses(t *testing.T) {
	authFailure := errors.New("credential provider unavailable")
	parentCause := errors.New("host requested interruption")
	for _, mode := range []string{"acknowledged", "http failure", "credential failure", "parent deadline", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: cleanup has a known reference, even though the parent is already interrupted.
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				if mode == "http failure" {
					w.WriteHeader(http.StatusServiceUnavailable)
				} else {
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			t.Cleanup(server.Close)
			var observed []CancellationDiagnostic
			observer := func(ctx context.Context, d CancellationDiagnostic) {
				deadline, ok := ctx.Deadline()
				assert.True(t, ok)
				assert.Greater(t, time.Until(deadline), time.Duration(0))
				assert.LessOrEqual(t, time.Until(deadline), cancelTaskTimeout)
				require.NoError(t, ctx.Err())
				observed = append(observed, d)
			}
			options := []func(*ClientOptions){WithAllowPrivateIPs(true)}
			if mode != "disabled" {
				options = append(options, WithCancellationObserver(observer))
			}
			client := mustClient(t, server.URL, options...)
			parent, cancel := context.WithCancelCause(t.Context())
			cancel(parentCause)
			if mode == "parent deadline" {
				deadlineCtx, deadlineCancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Hour))
				defer deadlineCancel()
				parent = deadlineCtx
			}
			run := toolsy.NewRunEnv(nil)
			if mode == "credential failure" {
				run = toolsy.NewRunEnv(nil, toolsy.WithCredentials(cancellationCredentials{failure: authFailure}))
			}
			// Act.
			client.cancelInterruptedTask(parent, run, "task")
			// Assert: exactly one diagnostic, original causes retained, no remote stop claim.
			assertCancellationDiagnostic(t, mode, observed, requests.Load(), authFailure, parentCause)
		})
	}
}

func TestCancellationDiagnosticDoesNotReplaceExecutionOutcome(t *testing.T) {
	for _, mode := range []string{"parent cancel", "stream timeout", "callback stop"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: real create/SSE remote action, failing cancel endpoint.
			var cancelled, observations atomic.Int32
			server := httptest.NewServer(cancellationStreamHandler(&cancelled))
			t.Cleanup(server.Close)
			policy := testPolicy()
			policy.MaxReconnects = 0
			if mode == "stream timeout" {
				policy.Timeout = 100 * time.Millisecond
			}
			client := mustClient(
				t,
				server.URL,
				WithAllowPrivateIPs(true),
				WithStreamPolicy(policy),
				WithCancellationObserver(func(_ context.Context, d CancellationDiagnostic) {
					observations.Add(1)
					assert.False(t, d.Acknowledged)
					require.ErrorContains(t, d.Cause, "status 503")
				}),
			)
			tool, err := AsTool("delegate", "remote", []byte(`{"type":"object"}`), client)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			callbackFailure := errors.New("host consumer stopped")
			// Act.
			executionErr := tool.Execute(
				ctx,
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
				func(chunk toolsy.Chunk) error {
					if chunk.Event == toolsy.EventProgress {
						if mode == "parent cancel" {
							cancel()
						}
						if mode == "callback stop" {
							return callbackFailure
						}
					}
					return nil
				},
			)
			// Assert: parent cancellation alone sends cleanup; other read stops preserve their cause.
			require.Error(t, executionErr)
			assertCancellationExecution(
				ctx,
				t,
				mode,
				executionErr,
				callbackFailure,
				cancelled.Load(),
				observations.Load(),
			)
		})
	}
}

func assertCancellationDiagnostic(
	t *testing.T,
	mode string,
	observed []CancellationDiagnostic,
	requests int32,
	authFailure, parentCause error,
) {
	t.Helper()
	if mode == "disabled" {
		assert.Empty(t, observed)
		assert.EqualValues(t, 1, requests)
		return
	}
	require.Len(t, observed, 1)
	diagnostic := observed[0]
	assert.Equal(t, "task", diagnostic.TaskID)
	if mode == "parent deadline" {
		require.ErrorIs(t, diagnostic.ParentInterrupt, context.DeadlineExceeded)
		require.ErrorIs(t, diagnostic.ParentCause, context.DeadlineExceeded)
	} else {
		require.ErrorIs(t, diagnostic.ParentInterrupt, context.Canceled)
		require.ErrorIs(t, diagnostic.ParentCause, parentCause)
	}
	if mode == "credential failure" {
		assert.Equal(t, CancellationCredentials, diagnostic.Stage)
		require.ErrorIs(t, diagnostic.Cause, authFailure)
		assert.False(t, diagnostic.Acknowledged)
		assert.Zero(t, requests)
		return
	}
	assert.Equal(t, CancellationRequest, diagnostic.Stage)
	assert.EqualValues(t, 1, requests)
	if mode == "http failure" {
		require.ErrorContains(t, diagnostic.Cause, "status 503")
		assert.False(t, diagnostic.Acknowledged)
	} else {
		require.NoError(t, diagnostic.Cause)
		assert.True(t, diagnostic.Acknowledged)
	}
}

func cancellationStreamHandler(cancelled *atomic.Int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cancel"):
			cancelled.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		case strings.HasSuffix(r.URL.Path, "/tasks"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"task_id":"t","artifacts":[]}`))
		default:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(stepFrame("s", "running", false)))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}
	})
}

func assertCancellationExecution(
	ctx context.Context,
	t *testing.T,
	mode string,
	executionErr, callbackFailure error,
	cancelled, observations int32,
) {
	t.Helper()
	switch mode {
	case "parent cancel":
		require.ErrorIs(t, executionErr, context.Canceled)
		assert.EqualValues(t, 1, cancelled)
		assert.EqualValues(t, 1, observations)
	case "callback stop":
		require.ErrorIs(t, executionErr, callbackFailure)
		assert.Zero(t, cancelled)
		assert.Zero(t, observations)
	case "stream timeout":
		require.ErrorIs(t, executionErr, toolsy.ErrTimeout)
		require.NoError(t, ctx.Err())
		assert.Zero(t, cancelled)
		assert.Zero(t, observations)
		toolErr, ok := toolsy.AsToolError(executionErr)
		require.True(t, ok)
		assert.False(t, toolErr.Retryable)
	}
}

func TestCancellationDiagnosticsPreserveHTTPTimeout(t *testing.T) {
	// Arrange: cleanup server blocks until its request context is interrupted.
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	var observed []CancellationDiagnostic
	client := mustClient(
		t,
		server.URL,
		WithAllowPrivateIPs(true),
		WithHTTPSettings(httptool.ClientSettings{Timeout: 50 * time.Millisecond, TLSConfig: nil}),
		WithCancellationObserver(func(_ context.Context, d CancellationDiagnostic) { observed = append(observed, d) }),
	)
	parent, cancel := context.WithCancel(t.Context())
	cancel()
	// Act: positive HTTP timeout expires inside the fresh cleanup deadline.
	client.cancelInterruptedTask(parent, toolsy.NewRunEnv(nil), "task")
	// Assert: request timeout is separate from the original parent cancellation.
	require.Len(t, observed, 1)
	require.ErrorIs(t, observed[0].ParentInterrupt, context.Canceled)
	require.ErrorIs(t, observed[0].Cause, context.DeadlineExceeded)
	assert.Equal(t, CancellationRequest, observed[0].Stage)
	assert.False(t, observed[0].Acknowledged)
}
