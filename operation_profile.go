package toolsy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// OperationIntent comes from an authenticated host, never model arguments.
// Digest must bind canonical arguments AND attachments, including secret identity
// changes. DisplayJSON is a bounded redacted description of that same snapshot.
type OperationIntent struct {
	Namespace, Scope, Subject, OperationID             string
	PolicyFingerprint, CanonicalDigest, CanonicalRules string
	DownstreamKey, AttemptID, GrantID                  string
	DisplayJSON                                        json.RawMessage
}

type PrepareOperation func(context.Context, PreparedCall) (OperationIntent, error)

// ApprovalChallenge is a host continuation request, not an approval credential.
type ApprovalChallenge struct {
	Binding     OperationBinding `json:"binding"`
	DisplayJSON json.RawMessage  `json:"display"`
}

type PendingApprovalError struct{ Challenge ApprovalChallenge }

func (e *PendingApprovalError) Error() string { return "toolsy: approval_required" }
func (e *PendingApprovalError) Unwrap() error { return ErrPause }

// OperationOutcomeError preserves the cause while explicitly forbidding blind
// retry after claim. Dispatch may have occurred; the journal decides recovery.
type OperationOutcomeError struct {
	Cause     error
	Binding   OperationBinding
	AttemptID string
	// DispatchInvoked records whether this profile called the dispatch continuation.
	// It does not prove an external effect or authorize rolling back a claim.
	DispatchInvoked bool
}

func (e *OperationOutcomeError) Error() string {
	return "toolsy: unknown_outcome after claim; dispatch may have occurred"
}
func (e *OperationOutcomeError) Unwrap() error { return e.Cause }

// OperationStateError gives the host a bound continuation reference, not a grant
// or dispatch permission. Resume must still use the current authorized executor.
type OperationStateError struct{ Record OperationRecord }

func (e *OperationStateError) Error() string { return "toolsy: operation " + string(e.Record.State) }
func (e *OperationStateError) Unwrap() error { return &OperationError{Kind: string(e.Record.State)} }

const operationCleanupTimeout = 5 * time.Second

// OperationProfile couples atomic approval and operation claims to the prepared
// boundary. It buffers one complete terminal result until producer success and
// durable persistence. It never retries handlers or treats cancellation as rollback.
type OperationProfile struct {
	store    OperationStore
	prepare  PrepareOperation
	codec    ResultCodec
	issuer   string
	clock    func() time.Time
	lease    time.Duration
	maxBytes int
}

// ErrOperationProfileConfiguration identifies invalid operation-profile setup.
var ErrOperationProfileConfiguration = errors.New("toolsy: invalid operation profile configuration")

// OperationProfileConfig names host ports and bounds for the prepared dispatch gate.
// Config fields are captured by value; host-owned referenced ports and callbacks
// must remain valid and support concurrent calls. Toolsy does not close them.
type OperationProfileConfig struct {
	Store   OperationStore
	Prepare PrepareOperation
	Codec   ResultCodec
	Issuer  string
	Clock   func() time.Time
	Lease   time.Duration
	// MaxBytes bounds both display JSON and encoded terminal result. Zero defaults
	// to 1 MiB; negative values fail construction. A positive value is inclusive.
	MaxBytes int
}

// NewOperationProfile validates and captures a named configuration without calling
// host ports. The profile never schedules retries or replaces a consumed grant.
func NewOperationProfile(config OperationProfileConfig) (*OperationProfile, error) {
	if err := validateOperationProfileConfig(config); err != nil {
		return nil, err
	}
	if config.MaxBytes == 0 {
		config.MaxBytes = defaultCacheResultLimit
	}
	return &OperationProfile{
		store: config.Store, prepare: config.Prepare, codec: config.Codec,
		issuer: config.Issuer, clock: config.Clock, lease: config.Lease, maxBytes: config.MaxBytes,
	}, nil
}

func validateOperationProfileConfig(config OperationProfileConfig) error {
	var reason string
	switch {
	case isNilValue(config.Store):
		reason = "store is nil"
	case config.Prepare == nil:
		reason = "host preparation is nil"
	case isNilValue(config.Codec):
		reason = "result codec is nil"
	case config.Issuer == "":
		reason = "issuer is empty"
	case config.Clock == nil:
		reason = "clock is nil"
	case config.Lease <= 0:
		reason = "lease must be positive"
	case config.MaxBytes < 0:
		reason = "max bytes must not be negative"
	default:
		return nil
	}
	return fmt.Errorf("%w: %s", ErrOperationProfileConfiguration, reason)
}

func (p *OperationProfile) binding(call PreparedCall, intent OperationIntent) (OperationBinding, error) {
	h := sha256.New()
	if err := writeManifestDigest(h, call.Manifest); err != nil {
		return OperationBinding{}, err
	}
	view, err := json.Marshal(call.View)
	if err != nil {
		return OperationBinding{}, err
	}
	viewHash := sha256.Sum256(view)
	inputDigest, err := operationInputDigest(call.Input, intent.CanonicalDigest)
	if err != nil {
		return OperationBinding{}, err
	}
	binding := OperationBinding{
		Namespace:           intent.Namespace,
		Scope:               intent.Scope,
		Subject:             intent.Subject,
		OperationID:         intent.OperationID,
		Tool:                call.Manifest.Name,
		ManifestFingerprint: hex.EncodeToString(h.Sum(nil)),
		ViewFingerprint:     hex.EncodeToString(viewHash[:]),
		PolicyFingerprint:   intent.PolicyFingerprint,
		CanonicalDigest:     inputDigest,
		CanonicalRules:      intent.CanonicalRules,
		DownstreamKey:       intent.DownstreamKey,
	}
	return binding, validateOperationBinding(binding)
}

// Host binding covers secret/dependency identity; the library additionally binds
// prepared JSON and attachment bytes so changing input cannot reuse a grant even
// if the host's opaque dependency identity remains unchanged.
func operationInputDigest(input ToolInput, hostDigest string) (string, error) {
	if hostDigest == "" {
		return "", &OperationError{Kind: "invalid_binding"}
	}
	attachments := make([]cacheAttachment, len(input.Attachments))
	for i, attachment := range input.Attachments {
		attachments[i] = cacheAttachment{MIME: attachment.MimeType, Data: attachment.Data}
	}
	raw, err := json.Marshal(struct {
		Args        json.RawMessage   `json:"args"`
		Attachments []cacheAttachment `json:"attachments"`
		HostDigest  string            `json:"host_digest"`
	}{Args: input.ArgsJSON, Attachments: attachments, HostDigest: hostDigest})
	if err != nil {
		return "", NewInternalError(err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func (p *OperationProfile) ExecutePrepared(
	ctx context.Context,
	call PreparedCall,
	invoke InvocationHandler,
	yield func(Chunk) error,
) error {
	intent, err := p.prepare(ctx, clonePreparedCall(call))
	if err != nil {
		return err
	}
	binding, err := p.binding(call, intent)
	if err != nil {
		return err
	}
	if len(intent.DisplayJSON) > p.maxBytes || !json.Valid(intent.DisplayJSON) {
		return &OperationError{Kind: "invalid_description"}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	now := p.clock()
	claim := OperationClaim{Binding: binding, AttemptID: intent.AttemptID, GrantID: intent.GrantID, Issuer: p.issuer,
		RequiresApproval: call.Manifest.RequiresConfirmation, Now: now, LeaseUntil: now.Add(p.lease)}
	decision, err := p.store.Claim(ctx, claim)
	if err != nil {
		var operationErr *OperationError
		if errors.As(err, &operationErr) && operationErr.Kind == operationApprovalRequired {
			challenge := ApprovalChallenge{
				Binding:     binding,
				DisplayJSON: append(json.RawMessage(nil), intent.DisplayJSON...),
			}
			if deliveryErr := yield(
				Chunk{Event: EventControl, Control: &PauseSignal{Reason: operationApprovalRequired}},
			); deliveryErr != nil {
				return deliveryErr
			}
			return &PendingApprovalError{Challenge: challenge}
		}
		if operationErr == nil {
			return &OperationStoreError{Stage: "claim", Cause: err}
		}
		return err
	}
	if decision.Record.Binding != binding {
		return &OperationError{Kind: "invalid_store_record"}
	}
	if !decision.Dispatch {
		if decision.Record.State == OperationCompleted {
			return replayResult(ctx, call, decision.Record.Result, p.codec, p.maxBytes, ReplaySourceOperation, yield)
		}
		return &OperationStateError{Record: cloneOperationRecord(decision.Record)}
	}
	if decision.Record.State != OperationInProgress || decision.Record.AttemptID != claim.AttemptID ||
		!decision.Record.LeaseUntil.Equal(claim.LeaseUntil) || decision.Record.GrantID != claim.GrantID {
		return &OperationError{Kind: "invalid_store_record"}
	}
	return p.dispatch(ctx, claim, decision.Record, invoke, yield)
}

func (p *OperationProfile) dispatch(
	ctx context.Context,
	claim OperationClaim,
	record OperationRecord,
	invoke InvocationHandler,
	yield func(Chunk) error,
) (err error) {
	finished := false
	dispatchInvoked := false
	defer func() {
		if !finished {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), operationCleanupTimeout)
			defer cancel()
			finishErr := p.store.Finish(
				cleanupCtx,
				OperationFinish{
					Binding:   claim.Binding,
					AttemptID: claim.AttemptID,
					State:     OperationUnknown,
					Result:    nil,
				},
			)
			if finishErr != nil {
				err = errors.Join(err, &OperationStoreError{Stage: "unknown_finish", Cause: finishErr})
			}
			err = NewInternalError(
				&OperationOutcomeError{
					Cause:           err,
					Binding:         claim.Binding,
					AttemptID:       claim.AttemptID,
					DispatchInvoked: dispatchInvoked,
				},
			)
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	if !p.clock().Before(claim.LeaseUntil) {
		return &OperationError{Kind: "claim_expired"}
	}
	if claim.RequiresApproval && (record.ApprovalExpiresAt.IsZero() || !p.clock().Before(record.ApprovalExpiresAt)) {
		return &OperationError{Kind: "approval_expired"}
	}
	capture := operationCapture{ctx: ctx, maxBytes: p.maxBytes, yield: yield, result: nil, err: nil}
	dispatchInvoked = true
	err = invoke(capture.accept)
	if err != nil {
		return err
	}
	if capture.err != nil {
		return capture.err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if capture.result == nil {
		return &OperationError{Kind: "missing_terminal"}
	}
	raw, err := p.codec.EncodeResult(*capture.result)
	if err != nil {
		return err
	}
	if len(raw) > p.maxBytes {
		return &OperationError{Kind: "result_limit"}
	}
	if err = p.store.Finish(
		ctx,
		OperationFinish{Binding: claim.Binding, AttemptID: claim.AttemptID, State: OperationCompleted, Result: raw},
	); err != nil {
		return &OperationStoreError{Stage: "completed_finish", Cause: err}
	}
	finished = true // delivery failure cannot undo the persisted execution outcome.
	return yield(*capture.result)
}

type operationCapture struct {
	ctx      context.Context
	maxBytes int
	yield    func(Chunk) error
	result   *Chunk
	err      error
}

func (c *operationCapture) accept(chunk Chunk) error {
	if c.err != nil {
		return c.err
	}
	if c.ctx.Err() != nil {
		c.err = c.ctx.Err()
		return c.err
	}
	if chunk.Event == EventResult {
		if c.result != nil || chunk.IsError || len(chunk.Data) > c.maxBytes {
			c.err = &OperationError{Kind: "invalid_terminal"}
			return c.err
		}
		copyChunk := cloneResultChunk(chunk)
		c.result = &copyChunk
		return nil
	}
	c.err = c.yield(chunk)
	if c.err == nil && chunk.Event == EventControl {
		c.err = ControlErrorFromSignal(chunk.Control)
	}
	return c.err
}
