package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/flowy"
	"github.com/skosovsky/flowy/checkpoint"
	"github.com/skosovsky/flowy/testutil"
	"github.com/skosovsky/guardy"
	"github.com/skosovsky/prompty"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/adapters/execution/filejournal"
	"github.com/skosovsky/toolsy/examples/host_dispatch/recipe"
	integration "github.com/skosovsky/toolsy/examples/host_dispatch_integration"
)

type subject struct {
	ID      string
	Allowed bool
}
type scope struct{ ID string }
type activity struct{ Identity string }
type args struct {
	ID     int64       `json:"id"`
	Amount json.Number `json:"amount"`
	Label  *string     `json:"label,omitempty"`
}
type request = recipe.Request[subject, scope, activity]
type dispatcher = recipe.Dispatcher[subject, scope, activity]
type trackingStore struct {
	toolsy.OperationStore

	mu      sync.Mutex
	binding toolsy.OperationBinding
}

func (s *trackingStore) Claim(ctx context.Context, c toolsy.OperationClaim) (toolsy.ClaimResult, error) {
	s.mu.Lock()
	s.binding = c.Binding
	s.mu.Unlock()
	return s.OperationStore.Claim(ctx, c)
}
func (s *trackingStore) last() toolsy.OperationBinding {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.binding
}

func setup(
	t *testing.T,
	store toolsy.OperationStore,
	calls *atomic.Int32,
	approval bool,
) (*dispatcher, *toolsy.RegistryView) {
	t.Helper()
	p, err := recipe.Profile[subject, scope, activity](
		store,
		toolsy.JSONResultCodec[args, string]{},
		"host",
		time.Now,
		func(s subject, c scope) (string, string, error) { return s.ID, c.ID, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	schemas := toolsy.NewSchemaRegistry()
	schemas.RegisterType(json.Number(""), "number", "")
	options := []toolsy.ToolOption{
		toolsy.WithSchemaRegistry(schemas),
		toolsy.WithCompletionPolicy(toolsy.CompletionContinue),
	}
	if approval {
		options = append(options, toolsy.WithRequiresConfirmation())
	}
	tool, err := toolsy.NewTypedTool(toolsy.TypedToolSpec[subject, scope, args, args, string]{
		Name:        "write",
		Description: "Exact typed write",
		Options:     options,
		Policy: func(_ context.Context, r toolsy.TypedPolicyRequest[subject, scope, args]) toolsy.Decision {
			if !r.Context.Subject.Allowed {
				return toolsy.DenyDecision("not authorized")
			}
			return toolsy.AllowDecision()
		},
		Handler: func(_ context.Context, _ toolsy.TypedCallContext[subject, scope], _ *toolsy.RunEnv, a toolsy.ValidatedArgs[args]) (toolsy.ToolResult[args, string], error) {
			calls.Add(1)
			return toolsy.NewToolResult[args, string](a.Value), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(p)).Add(tool).Build()
	if err != nil {
		t.Fatal(err)
	}
	view, err := reg.View(toolsy.RegistryViewSpec{ToolNames: []string{"write"}, Reason: "consumer", Owner: "host"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := recipe.New[subject, scope, activity](view, nil)
	if err != nil {
		t.Fatal(err)
	}
	return d, view
}
func hostRequest(call prompty.ToolCallPart) (request, error) {
	return request{
		OperationID:           "intent-" + call.ID,
		AttemptID:             "attempt-" + call.ID,
		ActivityIdentity:      activity{Identity: "activity-" + call.ID},
		Subject:               subject{ID: "user", Allowed: true},
		Scope:                 scope{ID: "tenant"},
		PolicyFingerprint:     "acl",
		DependencyFingerprint: "target",
	}, nil
}

type model struct{ parts []prompty.ContentPart }

func (m model) Execute(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
	return prompty.NewResponse(m.parts), nil
}
func (model) ExecuteStream(context.Context, *prompty.PromptExecution, prompty.StreamMode) *prompty.Stream {
	panic("not used")
}

//nolint:gocognit // Keep native correlation and exact-number assertions in the same end-to-end fixture.
func TestRealNativeCallsSourceSchemaAndExactNumbers(t *testing.T) {
	// Arrange: real prompt schema/scope and typed prepared tools; only provider is offline.
	ctx := context.Background()
	var calls atomic.Int32
	d, _ := setup(t, toolsy.NewMemoryOperationStore(), &calls, false)
	manifests, err := d.Manifests()
	if err != nil {
		t.Fatal(err)
	}
	exec, err := integration.Prompt(manifests)
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"id":9007199254740993,"amount":0.12345678901234567890123456789,"label":null}`
	m := model{
		parts: []prompty.ContentPart{
			prompty.ToolCallPart{ID: "native-A", Name: "write", Args: raw},
			prompty.ToolCallPart{ID: "native-B", Name: "write", Args: raw},
		},
	}
	// Act: host is the only tool execution owner.
	if err = prompty.ValidateExecutionToolScope(exec); err != nil {
		t.Fatal(err)
	}
	response, err := m.Execute(ctx, exec)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := integration.Requests(response, hostRequest)
	if err != nil {
		t.Fatal(err)
	}
	results, err := d.Dispatch(ctx, requests, false)
	// Assert.
	if err != nil || len(results) != 2 || calls.Load() != 2 {
		t.Fatalf("results=%+v calls=%d err=%v", results, calls.Load(), err)
	}
	source, _ := manifests.Manifest("write")
	sourceJSON, err := json.Marshal(source.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if string(exec.Tools[0].Parameters) != string(sourceJSON) {
		t.Fatal("schema reinferred")
	}
	for i, result := range results {
		if result.Model == nil {
			t.Fatalf("no model result: %+v", result)
		}
		part, err := integration.ModelPart(result.Model)
		if err != nil {
			t.Fatal(err)
		}
		if part.ToolCallID != requests[i].CallID || part.IsError {
			t.Fatalf("%+v", part)
		}
		var actual args
		if err := json.Unmarshal(result.Model.Value, &actual); err != nil {
			t.Fatal(err)
		}
		if actual.ID != 9007199254740993 || actual.Amount.String() != "0.12345678901234567890123456789" ||
			actual.Label != nil {
			t.Fatalf("%+v", actual)
		}
	}
}
func TestRealApprovalBarrierAndResume(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	store := toolsy.NewMemoryOperationStore()
	var calls atomic.Int32
	d, _ := setup(t, store, &calls, true)
	response := prompty.NewResponse(
		[]prompty.ContentPart{
			prompty.ToolCallPart{ID: "first", Name: "write", Args: `{"id":1,"amount":1}`},
			prompty.ToolCallPart{ID: "second", Name: "write", Args: `{"id":1,"amount":1}`},
		},
	)
	requests, err := integration.Requests(response, hostRequest)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	results, err := d.Dispatch(ctx, requests, false)
	if err != nil {
		t.Fatal(err)
	}
	var pending *toolsy.PendingApprovalError
	if !errors.As(results[0].Err, &pending) {
		t.Fatal(results)
	}
	now := time.Now()
	if err = store.PutGrant(
		ctx,
		toolsy.ApprovalGrant{
			ID:        "authenticated-grant",
			Issuer:    "host",
			Binding:   pending.Challenge.Binding,
			IssuedAt:  now,
			ExpiresAt: now.Add(time.Minute),
		},
	); err != nil {
		t.Fatal(err)
	}
	r := requests[0]
	r.GrantID = "authenticated-grant"
	r.CallID = "resumed-provider"
	r.AttemptID = "resume-attempt"
	resumed, err := d.OneShot(ctx, r)
	// Assert.
	if err != nil || len(results) != 1 || results[0].Decision != "approval" || resumed.Decision != "continue" ||
		calls.Load() != 1 {
		t.Fatalf("results=%+v resumed=%+v err=%v", results, resumed, err)
	}
}

func TestRealGuardDecisionsAndNoAutomaticRetry(t *testing.T) {
	// Arrange: canonical decisions preserve separate host paths.
	cases := []struct {
		report *guardy.Report
		want   string
	}{
		{&guardy.Report{Action: guardy.ActionPass}, "continue"},
		{&guardy.Report{Action: guardy.ActionBlock, Reason: "SECRET"}, "deny"},
		{&guardy.Report{Action: guardy.ActionRetry, Retryable: true, Feedback: "SECRET"}, "correction"},
		{&guardy.Report{Action: guardy.ActionPass, Disposition: guardy.DispositionSystemFault}, "fault"},
	}
	for _, tc := range cases {
		// Act.
		actual := integration.GuardDecision(guardy.DecisionFromReport(tc.report))
		// Assert: mapping itself never invokes dispatch or leaks report text.
		if actual != tc.want || strings.Contains(actual, "SECRET") {
			t.Fatalf("%s want %s", actual, tc.want)
		}
	}
}

type state struct {
	CallID, OperationID, ActivityID string
	Result                          []byte
}

//nolint:gocognit // Exercise the real dispatch/reconcile state machine in one readable recovery fixture.
func TestRealDurableActivityReconcilesDeliveryLossViaAuthorizedReplay(t *testing.T) {
	// Arrange: real continuation journal plus reopened durable tool journal.
	ctx := context.Background()
	path := t.TempDir() + "/operations.json"
	journal, err := filejournal.Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	tracked := &trackingStore{OperationStore: journal}
	var calls atomic.Int32
	d, _ := setup(t, tracked, &calls, false)
	lost := errors.New("delivery lost after completed outcome")
	graphBuilder := flowy.NewGraph[state, flowy.NoEffect](func(_, update state) state { return update })
	graphBuilder.AddNode("dispatch", func(ctx context.Context, s state) (state, flowy.Directive, error) {
		outcome, activityErr := flowy.CallActivity(ctx, flowy.ActivityRequest{
			Key:            "host-activity",
			Implementation: "typed-host-dispatch",
			Input:          []byte(`{"id":9007199254740993,"amount":1.25}`),
			Dispatch: func(ctx context.Context, inv flowy.ActivityInvocation) ([]byte, error) {
				r, _ := hostRequest(prompty.ToolCallPart{ID: s.CallID})
				r.CallID = s.CallID
				r.Tool = "write"
				r.Args = inv.Input
				r.OperationID = s.OperationID
				r.AttemptID = fmt.Sprintf("attempt-%d", inv.Attempt)
				r.ActivityIdentity = activity{Identity: inv.Identity}
				result, e := d.OneShot(ctx, r)
				if e != nil {
					return nil, e
				}
				if result.Decision != "continue" {
					return nil, result.Err
				}
				return nil, lost
			},
			Reconcile: func(ctx context.Context, record flowy.ActivityRecord) ([]byte, error) {
				reopened, e := filejournal.Open(path, 0)
				if e != nil {
					return nil, e
				}
				binding := tracked.last()
				stored, found, e := reopened.Inspect(ctx, binding)
				if e != nil {
					return nil, e
				}
				if !found || stored.State != toolsy.OperationCompleted {
					return nil, flowy.ErrActivityUnknown
				}
				recovered, _ := setup(t, reopened, &calls, false)
				r, _ := hostRequest(prompty.ToolCallPart{ID: "recovered-provider"})
				r.CallID = "recovered-provider"
				r.Tool = "write"
				r.Args = []byte(`{"id":9007199254740993,"amount":1.25}`)
				r.OperationID = s.OperationID
				r.AttemptID = "recovered-attempt"
				r.ActivityIdentity = activity{Identity: record.Identity}
				result, e := recovered.OneShot(ctx, r)
				if e != nil {
					return nil, e
				}
				if result.Decision != "continue" {
					return nil, result.Err
				}
				return json.Marshal(result.Model)
			},
		})
		if activityErr != nil {
			return s, flowy.Fail("host-dispatch"), activityErr
		}
		s.ActivityID = outcome.Identity
		s.Result = outcome.Payload
		return s, flowy.End(), nil
	}).AllowNoOutgoingRoute("dispatch").SetEntryPoint("dispatch")
	graph, err := graphBuilder.Compile()
	if err != nil {
		t.Fatal(err)
	}
	runner, err := flowy.NewDurableRunner(
		graph,
		testutil.NewMemoryExecutionStore(nil),
		flowy.ExecutionDescriptor{
			GraphID:           "host-recipe",
			GraphRevision:     "recipe",
			StateCodec:        "json",
			EffectsCodec:      "json",
			ExecutionContract: "sync",
			ReplayPolicy:      flowy.StepReplayPolicy{Label: "host-activities", Mode: flowy.StepReplaySafe},
		},
		checkpoint.JSONSerializer[state]{},
		checkpoint.JSONSerializer[[]flowy.NoEffect]{},
		flowy.DurableOptions{Owner: "host", LeaseTTL: time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	first, err := runner.Start(ctx, "run", state{CallID: "provider-original", OperationID: "host-intent"})
	if !errors.Is(err, lost) || first == nil {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := runner.Resume(ctx, first.ResumeToken)
	// Assert: activity reconciliation obtains result through current Session, not raw Inspect bytes.
	if err != nil || second == nil || calls.Load() != 1 {
		t.Fatalf("second=%+v calls=%d err=%v", second, calls.Load(), err)
	}
	var modelResult recipe.ModelResult
	if err := json.Unmarshal(second.State.Result, &modelResult); err != nil {
		t.Fatal(err)
	}
	if modelResult.CallID != "recovered-provider" || second.State.CallID != "provider-original" ||
		second.State.OperationID != "host-intent" ||
		second.State.ActivityID == "" ||
		second.State.ActivityID == second.State.OperationID {
		t.Fatalf("%+v model=%+v", second.State, modelResult)
	}
}
