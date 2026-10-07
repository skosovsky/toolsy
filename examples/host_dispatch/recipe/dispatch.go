// Package recipe demonstrates a host-owned sequential dispatch boundary.
// It is example application code, not a scheduler API in the tool engine.
package recipe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"sync"

	"github.com/skosovsky/toolsy"
)

// Host decisions determine whether another sequential invocation is permitted.
const (
	Continue      = "continue"
	Fault         = "fault"
	Correction    = "correction"
	Deny          = "deny"
	BusinessError = "business_error"
	Uncertain     = "uncertain"
	Approval      = "approval"
	Halt          = "halt"
	Pause         = "pause"
	Yield         = "yield"
	HostEvent     = "host_event"
	DeliveryFault = "delivery_fault"
)

// Request separates provider correlation from trusted host execution identity.
type Request[S, C any, A comparable] struct {
	CallID, OperationID, AttemptID, Tool              string
	ActivityIdentity                                  A
	Subject                                           S
	Scope                                             C
	Args                                              json.RawMessage
	Attachments                                       []toolsy.Attachment
	GrantID, PolicyFingerprint, DependencyFingerprint string
}

type requestKey[S, C any, A comparable] struct{}

// CurrentRequest is for the host's operation preparation callback only.
func CurrentRequest[S, C any, A comparable](ctx context.Context) (Request[S, C, A], bool) {
	r, ok := ctx.Value(requestKey[S, C, A]{}).(Request[S, C, A])
	r.Args = append(json.RawMessage(nil), r.Args...)
	r.Attachments = (toolsy.ToolInput{Attachments: r.Attachments}).Clone().Attachments
	return r, ok
}

// ModelResult is the only object serialized to model history.
type ModelResult struct {
	CallID  string          `json:"call_id"`
	Tool    string          `json:"tool"`
	MIME    string          `json:"mime"`
	Value   json.RawMessage `json:"value"`
	IsError bool            `json:"is_error"`
}

// Result keeps host-only data outside the model projection.
type Result struct {
	CallID      string
	OperationID string
	Decision    string
	Outcome     toolsy.ToolOutcome
	Err         error
	Model       *ModelResult
}

// Dispatcher has one local sequential owner. The journal handles cross-host claims.
type Dispatcher[S, C any, A comparable] struct {
	view    *toolsy.RegistryView
	session *toolsy.Session
	gate    sync.Mutex
	reduce  func(context.Context, []any) error
}

// New uses the same view for manifests and execution, preserving Session budgets.
func New[S, C any, A comparable](
	view *toolsy.RegistryView,
	reduce func(context.Context, []any) error,
	options ...toolsy.SessionOption,
) (*Dispatcher[S, C, A], error) {
	if view == nil {
		return nil, errors.New("host dispatch: view required")
	}
	session, err := view.NewSession(options...)
	if err != nil {
		return nil, err
	}
	return &Dispatcher[S, C, A]{view: view, session: session, reduce: reduce}, nil
}

// Manifests returns source schemas from the execution view without inference.
func (d *Dispatcher[S, C, A]) Manifests() (toolsy.ManifestSet, error) { return d.view.ManifestSet() }

// OneShot invokes one request and leaves continuation decisions to its caller.
func (d *Dispatcher[S, C, A]) OneShot(ctx context.Context, r Request[S, C, A]) (Result, error) {
	results, err := d.Dispatch(ctx, []Request[S, C, A]{r}, false)
	if len(results) == 0 {
		return Result{}, err
	}
	return results[0], err
}

// Dispatch preadmits identities and membership, then stops at the first barrier.
func (d *Dispatcher[S, C, A]) Dispatch(
	ctx context.Context,
	requests []Request[S, C, A],
	concurrent bool,
) ([]Result, error) {
	if concurrent || !d.gate.TryLock() {
		return nil, errors.New("host dispatch: concurrent batch unsupported")
	}
	defer d.gate.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owned := make([]Request[S, C, A], len(requests))
	for i, r := range requests {
		r.Args = append(json.RawMessage(nil), r.Args...)
		r.Attachments = (toolsy.ToolInput{Attachments: r.Attachments}).Clone().Attachments
		owned[i] = r
	}
	if err := d.admit(owned); err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(owned))
	for _, r := range owned {
		result := d.run(ctx, r)
		results = append(results, result)
		if result.Decision != Continue {
			break
		}
	}
	return results, nil
}

func (d *Dispatcher[S, C, A]) admit(requests []Request[S, C, A]) error {
	manifests, err := d.Manifests()
	if err != nil {
		return err
	}
	calls, ops, attempts := map[string]bool{}, map[string]bool{}, map[string]bool{}
	activities := map[A]bool{}
	var zero A
	for _, r := range requests {
		if r.CallID == "" || r.OperationID == "" || r.AttemptID == "" || r.ActivityIdentity == zero ||
			r.PolicyFingerprint == "" ||
			r.DependencyFingerprint == "" {
			return errors.New("host dispatch: trusted identities and fingerprints required")
		}
		if calls[r.CallID] || ops[r.OperationID] || attempts[r.AttemptID] || activities[r.ActivityIdentity] {
			return errors.New("host dispatch: duplicate batch identity")
		}
		if !manifests.Has(r.Tool) {
			return errors.New("host dispatch: tool outside view")
		}
		if !json.Valid(r.Args) {
			return errors.New("host dispatch: invalid raw JSON")
		}
		calls[r.CallID], ops[r.OperationID], attempts[r.AttemptID], activities[r.ActivityIdentity] = true, true, true, true
	}
	return nil
}

func (d *Dispatcher[S, C, A]) run(ctx context.Context, r Request[S, C, A]) Result {
	ctx = context.WithValue(ctx, requestKey[S, C, A]{}, r)
	outcome, err := d.session.RunCall(ctx, toolsy.ToolCall{
		ToolName: r.Tool, Input: toolsy.ToolInput{CallID: r.CallID, ArgsJSON: r.Args, Attachments: r.Attachments},
		CallContext: toolsy.NewCallContext(r.Subject, r.Scope), Env: toolsy.NewRunEnv(d.session),
	})
	result := Result{
		CallID:      r.CallID,
		OperationID: r.OperationID,
		Outcome:     outcome,
		Err:         err,
		Decision:    Classify(outcome, err),
	}
	if provenance, exists := outcome.Envelope.Metadata["toolsy.replay_source"]; exists {
		if provenance != "result_cache" && provenance != "completed_operation" {
			result.Decision = Fault
			result.Err = errors.New("host dispatch: invalid replay provenance")
			return result
		}
	} else if len(outcome.Effects) > 0 && d.reduce != nil {
		if reduceErr := d.reduce(ctx, outcome.Effects); reduceErr != nil {
			result.Decision = Fault
			result.Err = errors.Join(err, reduceErr)
			return result
		}
	}
	switch result.Decision {
	case Continue, Deny, Correction, BusinessError:
		if result.Decision != Continue && outcome.Envelope.Audience == "" {
			return result
		}
		result.Model, err = Project(r.CallID, r.Tool, outcome, result.Decision)
		if err != nil {
			result.Decision = DeliveryFault
			result.Err = errors.Join(result.Err, err)
		}
	}
	return result
}

// Classify gives uncertainty precedence over diagnostic control causes.
func Classify(o toolsy.ToolOutcome, err error) string {
	var business error
	if o.ExecutionError != nil {
		business = o.ExecutionError
	}
	if decision := journalDecision(err, business); decision != "" {
		return decision
	}
	if control := controlDecision(o.Controls, err); control != "" {
		return control
	}
	if decision := operationDecision(err, business); decision != "" {
		return decision
	}
	if err != nil && !toolsy.IsControlError(err) {
		if te, ok := toolsy.AsToolError(err); ok {
			switch te.Code {
			case toolsy.CodePolicyDenied, toolsy.CodeCapabilityDenied:
				return Deny
			case toolsy.CodeSchemaInvalid, toolsy.CodeValidationFailed:
				return Correction
			default:
				return Fault
			}
		}
		return Fault
	}
	if o.ExecutionError != nil {
		switch o.ExecutionError.Code {
		case toolsy.CodePolicyDenied, toolsy.CodeCapabilityDenied:
			return Deny
		case toolsy.CodeSchemaInvalid, toolsy.CodeValidationFailed:
			return Correction
		case toolsy.CodeInternal:
			return Fault
		default:
			return BusinessError
		}
	}
	switch o.Status {
	case toolsy.OutcomeSuccess, toolsy.OutcomeEmptySuccess, toolsy.OutcomeNoopSuccess:
	default:
		return Fault
	}
	switch o.CompletionPolicy {
	case "", toolsy.CompletionContinue:
		return Continue
	case toolsy.CompletionSilentYield:
		return Yield
	case toolsy.CompletionHalt:
		return Halt
	default:
		return Fault
	}
}

// Project never serializes diagnostics, controls, progress or host-typed objects.
func Project(id, name string, o toolsy.ToolOutcome, decision string) (*ModelResult, error) {
	switch o.Envelope.Audience {
	case toolsy.AudienceInternal, toolsy.AudienceUser:
		return nil, nil //nolint:nilnil // Absent projection is intentional for non-model audiences.
	case toolsy.AudienceModel:
	default:
		return nil, errors.New("host dispatch: unsupported audience")
	}
	var raw json.RawMessage
	isError := decision != Continue
	media := toolsy.MimeTypeJSON
	if o.ResultMimeType != "" {
		parsed, _, err := mime.ParseMediaType(o.ResultMimeType)
		if err != nil {
			return nil, err
		}
		switch parsed {
		case toolsy.MimeTypeJSON, "text/plain", toolsy.MimeTypeToolErrorJSON:
			media = parsed
		default:
			return nil, fmt.Errorf("host dispatch: unsupported model MIME %q", parsed)
		}
	} else if !isError && !o.EmptyResult && !o.Noop {
		return nil, errors.New("host dispatch: missing result MIME")
	}
	switch {
	case isError:
		raw, _ = json.Marshal(map[string]string{"status": decision})
		media = toolsy.MimeTypeJSON
	case o.EmptyResult || o.Noop:
		raw = json.RawMessage(`null`)
		media = toolsy.MimeTypeJSON
	default:
		switch media {
		case toolsy.MimeTypeJSON:
			if !json.Valid(o.Result) {
				return nil, errors.New("host dispatch: invalid result JSON")
			}
			raw = append(json.RawMessage(nil), o.Result...)
		case "text/plain":
			raw, _ = json.Marshal(string(o.Result))
		default:
			return nil, errors.New("host dispatch: error MIME on successful result")
		}
	}
	return &ModelResult{CallID: id, Tool: name, MIME: media, Value: raw, IsError: isError}, nil
}

func controlDecision(controls []toolsy.ControlSignal, err error) string {
	halt, pause, yield, event := false, false, false, false
	for _, control := range controls {
		switch control.(type) {
		case *toolsy.HaltSignal:
			halt = true
		case *toolsy.PauseSignal:
			pause = true
		case *toolsy.YieldSignal:
			yield = true
		case *toolsy.HostEventSignal:
			event = true
		default:
			return Fault
		}
	}
	if toolsy.IsControlError(err) {
		halt = halt || errors.Is(err, toolsy.ErrHalt)
		pause = pause || errors.Is(err, toolsy.ErrPause)
		yield = yield || errors.Is(err, toolsy.ErrYield)
		event = event || errors.Is(err, toolsy.ErrHostEvent)
	}
	switch {
	case halt:
		return Halt
	case pause:
		return Pause
	case yield:
		return Yield
	case event:
		return HostEvent
	}
	return ""
}

func journalDecision(err, business error) string {
	var unknown *toolsy.OperationOutcomeError
	var state *toolsy.OperationStateError
	var pending *toolsy.PendingApprovalError
	if errors.As(err, &unknown) || errors.As(business, &unknown) {
		return Uncertain
	}
	if errors.As(err, &state) || errors.As(business, &state) {
		switch state.Record.State {
		case toolsy.OperationInProgress, toolsy.OperationUnknown:
			return Uncertain
		case toolsy.OperationRejected:
			return Deny
		default:
			return Fault
		}
	}
	if errors.As(err, &pending) || errors.As(business, &pending) {
		return Approval
	}
	return ""
}

func operationDecision(err, business error) string {
	var operation *toolsy.OperationError
	if errors.As(err, &operation) || errors.As(business, &operation) {
		switch operation.Kind {
		case "binding_mismatch", "approval_rejected", "approval_expired", "approval_consumed":
			return Deny
		default:
			return Fault
		}
	}
	return ""
}
