package recipe_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/adapters/execution/filejournal"
	"github.com/skosovsky/toolsy/examples/host_dispatch/recipe"
)

type subject struct {
	ID      string
	Allowed bool
}
type scope struct{ ID string }
type activity struct{ Key string }
type args struct {
	ID   int64   `json:"id"`
	Note *string `json:"note,omitempty"`
}
type request = recipe.Request[subject, scope, activity]
type dispatcher = recipe.Dispatcher[subject, scope, activity]

type recordingStore struct {
	toolsy.OperationStore

	mu      sync.Mutex
	binding toolsy.OperationBinding
}

func (s *recordingStore) Claim(ctx context.Context, c toolsy.OperationClaim) (toolsy.ClaimResult, error) {
	s.mu.Lock()
	s.binding = c.Binding
	s.mu.Unlock()
	return s.OperationStore.Claim(ctx, c)
}
func (s *recordingStore) last() toolsy.OperationBinding {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.binding
}

type fixture struct {
	d                                    *dispatcher
	view                                 *toolsy.RegistryView
	store                                *recordingStore
	issuer                               toolsy.ApprovalIssuerStore
	calls                                atomic.Int32
	effects                              atomic.Int32
	binder                               atomic.Int32
	ids                                  []string
	mu                                   sync.Mutex
	mode                                 string
	now                                  time.Time
	boundAttachments, handlerAttachments []toolsy.Attachment
}

func newFixture(
	t *testing.T,
	mode string,
	store toolsy.OperationStore,
	issuer toolsy.ApprovalIssuerStore,
	profiled bool,
) *fixture {
	t.Helper()
	return newVariantFixture(t, mode, store, issuer, profiled, "")
}

//nolint:gocognit,gocyclo,cyclop // Keep contract variants in one fixture to expose the changed binding.
func newVariantFixture(
	t *testing.T,
	mode string,
	store toolsy.OperationStore,
	issuer toolsy.ApprovalIssuerStore,
	profiled bool,
	variant string,
) *fixture {
	t.Helper()
	f := &fixture{mode: mode, now: time.Now()}
	if store == nil {
		memory := toolsy.NewMemoryOperationStore()
		store, issuer = memory, memory
	}
	f.store = &recordingStore{OperationStore: store}
	f.issuer = issuer
	name, reason := "write", "test"
	if variant == "tool" {
		name = "write_alternative"
	}
	if variant == "view" {
		reason = "different-view"
	}
	var options []toolsy.ToolOption
	if variant == "manifest" {
		options = append(options, toolsy.WithTags("changed"))
	}
	if variant == "schema" {
		options = append(
			options,
			toolsy.WithOutputSchema(
				map[string]any{
					"type":       "object",
					"properties": map[string]any{"id": map[string]any{"type": "integer", "minimum": 0}},
					"required":   []string{"id"},
				},
			),
		)
	}
	if strings.HasPrefix(mode, "approval") {
		options = append(options, toolsy.WithRequiresConfirmation())
	}
	switch mode {
	case "completion_halt":
		options = append(options, toolsy.WithCompletionPolicy(toolsy.CompletionHalt))
	case "completion_yield":
		options = append(options, toolsy.WithCompletionPolicy(toolsy.CompletionSilentYield))
	}
	tool, err := toolsy.NewTypedTool(toolsy.TypedToolSpec[subject, scope, args, args, string]{
		Name:        name,
		Description: "Write exact arguments",
		Options:     options,
		ArgsBinder: func(_ context.Context, r toolsy.ArgsBindRequest) (toolsy.ValidatedArgs[args], error) {
			f.binder.Add(1)
			f.mu.Lock()
			f.boundAttachments = r.Input.Clone().Attachments
			f.mu.Unlock()
			var a args
			if err := json.Unmarshal(r.Input.ArgsJSON, &a); err != nil {
				return toolsy.ValidatedArgs[args]{}, toolsy.NewJSONParseError(err)
			}
			if a.ID == 0 {
				a.ID = 7
			}
			raw, err := json.Marshal(a)
			return toolsy.ValidatedArgs[args]{Value: a, Raw: raw}, err
		},
		Policy: func(_ context.Context, r toolsy.TypedPolicyRequest[subject, scope, args]) toolsy.Decision {
			if !r.Context.Subject.Allowed {
				return toolsy.DenyDecision("SECRET ACL")
			}
			return toolsy.AllowDecision()
		},
		Handler: func(_ context.Context, c toolsy.TypedCallContext[subject, scope], env *toolsy.RunEnv, a toolsy.ValidatedArgs[args]) (toolsy.ToolResult[args, string], error) {
			f.calls.Add(1)
			f.mu.Lock()
			f.handlerAttachments = env.Attachments()
			f.mu.Unlock()
			f.mu.Lock()
			f.ids = append(f.ids, c.Metadata.CallID)
			f.mu.Unlock()
			switch mode {
			case "business":
				return toolsy.ToolResult[args, string]{}, &toolsy.ToolError{
					Code:   toolsy.CodeRemoteExecution,
					Reason: "SECRET business",
				}
			case "correction":
				return toolsy.ToolResult[args, string]{}, toolsy.NewValidationError("SECRET correction")
			case "unknown":
				return toolsy.ToolResult[args, string]{}, errors.New("SECRET after effect")
			}
			r := toolsy.NewToolResult[args, string](a.Value)
			r.Effects = []string{"fresh"}
			switch mode {
			case "text":
				r.Raw = []byte("visible text")
				r.RawMimeType = toolsy.MimeTypeText
			case "internal":
				r.Audience = toolsy.AudienceInternal
			case "user":
				r.Audience = toolsy.AudienceUser
			case "empty":
				r = toolsy.NewEmptyToolResult[args, string]()
			case "noop":
				r = toolsy.NewNoopToolResult[args, string]()
			}
			switch mode {
			case "pause":
				r.Controls = []toolsy.ControlSignal{&toolsy.PauseSignal{Reason: "SECRET pause"}}
			case "yield":
				r.Controls = []toolsy.ControlSignal{&toolsy.YieldSignal{Result: "SECRET yield"}}
			case "halt":
				r.Controls = []toolsy.ControlSignal{&toolsy.HaltSignal{Reason: "SECRET halt"}}
			}
			return r, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := toolsy.NewTool[args, args](
		"hidden",
		"hidden",
		func(_ context.Context, _ *toolsy.RunEnv, a args) (args, error) { f.calls.Add(1); return a, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	var registryOptions []toolsy.RegistryOption
	if profiled {
		p, profileErr := recipe.Profile[subject, scope, activity](
			f.store,
			toolsy.JSONResultCodec[args, string]{},
			"host",
			func() time.Time { return f.now },
			func(s subject, c scope) (string, string, error) { return s.ID, c.ID, nil },
		)
		if profileErr != nil {
			t.Fatal(profileErr)
		}
		registryOptions = append(registryOptions, toolsy.WithExecutionProfile(p))
		// Protected profiles cannot include unprepared raw tools.
		hidden, err = toolsy.NewTypedTool(
			toolsy.TypedToolSpec[subject, scope, args, args, string]{
				Name:        "hidden",
				Description: "Hidden",
				Handler: func(_ context.Context, _ toolsy.TypedCallContext[subject, scope], _ *toolsy.RunEnv, a toolsy.ValidatedArgs[args]) (toolsy.ToolResult[args, string], error) {
					f.calls.Add(1)
					return toolsy.NewToolResult[args, string](a.Value), nil
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	reg, err := toolsy.NewRegistryBuilder(registryOptions...).Add(tool, hidden).Build()
	if err != nil {
		t.Fatal(err)
	}
	f.view, err = reg.View(toolsy.RegistryViewSpec{ToolNames: []string{name}, Owner: "host", Reason: reason})
	if err != nil {
		t.Fatal(err)
	}
	f.d, err = recipe.New[subject, scope, activity](
		f.view,
		func(_ context.Context, e []any) error { f.effects.Add(int32(len(e))); return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func req(id string) request {
	return request{
		CallID:                "call-" + id,
		OperationID:           "op-" + id,
		AttemptID:             "attempt-" + id,
		ActivityIdentity:      activity{Key: "activity-" + id},
		Subject:               subject{ID: "operator", Allowed: true},
		Scope:                 scope{ID: "workspace"},
		Tool:                  "write",
		Args:                  json.RawMessage(`{"id":9007199254740993,"note":null}`),
		PolicyFingerprint:     "acl",
		DependencyFingerprint: "credential",
	}
}
func one(t *testing.T, d *dispatcher, r request) recipe.Result {
	t.Helper()
	out, err := d.OneShot(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func approve(t *testing.T, f *fixture, r request, rejected bool) request {
	t.Helper()
	out := one(t, f.d, r)
	var pending *toolsy.PendingApprovalError
	if out.Decision != "approval" || !errors.As(out.Err, &pending) {
		t.Fatalf("pending: %+v", out)
	}
	var canonical args
	if err := json.Unmarshal(pending.Challenge.DisplayJSON, &canonical); err != nil {
		t.Fatal(err)
	}
	if canonical.ID == 0 {
		t.Fatal("noncanonical approval")
	}
	if err := f.issuer.PutGrant(
		context.Background(),
		toolsy.ApprovalGrant{
			ID:        "grant-" + r.OperationID,
			Issuer:    "host",
			Binding:   pending.Challenge.Binding,
			IssuedAt:  f.now,
			ExpiresAt: f.now.Add(time.Minute),
			Rejected:  rejected,
		},
	); err != nil {
		t.Fatal(err)
	}
	r.GrantID = "grant-" + r.OperationID
	r.CallID += "-resume"
	r.AttemptID += "-resume"
	return r
}

func TestAC1IdentityAndReplay(t *testing.T) {
	// Arrange.
	f := newFixture(t, "success", nil, nil, true)
	a, b := req("a"), req("b")
	// Act.
	out, err := f.d.Dispatch(context.Background(), []request{a, b}, false)
	a.CallID = "new-provider"
	a.AttemptID = "new-attempt"
	replay := one(t, f.d, a)
	// Assert.
	if err != nil || len(out) != 2 || out[0].Model.CallID != "call-a" || out[1].Model.CallID != "call-b" ||
		replay.Model.CallID != "new-provider" ||
		f.calls.Load() != 2 ||
		f.effects.Load() != 2 {
		t.Fatalf("out=%+v err=%v replay=%+v calls=%d effects=%d", out, err, replay, f.calls.Load(), f.effects.Load())
	}
	if string(replay.Model.Value) != `{"id":9007199254740993}` {
		t.Fatal(string(replay.Model.Value))
	}
	if len(f.ids) != 2 || f.ids[0] != "call-a" || f.ids[1] != "call-b" {
		t.Fatal(f.ids)
	}
}
func TestAC1Admission(t *testing.T) {
	for _, kind := range []string{"call", "operation", "attempt", "activity", "missing", "hidden", "json", "concurrent"} {
		t.Run(kind, func(t *testing.T) {
			// Arrange.
			f := newFixture(t, "success", nil, nil, true)
			a, b := req("a"), req("b")
			parallel := false
			switch kind {
			case "call":
				b.CallID = a.CallID
			case "operation":
				b.OperationID = a.OperationID
			case "attempt":
				b.AttemptID = a.AttemptID
			case "activity":
				b.ActivityIdentity = a.ActivityIdentity
			case "missing":
				b.CallID = ""
			case "hidden":
				b.Tool = "hidden"
			case "json":
				b.Args = json.RawMessage(`{`)
			case "concurrent":
				parallel = true
			}
			// Act.
			_, err := f.d.Dispatch(context.Background(), []request{a, b}, parallel)
			// Assert.
			if err == nil || f.calls.Load() != 0 {
				t.Fatalf("%v calls=%d", err, f.calls.Load())
			}
		})
	}
}
func TestAC2Barriers(t *testing.T) {
	for mode, decision := range map[string]string{"pause": "pause", "yield": "yield", "halt": "halt", "completion_halt": "halt", "completion_yield": "yield", "approval": "approval", "unknown": "uncertain"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			f := newFixture(t, mode, nil, nil, true)
			// Act.
			out, err := f.d.Dispatch(context.Background(), []request{req("a"), req("b")}, false)
			// Assert.
			want := int32(1)
			if mode == "approval" {
				want = 0
			}
			if err != nil || len(out) != 1 || out[0].Decision != decision || f.calls.Load() != want ||
				out[0].Model != nil {
				t.Fatalf("%+v %v calls=%d", out, err, f.calls.Load())
			}
		})
	}
}
func TestAC3BusinessCorrectionEmptyNoop(t *testing.T) {
	for mode, decision := range map[string]string{"business": "business_error", "correction": "correction", "empty": "continue", "noop": "continue"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: business errors without a durable successful-result profile.
			f := newFixture(t, mode, nil, nil, false)
			// Act.
			out := one(t, f.d, req("a"))
			// Assert.
			if out.Decision != decision || out.Err != nil || out.Model == nil {
				t.Fatalf("%+v", out)
			}
			if (mode == "business" || mode == "correction") && out.Outcome.ExecutionError == nil {
				t.Fatal("business channel lost")
			}
			raw, err := json.Marshal(out.Model)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "SECRET") {
				t.Fatal(string(raw))
			}
		})
	}
}
func TestAC4AudienceAndProjection(t *testing.T) {
	for _, audience := range []string{"internal", "user"} {
		t.Run(audience, func(t *testing.T) {
			// Arrange.
			f := newFixture(t, audience, nil, nil, true)
			// Act.
			out := one(t, f.d, req("a"))
			// Assert.
			if out.Decision != "continue" || out.Model != nil || len(out.Outcome.Result) == 0 || f.effects.Load() != 1 {
				t.Fatalf("%+v", out)
			}
		})
	}
	// Arrange: safe projection never considers raw, typed, progress or diagnostics from errors.
	for _, audience := range []toolsy.ToolAudience{toolsy.AudienceInternal, toolsy.AudienceUser, toolsy.AudienceModel, "invalid"} {
		o := toolsy.ToolOutcome{
			Envelope:       toolsy.ToolEnvelope{Audience: audience},
			Result:         []byte(`"SECRET"`),
			ExecutionError: toolsy.NewValidationError("SECRET"),
			Progress:       []toolsy.Chunk{{Data: []byte("SECRET")}},
		}
		// Act.
		projection, err := recipe.Project("id", "name", o, "correction")
		// Assert.
		if audience == "invalid" {
			if err == nil {
				t.Fatal("unknown audience allowed")
			}
			continue
		}
		if audience != toolsy.AudienceModel && projection != nil {
			t.Fatal("audience leak")
		}
		raw, e := json.Marshal(projection)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(raw), "SECRET") {
			t.Fatal(string(raw))
		}
	}
}
func TestAC5SchemaAndCanonicalSnapshot(t *testing.T) {
	// Arrange.
	f := newFixture(t, "approval", nil, nil, true)
	r := req("a")
	r.Args = json.RawMessage(`{"id":0,"note":null}`)
	r.Attachments = []toolsy.Attachment{{MimeType: "text/plain", Data: []byte("exact")}}
	// Act.
	manifests, err := f.d.Manifests()
	if err != nil {
		t.Fatal(err)
	}
	manifest, ok := manifests.Manifest("write")
	if !ok {
		t.Fatal("manifest missing")
	}
	r = approve(t, f, r, false)
	out := one(t, f.d, r)
	// Assert.
	if out.Decision != "continue" || f.binder.Load() != 2 || string(out.Model.Value) != `{"id":7}` ||
		len(manifest.Parameters) == 0 ||
		manifests.Has("hidden") ||
		len(f.boundAttachments) != 1 ||
		len(f.handlerAttachments) != 1 ||
		string(f.boundAttachments[0].Data) != "exact" ||
		string(f.handlerAttachments[0].Data) != "exact" ||
		f.handlerAttachments[0].MimeType != "text/plain" {
		t.Fatalf("%+v binder=%d manifest=%+v", out, f.binder.Load(), manifest)
	}
}
func TestAC6BindingEditsAndCurrentAuthorization(t *testing.T) {
	for _, edit := range []string{"args", "attachment", "subject", "scope", "policy", "dependency", "activity", "revoke"} {
		t.Run(edit, func(t *testing.T) {
			// Arrange.
			f := newFixture(t, "approval", nil, nil, true)
			r := approve(t, f, req("a"), false)
			switch edit {
			case "args":
				r.Args = json.RawMessage(`{"id":2}`)
			case "attachment":
				r.Attachments = []toolsy.Attachment{{Data: []byte("changed")}}
			case "subject":
				r.Subject.ID = "other"
			case "scope":
				r.Scope.ID = "other"
			case "policy":
				r.PolicyFingerprint = "other"
			case "dependency":
				r.DependencyFingerprint = "other"
			case "activity":
				r.ActivityIdentity.Key = "other"
			case "revoke":
				r.Subject.Allowed = false
			}
			// Act.
			out := one(t, f.d, r)
			// Assert.
			if out.Decision == "continue" || f.calls.Load() != 0 {
				t.Fatalf("edit=%s out=%+v", edit, out)
			}
		})
	}
	// Arrange.
	f := newFixture(t, "approval", nil, nil, true)
	r := approve(t, f, req("ok"), false)
	// Act.
	success := one(t, f.d, r)
	r.Subject.Allowed = false
	r.AttemptID = "revoked"
	r.CallID = "revoked"
	denied := one(t, f.d, r)
	// Assert.
	if success.Decision != "continue" || denied.Decision != "deny" || f.calls.Load() != 1 || denied.Model != nil {
		t.Fatalf("success=%+v denied=%+v", success, denied)
	}
}
func TestAC6RejectedExpiredAndConcurrentResume(t *testing.T) {
	for _, mode := range []string{"rejected", "expired"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			f := newFixture(t, "approval", nil, nil, true)
			r := approve(t, f, req("a"), mode == "rejected")
			if mode == "expired" {
				f.now = f.now.Add(2 * time.Minute)
			}
			// Act.
			out := one(t, f.d, r)
			// Assert.
			if out.Decision != recipe.Deny || f.calls.Load() != 0 {
				t.Fatalf("%+v", out)
			}
		})
	}
	// Arrange: separate hosts sharing the actual atomic journal.
	f := newFixture(t, "approval", nil, nil, true)
	r := approve(t, f, req("parallel"), false)
	other, err := recipe.New[subject, scope, activity](f.view, nil)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	// Act.
	for i, d := range []*dispatcher{f.d, other} {
		wg.Go(func() {
			copyR := r
			copyR.AttemptID += string(rune('a' + i))
			_, e := d.OneShot(context.Background(), copyR)
			if e != nil {
				t.Error(e)
			}
		})
	}
	wg.Wait()
	// Assert.
	if f.calls.Load() != 1 {
		t.Fatal(f.calls.Load())
	}
}
func TestAC7DurableLostDeliveryReopen(t *testing.T) {
	// Arrange.
	path := t.TempDir() + "/journal.json"
	store, err := filejournal.Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, "success", store, store, true)
	r := req("durable")
	// Act: committed response is discarded, simulating downstream delivery loss.
	committed := one(t, f.d, r)
	binding := f.store.last()
	reopened, err := filejournal.Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	record, found, err := reopened.Inspect(context.Background(), binding)
	if err != nil || !found || record.State != toolsy.OperationCompleted {
		t.Fatalf("%+v %v", record, err)
	}
	before := f.calls.Load()
	r.CallID = "recovered-provider"
	r.AttemptID = "recovered-attempt"
	restored := newFixture(t, "success", reopened, reopened, true)
	replayed := one(t, restored.d, r)
	// Assert.
	if committed.Decision != "continue" || replayed.Decision != "continue" ||
		replayed.Model.CallID != "recovered-provider" ||
		restored.calls.Load() != 0 ||
		restored.effects.Load() != 0 ||
		before != 1 {
		t.Fatalf("replayed=%+v calls=%d", replayed, restored.calls.Load())
	}
}
func TestAC7UnknownNeverBlindlyRedispatches(t *testing.T) {
	// Arrange.
	f := newFixture(t, "unknown", nil, nil, true)
	r := req("a")
	// Act.
	first := one(t, f.d, r)
	binding := f.store.last()
	record, found, err := f.store.Inspect(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	r.CallID = "redelivery"
	r.AttemptID = "redelivery"
	second := one(t, f.d, r)
	// Assert.
	if first.Decision != "uncertain" || second.Decision != "uncertain" || !found ||
		record.State != toolsy.OperationUnknown ||
		f.calls.Load() != 1 {
		t.Fatalf("first=%+v second=%+v record=%+v", first, second, record)
	}
}

func TestAC6ToolManifestViewSchemaChanges(t *testing.T) {
	for _, variant := range []string{"tool", "manifest", "view", "schema"} {
		t.Run(variant, func(t *testing.T) {
			// Arrange: authenticate the old immutable challenge, then change execution binding.
			memory := toolsy.NewMemoryOperationStore()
			old := newFixture(t, "approval", memory, memory, true)
			r := approve(t, old, req("a"), false)
			changed := newVariantFixture(t, "approval", memory, memory, true, variant)
			if variant == "tool" {
				r.Tool = "write_alternative"
			}
			// Act.
			out := one(t, changed.d, r)
			// Assert.
			if out.Decision == "continue" || changed.calls.Load() != 0 || out.Model != nil {
				t.Fatalf("variant=%s out=%+v", variant, out)
			}
		})
	}
}

type retainedClaimStore struct{ toolsy.OperationStore }

func (s retainedClaimStore) Finish(context.Context, toolsy.OperationFinish) error {
	return errors.New("persistence unavailable")
}
func TestAC2InProgressStopsNextEffect(t *testing.T) {
	// Arrange: failed persistence preserves the live claimed record in the real journal.
	memory := toolsy.NewMemoryOperationStore()
	store := retainedClaimStore{OperationStore: memory}
	f := newFixture(t, "success", store, memory, true)
	r := req("busy")
	first := one(t, f.d, r)
	binding := f.store.last()
	record, found, err := memory.Inspect(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	r.CallID = "new-call"
	r.AttemptID = "new-attempt"
	// Act.
	results, err := f.d.Dispatch(context.Background(), []request{r, req("next")}, false)
	// Assert.
	if first.Decision != "uncertain" || !found || record.State != toolsy.OperationInProgress || err != nil ||
		len(results) != 1 ||
		results[0].Decision != "uncertain" ||
		f.calls.Load() != 1 {
		t.Fatalf("first=%+v record=%+v results=%+v err=%v", first, record, results, err)
	}
}

type failCompletedStore struct{ toolsy.OperationStore }

func (s failCompletedStore) Finish(ctx context.Context, f toolsy.OperationFinish) error {
	if f.State == toolsy.OperationCompleted {
		return errors.New("injected completed persistence failure")
	}
	return s.OperationStore.Finish(ctx, f)
}
func TestAC7PersistenceFailureAndTrustedReconciliation(t *testing.T) {
	// Arrange: an external handler effect happened before completed persistence fails.
	ctx := context.Background()
	memory := toolsy.NewMemoryOperationStore()
	f := newFixture(t, "success", failCompletedStore{OperationStore: memory}, memory, true)
	r := req("effect")
	// Act.
	first := one(t, f.d, r)
	binding := f.store.last()
	r.CallID = "redelivery"
	r.AttemptID = "redelivery"
	second := one(t, f.d, r)
	// The trusted host verifies external receipt and resolves completed bytes. No retry permission.
	codec := toolsy.JSONResultCodec[args, string]{}
	value := args{ID: 9007199254740993}
	wire, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := codec.EncodeResult(
		toolsy.Chunk{
			Event:       toolsy.EventResult,
			Data:        wire,
			MimeType:    toolsy.MimeTypeJSON,
			TypedResult: value,
			Effects:     []any{"fresh"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = toolsy.ReconcileOperation(
		ctx,
		memory,
		binding,
		time.Now,
		func(context.Context, toolsy.OperationRecord) (toolsy.ReconciliationDecision, error) {
			return toolsy.ReconciliationDecision{
				State:   toolsy.OperationCompleted,
				Result:  encoded,
				ProofID: "trusted-external-receipt",
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	third := one(t, f.d, r)
	// Assert: completed replay exposes no new dispatch or reducer effect.
	if first.Decision != "uncertain" || second.Decision != "uncertain" || third.Decision != "continue" ||
		f.calls.Load() != 1 ||
		f.effects.Load() != 0 ||
		third.Model.CallID != "redelivery" {
		t.Fatalf(
			"first=%+v second=%+v third=%+v calls=%d effects=%d",
			first,
			second,
			third,
			f.calls.Load(),
			f.effects.Load(),
		)
	}
}

type partialTool struct{ calls *atomic.Int32 }

func (p partialTool) Manifest() toolsy.ToolManifest {
	return toolsy.ToolManifest{Name: "write", Parameters: map[string]any{"type": "object"}}
}

func (p partialTool) Execute(
	_ context.Context,
	_ *toolsy.RunEnv,
	_ toolsy.ToolInput,
	yield func(toolsy.Chunk) error,
) error {
	p.calls.Add(1)
	if err := yield(
		toolsy.Chunk{
			Event:    toolsy.EventProgress,
			Data:     []byte("SECRET progress"),
			MimeType: toolsy.MimeTypeText,
			Envelope: toolsy.NewResultEnvelope(
				nil,
				[]byte("SECRET progress"),
				toolsy.MimeTypeText,
				"",
				toolsy.AudienceInternal,
				nil,
			),
		},
	); err != nil {
		return err
	}
	if err := yield(
		toolsy.Chunk{
			Event:    toolsy.EventResult,
			Data:     []byte("partial text"),
			MimeType: toolsy.MimeTypeText,
			Controls: []toolsy.ControlSignal{&toolsy.PauseSignal{Reason: "partial"}},
			Envelope: toolsy.NewResultEnvelope(
				nil,
				[]byte("partial text"),
				toolsy.MimeTypeText,
				"",
				toolsy.AudienceUser,
				nil,
			),
		},
	); err != nil {
		return err
	}
	return toolsy.ErrHalt
}
func TestAC3PartialOutcomeAndControlError(t *testing.T) {
	// Arrange.
	var calls atomic.Int32
	reg, err := toolsy.NewRegistryBuilder().Add(partialTool{calls: &calls}).Build()
	if err != nil {
		t.Fatal(err)
	}
	view, err := reg.View(toolsy.RegistryViewSpec{ToolNames: []string{"write"}, Reason: "partial", Owner: "host"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := recipe.New[subject, scope, activity](view, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	out, err := d.Dispatch(context.Background(), []request{req("a"), req("b")}, false)
	// Assert.
	if err != nil || len(out) != 1 || out[0].Decision != "halt" || !errors.Is(out[0].Err, toolsy.ErrHalt) ||
		string(out[0].Outcome.Result) != "partial text" ||
		out[0].Outcome.ResultMimeType != toolsy.MimeTypeText ||
		out[0].Outcome.Envelope.Audience != toolsy.AudienceUser ||
		len(out[0].Outcome.Progress) != 1 ||
		len(out[0].Outcome.Controls) != 1 ||
		out[0].Model != nil ||
		calls.Load() != 1 {
		t.Fatalf("%+v %v", out, err)
	}
}
func TestMIMEAndControlPrecedenceRegressions(t *testing.T) {
	// Arrange, Act, Assert: no unknown MIME or arbitrary MIME parameter is model-facing.
	for _, empty := range []bool{false, true} {
		o := toolsy.ToolOutcome{
			Envelope:       toolsy.ToolEnvelope{Audience: toolsy.AudienceModel},
			EmptyResult:    empty,
			ResultMimeType: "application/x-secret",
		}
		if projected, err := recipe.Project("id", "name", o, "continue"); err == nil || projected != nil {
			t.Fatal("unsupported MIME")
		}
	}
	o := toolsy.ToolOutcome{
		Envelope:       toolsy.ToolEnvelope{Audience: toolsy.AudienceModel},
		ResultMimeType: `text/plain; diagnostic="SECRET"`,
	}
	projection, err := recipe.Project("id", "name", o, "business_error")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SECRET") || projection.MIME != toolsy.MimeTypeJSON {
		t.Fatal(string(raw))
	}
	if recipe.Classify(
		toolsy.ToolOutcome{Controls: []toolsy.ControlSignal{&toolsy.HaltSignal{}}},
		errors.New("fault"),
	) != "halt" {
		t.Fatal("fault masked halt")
	}
	if recipe.Classify(
		toolsy.ToolOutcome{Controls: []toolsy.ControlSignal{&toolsy.PauseSignal{}}},
		toolsy.ErrHalt,
	) != "halt" {
		t.Fatal("pause masked halt")
	}
}

func TestAC3TextMIMEProjection(t *testing.T) {
	// Arrange.
	f := newFixture(t, "text", nil, nil, true)
	// Act.
	out := one(t, f.d, req("text"))
	// Assert: preserve host MIME; model text is JSON-quoted once with sanitized media type.
	if out.Decision != recipe.Continue || out.Outcome.ResultMimeType != toolsy.MimeTypeText || out.Model == nil ||
		out.Model.MIME != "text/plain" ||
		string(out.Model.Value) != `"visible text"` {
		t.Fatalf("%+v", out)
	}
}
func TestAC4CacheReplayEffects(t *testing.T) {
	// Arrange: read cache effects are host observations, not a durable write intent.
	var calls, effects atomic.Int32
	cache, err := toolsy.NewResultCache(
		toolsy.NewMemoryResultCacheStore(),
		func(context.Context, toolsy.PreparedCall) (bool, error) { return true, nil },
		func(context.Context, toolsy.PreparedCall) (string, error) { return "host-data-partition", nil },
		toolsy.JSONResultCodec[string, string]{},
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := toolsy.NewTypedTool(
		toolsy.TypedToolSpec[subject, scope, args, string, string]{
			Name:        "write",
			Description: "Read for cache test",
			Options:     []toolsy.ToolOption{toolsy.WithReadOnly()},
			Handler: func(context.Context, toolsy.TypedCallContext[subject, scope], *toolsy.RunEnv, toolsy.ValidatedArgs[args]) (toolsy.ToolResult[string, string], error) {
				calls.Add(1)
				result := toolsy.NewToolResult[string, string]("observed")
				result.Effects = []string{"observation"}
				return result, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(cache)).Add(tool).Build()
	if err != nil {
		t.Fatal(err)
	}
	view, err := reg.View(toolsy.RegistryViewSpec{ToolNames: []string{"write"}, Reason: "cache", Owner: "host"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := recipe.New[subject, scope, activity](
		view,
		func(_ context.Context, e []any) error { effects.Add(int32(len(e))); return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	first := one(t, d, req("first"))
	second := one(t, d, req("second"))
	// Assert.
	if first.Decision != recipe.Continue || second.Decision != recipe.Continue || calls.Load() != 1 ||
		effects.Load() != 1 ||
		second.Outcome.Envelope.Metadata[toolsy.ReplaySourceMetadata] != toolsy.ReplaySourceCache {
		t.Fatalf("first=%+v second=%+v calls=%d effects=%d", first, second, calls.Load(), effects.Load())
	}
}
