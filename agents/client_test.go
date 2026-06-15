package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

func TestCreateTask_Accepts202Accepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"task_id":"task-1","status":"pending"}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, WithHTTPClient(srv.Client()), WithAllowPrivateIPs(true))
	task, err := client.CreateTask(context.Background(), json.RawMessage(`{"q":"x"}`), "")
	require.NoError(t, err)
	require.Equal(t, "task-1", task.TaskID)
}

func TestCancelTask_Accepts204NoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, WithHTTPClient(srv.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, client.CancelTask(context.Background(), "task-1", ""))
}

func TestCreateTask_Non2xxStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, WithHTTPClient(srv.Client()), WithAllowPrivateIPs(true))
	_, err := client.CreateTask(context.Background(), json.RawMessage(`{"q":"x"}`), "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}

func TestCancelTask_Non2xxStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, WithHTTPClient(srv.Client()), WithAllowPrivateIPs(true))
	err := client.CancelTask(context.Background(), "task-1", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "400")
}

func TestCreateTask_ExceedsResponseLimit(t *testing.T) {
	largeBody := strings.Repeat("x", 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeBody))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(
		srv.URL,
		WithHTTPClient(srv.Client()),
		WithAllowPrivateIPs(true),
		WithMaxResponseBody(20),
	)
	_, err := client.CreateTask(context.Background(), json.RawMessage(`{"q":"x"}`), "")
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "20")
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestStreamStepsOnce_Non2xxStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, WithHTTPClient(srv.Client()), WithAllowPrivateIPs(true))
	_, _, _, err := client.streamStepsOnce(
		context.Background(),
		"task-1",
		"",
		"",
		func(Step, error) bool { return true },
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}

func TestCreateTask_CancelOverResponseLimit(t *testing.T) {
	t.Parallel()
	const responseCap = 64
	largeBody := strings.Repeat("z", responseCap+100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/tasks") {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(largeBody))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(
		srv.URL,
		WithHTTPClient(srv.Client()),
		WithAllowPrivateIPs(true),
		WithMaxResponseBody(responseCap),
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.CreateTask(ctx, json.RawMessage(`{}`), "")
	require.ErrorIs(t, err, context.Canceled)
}

func TestMapCreateTaskReadError_InterruptInChainOverReadLimit(t *testing.T) {
	t.Parallel()
	composite := fmt.Errorf(
		"read: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	err := mapCreateTaskReadError(context.Background(), composite, 1024)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}
