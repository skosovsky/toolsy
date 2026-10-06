package toolsy

import (
	"context"
	"time"
)

const (
	operationApprovalRequired  = "approval_required"
	operationBindingMismatch   = "binding_mismatch"
	operationInvalidTransition = "invalid_transition"
	operationCorruptSnapshot   = "corrupt_snapshot"
	operationStaleAttempt      = "stale_attempt"
)

// OperationStoreError identifies infrastructure failure separately from a
// binding/approval decision or handler error. Cause stays host-only diagnostics.
type OperationStoreError struct {
	Stage string
	Cause error
}

func (e *OperationStoreError) Error() string { return "toolsy: operation store unavailable" }
func (e *OperationStoreError) Unwrap() error { return e.Cause }

// OperationBinding binds one host-defined logical intent to authorized inputs.
// Scope/Subject are opaque host identifiers, not library business identity types.
// CanonicalDigest may be keyed by the host when arguments contain secret refs.
type OperationBinding struct {
	Namespace           string `json:"namespace"`
	Scope               string `json:"scope"`
	Subject             string `json:"subject"`
	OperationID         string `json:"operation_id"`
	Tool                string `json:"tool"`
	ManifestFingerprint string `json:"manifest_fingerprint"`
	ViewFingerprint     string `json:"view_fingerprint"`
	PolicyFingerprint   string `json:"policy_fingerprint"`
	CanonicalDigest     string `json:"canonical_digest"`
	CanonicalRules      string `json:"canonical_rules"`
	DownstreamKey       string `json:"downstream_key"`
}

// OperationState is the durable state, distinct from transport delivery status.
type OperationState string

const (
	OperationInProgress      OperationState = "in_progress"
	OperationCompleted       OperationState = "completed"
	OperationUnknown         OperationState = "unknown_outcome"
	OperationRejected        OperationState = "rejected"
	OperationRetryAuthorized OperationState = "retry_authorized"
)

// ApprovalGrant is written only through a trusted host issuer port. Grant IDs,
// signatures and model-provided strings alone never authorize replay.
type ApprovalGrant struct {
	ID            string           `json:"id"`
	Issuer        string           `json:"issuer"`
	Binding       OperationBinding `json:"binding"`
	IssuedAt      time.Time        `json:"issued_at"`
	ExpiresAt     time.Time        `json:"expires_at"`
	Rejected      bool             `json:"rejected"`
	AllowRecovery bool             `json:"allow_recovery"`
}

// OperationClaim requests one atomic approval reservation and dispatch claim.
// Lease expiry changes uncertainty, not permission to start another effect.
// GrantID must be empty when RequiresApproval is false. Recovery retains the
// original approval mode; it cannot erase a consumed reservation.
type OperationClaim struct {
	Binding          OperationBinding
	AttemptID        string
	GrantID          string
	Issuer           string
	RequiresApproval bool
	Now              time.Time
	LeaseUntil       time.Time
}

// OperationRecord exposes a durable result or uncertainty, never a new grant.
type OperationRecord struct {
	Binding           OperationBinding `json:"binding"`
	State             OperationState   `json:"state"`
	AttemptID         string           `json:"attempt_id"`
	LeaseUntil        time.Time        `json:"lease_until"`
	Result            []byte           `json:"result"`
	GrantID           string           `json:"grant_id"`
	ApprovalExpiresAt time.Time        `json:"approval_expires_at"`
	Attempts          []string         `json:"attempts"`
	ResolutionProofID string           `json:"resolution_proof_id"`
	ResolvedAt        time.Time        `json:"resolved_at"`
}

// ClaimResult reports whether this attempt may dispatch. A stored completed
// result is replayed only after current invocation policy has already succeeded.
type ClaimResult struct {
	Dispatch bool
	Record   OperationRecord
}

// OperationFinish fences a terminal state update to the reserved attempt.
// Unknown is required if the handler started but its external outcome is unproven.
type OperationFinish struct {
	Binding   OperationBinding
	AttemptID string
	State     OperationState
	Result    []byte
}

// OperationResolution is a trusted host reconciliation decision. ProofID is
// provenance, not authentication. A host may authorize a retry only with verified
// reconciliation or declared downstream idempotency using the original key.
type OperationResolution struct {
	Binding           OperationBinding
	ExpectedAttemptID string
	State             OperationState
	Result            []byte
	ProofID           string
	Now               time.Time
}

// OperationStore atomically owns approval reservation and operation state.
// Implementations must enforce binding conflicts, issuer/expiry, fencing and
// crash recovery. Separate independent grant-consume and operation-claim writes
// do not satisfy this contract.
type OperationStore interface {
	// Inspect is a host-only read: it must not reserve, consume approval, expire
	// a lease, create an operation or authorize dispatch.
	Inspect(context.Context, OperationBinding) (OperationRecord, bool, error)
	Claim(context.Context, OperationClaim) (ClaimResult, error)
	Finish(context.Context, OperationFinish) error
	Resolve(context.Context, OperationResolution) error
}

// ReconciliationDecision describes trusted external evidence. RetryAuthorized
// requires verified absence or downstream idempotency with the original key;
// no lease/timeout inference or model-supplied proof is sufficient.
type ReconciliationDecision struct {
	State   OperationState
	Result  []byte
	ProofID string
}

// OperationReconciler is an authenticated host callback, not a model-facing tool.
// It receives a private copy of the uncertain operation and original downstream
// key. It cannot redirect resolution to another binding or fencing attempt.
type OperationReconciler func(context.Context, OperationRecord) (ReconciliationDecision, error)

type ReconciliationError struct{ Cause error }

func (e *ReconciliationError) Error() string { return "toolsy: reconciliation unavailable" }
func (e *ReconciliationError) Unwrap() error { return e.Cause }

// ApprovalIssuerStore is a host-only durable grant writer. Never expose it to
// tool code or use a model's text as an authenticated approval decision.
type ApprovalIssuerStore interface {
	PutGrant(context.Context, ApprovalGrant) error
}
