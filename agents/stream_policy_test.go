package agents

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

func TestLogicalStreamBudgetAcrossReconnect(t *testing.T) {
	// Arrange: each connection below its old per-connection cap.
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(":" + strings.Repeat("x", 60) + "\n\n"))
	}))
	defer server.Close()
	policy := testPolicy()
	policy.MaxReconnects = 5
	client := NewClient(server.URL, WithAllowPrivateIPs(true), WithStreamPolicy(policy), WithMaxSSEStreamBytes(100))
	var streamErr error
	// Act.
	for _, err := range client.StreamSteps(t.Context(), "t", "") {
		streamErr = err
	}
	// Assert: second read consumes remaining logical budget, no third connection.
	require.ErrorIs(t, streamErr, textprocessor.ErrReadLimitExceeded)
	var outcome *RemoteOutcomeError
	require.ErrorAs(t, streamErr, &outcome)
	require.Equal(t, OutcomeUnknown, outcome.Outcome)
	require.EqualValues(t, 2, requests.Load())
}

func TestResumeDuplicateEvents(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(strconv.FormatBool(conflict), func(t *testing.T) {
			// Arrange: upstream repeats acknowledged ID on resume.
			var requests atomic.Int32
			headers := make(chan string, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				headers <- r.Header.Get("Last-Event-ID")
				if requests.Add(1) == 1 {
					_, _ = w.Write([]byte(stepFrame("s", "running", false)))
					return
				}
				duplicate := stepFrame("s", "running", false)
				if conflict {
					duplicate = stepFrame("s", "completed", true)
				}
				_, _ = w.Write([]byte(duplicate + stepFrame("done", "completed", true)))
			}))
			defer server.Close()
			policy := testPolicy()
			policy.MaxReconnects = 2
			client := NewClient(server.URL, WithAllowPrivateIPs(true), WithStreamPolicy(policy))
			var steps []Step
			var streamErr error
			// Act.
			for step, err := range client.StreamSteps(t.Context(), "t", "") {
				if err != nil {
					streamErr = err
				} else {
					steps = append(steps, step)
				}
			}
			// Assert.
			require.Empty(t, <-headers)
			require.Equal(t, "s", <-headers)
			require.EqualValues(t, 2, requests.Load())
			if conflict {
				var outcome *RemoteOutcomeError
				require.ErrorAs(t, streamErr, &outcome)
				require.Equal(t, OutcomeMalformed, outcome.Outcome)
				require.Len(t, steps, 1)
				return
			}
			require.NoError(t, streamErr)
			require.Len(t, steps, 2)
			require.True(t, steps[1].IsLast)
		})
	}
}

func TestEmptyEventIDResetsResumeCursor(t *testing.T) {
	// Arrange: ID-only empty frame clears prior acknowledged cursor.
	var requests atomic.Int32
	headers := make(chan string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Get("Last-Event-ID")
		switch requests.Add(1) {
		case 1:
			_, _ = w.Write([]byte(stepFrame("s", "running", false)))
		case 2:
			_, _ = w.Write([]byte("id:\n\n"))
		default:
			_, _ = w.Write([]byte(stepFrame("done", "completed", true)))
		}
	}))
	defer server.Close()
	policy := testPolicy()
	policy.MaxReconnects = 2
	client := NewClient(server.URL, WithAllowPrivateIPs(true), WithStreamPolicy(policy))
	// Act.
	for _, err := range client.StreamSteps(t.Context(), "t", "") {
		require.NoError(t, err)
	}
	// Assert.
	require.Empty(t, <-headers)
	require.Equal(t, "s", <-headers)
	require.Empty(t, <-headers)
}

func TestSSEAggregateEventAndUnterminatedFrame(t *testing.T) {
	tests := []struct {
		name, frame string
		cap         int
		wantLimit   bool
	}{
		{"many small lines", strings.Repeat("data: a\n", 100) + "\n", 128, true},
		{"unterminated valid JSON", strings.TrimSuffix(stepFrame("s", "completed", true), "\n\n"), 4096, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange.
			var count int
			// Act.
			_, terminal, _, err := parseSSESteps(
				t.Context(),
				strings.NewReader(tt.frame),
				tt.cap,
				func(Step, error) bool { count++; return true },
			)
			// Assert: no incomplete frame is dispatched, aggregate lines cannot evade event cap.
			require.Zero(t, count)
			require.False(t, terminal)
			if tt.wantLimit {
				require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestStreamConsumerStopsWithoutReconnect(t *testing.T) {
	// Arrange.
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(stepFrame("s", "running", false)))
	}))
	defer server.Close()
	client := NewClient(server.URL, WithAllowPrivateIPs(true))
	// Act.
	for _, err := range client.StreamSteps(t.Context(), "t", "") {
		require.NoError(t, err)
		break
	}
	// Assert.
	require.EqualValues(t, 1, requests.Load())
}

func TestInvalidStreamPolicyFailsBeforeCreation(t *testing.T) {
	// Arrange.
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	policy := testPolicy()
	policy.MaxEventBytes = math.MaxInt
	client := NewClient(server.URL, WithAllowPrivateIPs(true), WithStreamPolicy(policy))
	// Act.
	_, err := client.CreateTask(t.Context(), []byte(`{}`), "")
	// Assert.
	require.Error(t, err)
	require.Zero(t, requests.Load())
}

func TestBackoffCancellationDoesNotReconnect(t *testing.T) {
	// Arrange: cancellation triggered by first delivered progress observation.
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(stepFrame("s", "running", false)))
	}))
	defer server.Close()
	policy := testPolicy()
	policy.MaxReconnects = 2
	policy.Backoff = time.Hour
	client := NewClient(server.URL, WithAllowPrivateIPs(true), WithStreamPolicy(policy))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var streamErr error
	// Act.
	for _, err := range client.StreamSteps(ctx, "t", "") {
		if err != nil {
			streamErr = err
		} else {
			cancel()
		}
	}
	// Assert.
	require.ErrorIs(t, streamErr, context.Canceled)
	require.EqualValues(t, 1, requests.Load())
}

func TestCRLFPhysicalEventLimit(t *testing.T) {
	// Arrange: actual bytes exceed cap; LF-normalized frame would be below it.
	frame := strings.Repeat(":\r\n", 30) + "\r\n"
	// Act.
	_, _, _, err := parseSSESteps(t.Context(), strings.NewReader(frame), 70, func(Step, error) bool { return true })
	// Assert.
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestInvalidUTF8SSEFieldsCannotConfirmCompletion(t *testing.T) {
	for _, field := range []string{"id: ", ":", "event: "} {
		t.Run(field, func(t *testing.T) {
			// Arrange: invalid metadata/comment before otherwise valid completed data.
			frame := field + string([]byte{0xff}) + "\n" + stepFrame("s", "completed", true)
			var count int
			// Act.
			_, done, _, err := parseSSESteps(
				t.Context(),
				strings.NewReader(frame),
				4096,
				func(Step, error) bool { count++; return true },
			)
			// Assert.
			var outcome *RemoteOutcomeError
			require.ErrorAs(t, err, &outcome)
			require.Equal(t, OutcomeMalformed, outcome.Outcome)
			require.False(t, done)
			require.Zero(t, count)
		})
	}
}
