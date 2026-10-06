package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"sync"
	"time"
)

// OperationError is a non-retryable operation decision. Unknown outcomes require
// trusted reconciliation, never an automatic retry of the handler.
type OperationError struct{ Kind string }

func (e *OperationError) Error() string { return "toolsy: operation " + e.Kind }

// OperationSnapshot is the complete atomic journal image. Only trusted storage
// adapters may restore it. Neither grants nor snapshots are model input.
type OperationSnapshot struct {
	Records  map[string]OperationRecord `json:"records"`
	Grants   map[string]ApprovalGrant   `json:"grants"`
	Consumed map[string]string          `json:"consumed"`
}

// MemoryOperationStore implements the journal transition contract, not durable
// recovery. Hosts needing crash recovery must select a durable adapter.
type MemoryOperationStore struct {
	mu    sync.Mutex
	state OperationSnapshot
}

func NewMemoryOperationStore() *MemoryOperationStore {
	return &MemoryOperationStore{mu: sync.Mutex{}, state: OperationSnapshot{
		Records: map[string]OperationRecord{}, Grants: map[string]ApprovalGrant{}, Consumed: map[string]string{},
	}}
}

// RestoreOperationStore copies a trusted, complete image for atomic adapters.
func RestoreOperationStore(snapshot OperationSnapshot) (*MemoryOperationStore, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	s := NewMemoryOperationStore()
	if err = json.Unmarshal(raw, &s.state); err != nil {
		return nil, err
	}
	if s.state.Records == nil || s.state.Grants == nil || s.state.Consumed == nil {
		return nil, errors.New("toolsy: incomplete operation snapshot")
	}
	if err = validateOperationSnapshot(s.state); err != nil {
		return nil, err
	}
	return s, nil
}

func validateOperationSnapshot(state OperationSnapshot) error {
	for key, record := range state.Records {
		if err := validateOperationRecord(key, record); err != nil {
			return err
		}
	}
	for id, grant := range state.Grants {
		if id != grant.ID || id == "" || grant.Issuer == "" || grant.IssuedAt.IsZero() ||
			!grant.ExpiresAt.After(grant.IssuedAt) ||
			validateOperationBinding(grant.Binding) != nil {
			return &OperationError{Kind: operationCorruptSnapshot}
		}
	}
	for id, key := range state.Consumed {
		grant, hasGrant := state.Grants[id]
		record, hasRecord := state.Records[key]
		if !hasGrant || !hasRecord || record.GrantID != id || grant.Binding != record.Binding ||
			!record.ApprovalExpiresAt.Equal(grant.ExpiresAt) {
			return &OperationError{Kind: operationCorruptSnapshot}
		}
	}
	return nil
}

func validateOperationRecord(key string, record OperationRecord) error {
	if validateOperationBinding(record.Binding) != nil || key != operationKey(record.Binding) ||
		record.AttemptID == "" || record.LeaseUntil.IsZero() {
		return &OperationError{Kind: operationCorruptSnapshot}
	}
	if err := validateOperationAttempts(record); err != nil {
		return err
	}
	switch record.State {
	case OperationCompleted:
		if len(record.Result) == 0 {
			return &OperationError{Kind: operationCorruptSnapshot}
		}
	case OperationRejected, OperationRetryAuthorized:
		if record.ResolutionProofID == "" || record.ResolvedAt.IsZero() || len(record.Result) != 0 {
			return &OperationError{Kind: operationCorruptSnapshot}
		}
	case OperationInProgress, OperationUnknown:
		if len(record.Result) != 0 {
			return &OperationError{Kind: operationCorruptSnapshot}
		}
	default:
		return &OperationError{Kind: operationCorruptSnapshot}
	}
	return nil
}

func validateOperationAttempts(record OperationRecord) error {
	attempts := make(map[string]bool, len(record.Attempts))
	for _, attempt := range record.Attempts {
		if attempt == "" || attempts[attempt] {
			return &OperationError{Kind: operationCorruptSnapshot}
		}
		attempts[attempt] = true
	}
	if !attempts[record.AttemptID] {
		return &OperationError{Kind: operationCorruptSnapshot}
	}
	return nil
}

func (s *MemoryOperationStore) Snapshot() OperationSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := OperationSnapshot{
		Records:  map[string]OperationRecord{},
		Grants:   map[string]ApprovalGrant{},
		Consumed: map[string]string{},
	}
	for k, v := range s.state.Records {
		state.Records[k] = cloneOperationRecord(v)
	}
	maps.Copy(state.Grants, s.state.Grants)
	maps.Copy(state.Consumed, s.state.Consumed)
	return state
}

func operationKey(binding OperationBinding) string {
	raw, _ := json.Marshal([3]string{binding.Namespace, binding.Scope, binding.OperationID})
	return string(raw)
}

func validateOperationBinding(b OperationBinding) error {
	if b.Namespace == "" || b.Scope == "" || b.Subject == "" || b.OperationID == "" || b.Tool == "" ||
		b.ManifestFingerprint == "" || b.PolicyFingerprint == "" || b.CanonicalDigest == "" || b.CanonicalRules == "" {
		return &OperationError{Kind: "invalid_binding"}
	}
	return nil
}

func cloneOperationRecord(r OperationRecord) OperationRecord {
	r.Result = append([]byte(nil), r.Result...)
	r.Attempts = append([]string(nil), r.Attempts...)
	return r
}

func (s *MemoryOperationStore) Inspect(ctx context.Context, binding OperationBinding) (OperationRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return OperationRecord{}, false, err
	}
	if err := validateOperationBinding(binding); err != nil {
		return OperationRecord{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.state.Records[operationKey(binding)]
	if !exists {
		return OperationRecord{}, false, nil
	}
	if record.Binding != binding {
		return OperationRecord{}, false, &OperationError{Kind: operationBindingMismatch}
	}
	return cloneOperationRecord(record), true, nil
}

func (s *MemoryOperationStore) PutGrant(ctx context.Context, grant ApprovalGrant) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateOperationBinding(grant.Binding); err != nil {
		return err
	}
	if grant.ID == "" || grant.Issuer == "" || grant.IssuedAt.IsZero() || !grant.ExpiresAt.After(grant.IssuedAt) {
		return &OperationError{Kind: "invalid_grant"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, exists := s.state.Grants[grant.ID]; exists && !sameApprovalGrant(old, grant) {
		return &OperationError{Kind: "grant_conflict"}
	}
	s.state.Grants[grant.ID] = grant
	return nil
}

func sameApprovalGrant(a, b ApprovalGrant) bool {
	return a.ID == b.ID && a.Issuer == b.Issuer && a.Binding == b.Binding && a.IssuedAt.Equal(b.IssuedAt) &&
		a.ExpiresAt.Equal(b.ExpiresAt) && a.Rejected == b.Rejected && a.AllowRecovery == b.AllowRecovery
}

func (s *MemoryOperationStore) Claim(ctx context.Context, claim OperationClaim) (ClaimResult, error) {
	if err := ctx.Err(); err != nil {
		return ClaimResult{}, err
	}
	if err := validateOperationBinding(claim.Binding); err != nil {
		return ClaimResult{}, err
	}
	if claim.AttemptID == "" || claim.Now.IsZero() || !claim.LeaseUntil.After(claim.Now) {
		return ClaimResult{}, &OperationError{Kind: "invalid_claim"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := operationKey(claim.Binding)
	record, exists := s.state.Records[key]
	if exists && record.Binding != claim.Binding {
		return ClaimResult{}, &OperationError{Kind: operationBindingMismatch}
	}
	if exists && record.State != OperationRetryAuthorized {
		if record.State == OperationInProgress && !claim.Now.Before(record.LeaseUntil) {
			record.State = OperationUnknown
			s.state.Records[key] = record
		}
		return ClaimResult{Dispatch: false, Record: cloneOperationRecord(record)}, nil
	}
	if exists && slices.Contains(record.Attempts, claim.AttemptID) {
		return ClaimResult{}, &OperationError{Kind: operationStaleAttempt}
	}
	if claim.RequiresApproval {
		if err := s.checkGrant(claim, key, exists); err != nil {
			return ClaimResult{}, err
		}
	}
	record.Binding = claim.Binding
	record.State = OperationInProgress
	record.AttemptID = claim.AttemptID
	record.LeaseUntil = claim.LeaseUntil
	record.GrantID = claim.GrantID
	record.ApprovalExpiresAt = time.Time{}
	record.Result = nil
	record.Attempts = append(record.Attempts, claim.AttemptID)
	if claim.RequiresApproval {
		record.ApprovalExpiresAt = s.state.Grants[claim.GrantID].ExpiresAt
		s.state.Consumed[claim.GrantID] = key
	}
	s.state.Records[key] = record
	return ClaimResult{Dispatch: true, Record: cloneOperationRecord(record)}, nil
}

func (s *MemoryOperationStore) checkGrant(claim OperationClaim, key string, recovery bool) error {
	grant, exists := s.state.Grants[claim.GrantID]
	if !exists {
		return &OperationError{Kind: operationApprovalRequired}
	}
	if grant.Binding != claim.Binding || grant.Issuer != claim.Issuer {
		return &OperationError{Kind: operationBindingMismatch}
	}
	if claim.Now.Before(grant.IssuedAt) || !claim.Now.Before(grant.ExpiresAt) {
		return &OperationError{Kind: "approval_expired"}
	}
	if grant.Rejected {
		return &OperationError{Kind: "approval_rejected"}
	}
	if consumed, used := s.state.Consumed[claim.GrantID]; used &&
		(consumed != key || !recovery || !grant.AllowRecovery) {
		return &OperationError{Kind: "approval_consumed"}
	}
	if recovery && s.state.Records[key].GrantID != claim.GrantID {
		return &OperationError{Kind: "approval_consumed"}
	}
	return nil
}

func (s *MemoryOperationStore) Finish(ctx context.Context, finish OperationFinish) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if finish.State != OperationCompleted && finish.State != OperationUnknown {
		return &OperationError{Kind: operationInvalidTransition}
	}
	if finish.State == OperationCompleted && len(finish.Result) == 0 {
		return &OperationError{Kind: "missing_result"}
	}
	if finish.State == OperationUnknown && len(finish.Result) != 0 {
		return &OperationError{Kind: operationInvalidTransition}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := operationKey(finish.Binding)
	record, exists := s.state.Records[key]
	if !exists || record.Binding != finish.Binding || record.AttemptID != finish.AttemptID ||
		record.State != OperationInProgress {
		return &OperationError{Kind: operationStaleAttempt}
	}
	record.State = finish.State
	record.Result = append([]byte(nil), finish.Result...)
	s.state.Records[key] = record
	return nil
}

func (s *MemoryOperationStore) Resolve(ctx context.Context, resolution OperationResolution) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if resolution.ProofID == "" || resolution.Now.IsZero() {
		return &OperationError{Kind: "reconciliation_required"}
	}
	if resolution.State != OperationCompleted && resolution.State != OperationRejected &&
		resolution.State != OperationRetryAuthorized {
		return &OperationError{Kind: operationInvalidTransition}
	}
	if resolution.State == OperationCompleted && len(resolution.Result) == 0 {
		return &OperationError{Kind: "missing_result"}
	}
	if resolution.State != OperationCompleted && len(resolution.Result) != 0 {
		return &OperationError{Kind: operationInvalidTransition}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := operationKey(resolution.Binding)
	record, exists := s.state.Records[key]
	if !exists || record.Binding != resolution.Binding || record.AttemptID != resolution.ExpectedAttemptID {
		return &OperationError{Kind: operationStaleAttempt}
	}
	if record.State == OperationInProgress && !resolution.Now.Before(record.LeaseUntil) {
		record.State = OperationUnknown
	}
	if record.State != OperationUnknown {
		return &OperationError{Kind: operationInvalidTransition}
	}
	record.State = resolution.State
	record.Result = append([]byte(nil), resolution.Result...)
	record.ResolutionProofID = resolution.ProofID
	record.ResolvedAt = resolution.Now
	s.state.Records[key] = record
	return nil
}
