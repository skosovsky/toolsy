package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/skosovsky/toolsy"
)

func TestPinnedNormativeStatusFixture(t *testing.T) {
	// Arrange: exact upstream OpenAPI fixture, independent of bridge extension table.
	raw, err := os.ReadFile("testdata/agent-protocol-v1.openapi.yml")
	require.NoError(t, err)
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				AllOf []struct {
					Properties map[string]struct {
						Enum []string `yaml:"enum"`
					} `yaml:"properties"`
				} `yaml:"allOf"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	// Act.
	require.NoError(t, yaml.Unmarshal(raw, &spec))
	// Assert: no A2A failure/cancellation vocabulary invented as normative.
	require.Equal(
		t,
		[]string{"created", "running", "completed"},
		spec.Components.Schemas["Step"].AllOf[1].Properties["status"].Enum,
	)
}

func testPolicy() StreamPolicy {
	return StreamPolicy{MaxReconnects: 0, Backoff: 0, Timeout: time.Second, MaxEventBytes: 4096}
}

func stepFrame(id, status string, last bool) string {
	return fmt.Sprintf(
		"id: %s\ndata: {\"artifacts\":[],\"step_id\":\"%s\",\"task_id\":\"t\",\"status\":\"%s\",\"is_last\":%t,\"output\":\"partial\"}\n\n",
		id,
		id,
		status,
		last,
	)
}

func TestDelegateTerminalOutcomes(t *testing.T) {
	var tests []struct {
		Name    string  `json:"name"`
		Status  string  `json:"status"`
		Last    bool    `json:"last"`
		Outcome Outcome `json:"outcome"`
		Success bool    `json:"success"`
	}
	raw, err := os.ReadFile("testdata/toolsy-step-stream-v1.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &tests))
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			// Arrange.
			var creates atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					creates.Add(1)
					_, _ = w.Write([]byte(`{"task_id":"t","artifacts":[]}`))
					return
				}
				_, _ = w.Write([]byte(stepFrame("s", tt.Status, tt.Last)))
			}))
			defer server.Close()
			client := NewClient(server.URL, WithAllowPrivateIPs(true), WithStreamPolicy(testPolicy()))
			tool, err := AsTool("delegate", "delegate", []byte(`{"type":"object"}`), client)
			require.NoError(t, err)
			var chunks []toolsy.Chunk
			// Act.
			err = tool.Execute(
				t.Context(),
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
				func(chunk toolsy.Chunk) error { chunks = append(chunks, chunk); return nil },
			)
			// Assert.
			require.EqualValues(t, 1, creates.Load())
			if tt.Success {
				require.NoError(t, err)
				require.Len(t, chunks, 1)
				require.Equal(t, toolsy.EventResult, chunks[0].Event)
				return
			}
			var remote *RemoteOutcomeError
			require.ErrorAs(t, err, &remote)
			require.Equal(t, tt.Outcome, remote.Outcome)
			require.Equal(t, "t", remote.TaskID)
			for _, chunk := range chunks {
				require.NotEqual(t, toolsy.EventResult, chunk.Event)
			}
		})
	}
}

func TestDelegateEOFAndAcceptedReference(t *testing.T) {
	// Arrange: successful normative creation; stream never confirms completion.
	var creates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			creates.Add(1)
			var request map[string]json.RawMessage
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			assert.JSONEq(t, `{"payload":1}`, string(request["additional_input"]))
			assert.NotContains(t, request, "input")
			_, _ = w.Write([]byte(`{"task_id":"t","artifacts":[]}`))
			return
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, WithAllowPrivateIPs(true), WithStreamPolicy(testPolicy()))
	syncTool, err := AsTool("delegate", "delegate", []byte(`{"type":"object"}`), client)
	require.NoError(t, err)
	// Act/Assert: EOF is typed incomplete and never repeats POST.
	err = syncTool.Execute(
		t.Context(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"payload":1}`)},
		func(toolsy.Chunk) error { t.Fatal("unexpected result"); return nil },
	)
	var outcome *RemoteOutcomeError
	require.ErrorAs(t, err, &outcome)
	require.Equal(t, OutcomeIncomplete, outcome.Outcome)
	require.Equal(t, "observe", outcome.Phase)
	require.EqualValues(t, 1, creates.Load())
	// Arrange/Act: a separate explicitly requested background action.
	background, err := AsBackgroundTool("background", "background", []byte(`{"type":"object"}`), client)
	require.NoError(t, err)
	var reference AcceptedTaskReference
	err = background.Execute(
		t.Context(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"payload":1}`)},
		func(chunk toolsy.Chunk) error { return json.Unmarshal(chunk.Data, &reference) },
	)
	// Assert: acknowledgement DTO and declared output schema, no completed claim.
	require.NoError(t, err)
	require.Equal(t, AcceptedTaskReference{TaskID: "t", Accepted: true}, reference)
	require.NotEmpty(t, background.Manifest().OutputSchema)
	require.EqualValues(t, 2, creates.Load())
}

func TestCreateCollectionUnknown(t *testing.T) {
	for _, body := range []string{`{"task_id":"t"}`, `{"task":{"task_id":"t","artifacts":[]}}`, `invalid`} {
		t.Run(body, func(t *testing.T) {
			// Arrange.
			var creates atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, _ *http.Request) { creates.Add(1); _, _ = w.Write([]byte(body)) },
				),
			)
			defer server.Close()
			client := NewClient(server.URL, WithAllowPrivateIPs(true))
			// Act.
			_, err := client.CreateTask(t.Context(), json.RawMessage(`{}`), "")
			// Assert.
			var outcome *RemoteOutcomeError
			require.ErrorAs(t, err, &outcome)
			require.Equal(t, OutcomeUnknown, outcome.Outcome)
			require.Equal(t, "create", outcome.Phase)
			if body == `{"task_id":"t"}` {
				require.Equal(t, "t", outcome.TaskID)
			}
			require.EqualValues(t, 1, creates.Load())
		})
	}
}

func TestDelegatePostCreationTimeoutPreservesReference(t *testing.T) {
	// Arrange: idle stream starts only after one accepted action.
	var creates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			creates.Add(1)
			_, _ = w.Write([]byte(`{"task_id":"t","artifacts":[]}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	policy := testPolicy()
	policy.Timeout = 30 * time.Millisecond
	tool, err := AsTool(
		"delegate",
		"delegate",
		[]byte(`{"type":"object"}`),
		NewClient(server.URL, WithAllowPrivateIPs(true), WithStreamPolicy(policy)),
	)
	require.NoError(t, err)
	// Act.
	err = tool.Execute(
		t.Context(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	// Assert: timeout remains timeout, but action retry is never suggested.
	require.ErrorIs(t, err, context.DeadlineExceeded)
	var outcome *RemoteOutcomeError
	require.ErrorAs(t, err, &outcome)
	require.Equal(t, "t", outcome.TaskID)
	require.Equal(t, OutcomeUnknown, outcome.Outcome)
	typed, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.False(t, typed.Retryable)
	require.EqualValues(t, 1, creates.Load())
}

func TestDelegateConsumerAbortIsPreserved(t *testing.T) {
	// Arrange.
	abort := errors.New("host stopped observation")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"task_id":"t","artifacts":[]}`))
			return
		}
		_, _ = w.Write([]byte(stepFrame("s", "running", false)))
	}))
	defer server.Close()
	policy := testPolicy()
	policy.MaxReconnects = 2
	tool, err := AsTool(
		"delegate",
		"delegate",
		[]byte(`{"type":"object"}`),
		NewClient(server.URL, WithAllowPrivateIPs(true), WithStreamPolicy(policy)),
	)
	require.NoError(t, err)
	// Act.
	err = tool.Execute(
		t.Context(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return abort },
	)
	// Assert.
	require.ErrorIs(t, err, abort)
	require.EqualValues(t, 2, requests.Load())
}

type timeoutStreamCredentials struct{}

func (timeoutStreamCredentials) GetAuth(_ context.Context, name string) (string, error) {
	if name == streamStepsAuthToolName {
		return "", context.DeadlineExceeded
	}
	return "", nil
}

func TestPostCreationCredentialTimeoutIsNotRetryable(t *testing.T) {
	// Arrange.
	var creates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		creates.Add(1)
		_, _ = w.Write([]byte(`{"task_id":"t","artifacts":[]}`))
	}))
	defer server.Close()
	tool, err := AsTool(
		"delegate",
		"delegate",
		[]byte(`{"type":"object"}`),
		NewClient(server.URL, WithAllowPrivateIPs(true)),
	)
	require.NoError(t, err)
	// Act.
	err = tool.Execute(
		t.Context(),
		toolsy.NewRunEnv(nil, toolsy.WithCredentials(timeoutStreamCredentials{})),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	// Assert.
	require.ErrorIs(t, err, context.DeadlineExceeded)
	var outcome *RemoteOutcomeError
	require.ErrorAs(t, err, &outcome)
	require.Equal(t, "t", outcome.TaskID)
	typed, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.False(t, typed.Retryable)
	require.EqualValues(t, 1, creates.Load())
}

func TestDuplicateWireFieldsCannotFabricateCompletion(t *testing.T) {
	// Arrange: attacker supplies conflicting terminal status and duplicate task reference.
	frames := []string{
		`{"task_id":"t","step_id":"s","artifacts":[],"is_last":true,"status":"failed","Status":"completed"}`,
		`{"task_id":"t","step_id":"s","artifacts":[],"is_last":true,"status":"failed","status":"completed"}`,
		`{"task_id":"other","task_id":"t","step_id":"s","artifacts":[],"is_last":true,"status":"completed"}`,
		`{"task_id":"t","step_id":"s","artifacts":[],"is_last":false,"is_last":true,"status":"completed"}`,
	}
	for _, frame := range frames {
		t.Run(frame, func(t *testing.T) {
			// Act.
			var count int
			_, done, _, err := parseSSESteps(
				t.Context(),
				strings.NewReader("data: "+frame+"\n\n"),
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

func TestPinnedNormativeEnvelopeFixtures(t *testing.T) {
	// Arrange.
	taskRaw, err := os.ReadFile("testdata/normative-task.json")
	require.NoError(t, err)
	stepRaw, err := os.ReadFile("testdata/normative-step.json")
	require.NoError(t, err)
	var task Task
	var step Step
	// Act.
	require.NoError(t, json.Unmarshal(taskRaw, &task))
	require.NoError(t, json.Unmarshal(stepRaw, &step))
	// Assert.
	_, err = validateCreatedTask(&task)
	require.NoError(t, err)
	require.NoError(t, validateStep(step))
	require.True(t, step.IsLast)
	require.Equal(t, "completed", step.Status)
}

func TestInvalidAdditionalInputHasNoRemoteOutcome(t *testing.T) {
	// Arrange.
	var creates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { creates.Add(1) }))
	defer server.Close()
	client := NewClient(server.URL, WithAllowPrivateIPs(true))
	for _, input := range []string{`"prompt"`, `null`, `{"x":1,"x":2}`} {
		t.Run(input, func(t *testing.T) {
			// Act.
			_, err := client.CreateTask(t.Context(), json.RawMessage(input), "")
			// Assert.
			require.ErrorIs(t, err, toolsy.ErrValidation)
			var remote *RemoteOutcomeError
			require.NotErrorAs(t, err, &remote)
			require.Zero(t, creates.Load())
		})
	}
}
