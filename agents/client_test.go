package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
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
