package toolsy

import (
	"context"
	"fmt"

	"github.com/skosovsky/toolsy/textprocessor"
)

// ToolkitContextError wraps ctx cancel/deadline as an internal tool error when ctx is done.
// Use before stat/size pre-checks so cooperative cancel wins over read-limit mapping.
func ToolkitContextError(ctx context.Context, op string) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return NewInternalError(fmt.Errorf("%s: %w", op, ctxErr))
	}
	return nil
}

// MapToolkitReadError enforces golden order for toolkit post-read paths:
// ctx interrupt → read-limit validation → nil for other errors (caller wraps).
func MapToolkitReadError(
	ctx context.Context,
	err error,
	op string,
	maxBytes int,
	subject, hint string,
) error {
	if err == nil {
		return nil
	}
	if ie := ToolkitContextError(ctx, op); ie != nil {
		return ie
	}
	if IsContextInterrupt(err) {
		return NewInternalError(fmt.Errorf("%s: %w", op, err))
	}
	if textprocessor.IsReadLimitExceeded(err) {
		return MapReadLimitErrorFor(err, maxBytes, subject, hint)
	}
	return nil
}

// MapToolkitCapError enforces golden order for locally detected budget exceed
// (stat size, semantic table/markdown/wire cap) without an underlying read error.
func MapToolkitCapError(ctx context.Context, op string, maxBytes int, subject, hint string) error {
	if ie := ToolkitContextError(ctx, op); ie != nil {
		return ie
	}
	return MapReadLimitErrorFor(textprocessor.ErrReadLimitExceeded, maxBytes, subject, hint)
}
