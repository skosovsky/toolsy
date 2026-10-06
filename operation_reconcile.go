package toolsy

import (
	"context"
	"errors"
	"time"
)

// ReconcileOperation is an explicit trusted-host action. The host authenticates
// callers and supplies a callback that queries external outcome/downstream
// idempotency semantics. Never register this function as a tool for model input.
func ReconcileOperation(ctx context.Context, store OperationStore, binding OperationBinding,
	clock func() time.Time, reconcile OperationReconciler,
) error {
	if store == nil || clock == nil || reconcile == nil {
		return &OperationError{Kind: "reconciliation_required"}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	record, found, err := store.Inspect(ctx, binding)
	if err != nil {
		return operationReadError(err)
	}
	if !found {
		return &OperationError{Kind: "operation_not_found"}
	}
	if record.Binding != binding {
		return &OperationError{Kind: operationBindingMismatch}
	}
	now := clock()
	if now.IsZero() {
		return &OperationError{Kind: "invalid_clock"}
	}
	if record.State == OperationInProgress && !now.Before(record.LeaseUntil) {
		record.State = OperationUnknown
	}
	if record.State != OperationUnknown {
		return &OperationError{Kind: operationInvalidTransition}
	}
	decision, err := reconcile(ctx, cloneOperationRecord(record))
	if err != nil {
		return &ReconciliationError{Cause: err}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	resolution := OperationResolution{Binding: binding, ExpectedAttemptID: record.AttemptID, State: decision.State,
		Result: append([]byte(nil), decision.Result...), ProofID: decision.ProofID, Now: clock()}
	if err = store.Resolve(ctx, resolution); err != nil {
		if _, ok := errors.AsType[*OperationError](err); ok {
			return err
		}
		return &OperationStoreError{Stage: "resolve", Cause: err}
	}
	return nil
}

func operationReadError(err error) error {
	if _, ok := errors.AsType[*OperationError](err); ok {
		return err
	}
	return &OperationStoreError{Stage: "inspect", Cause: err}
}
