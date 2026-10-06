package sandboxfs

import (
	"errors"

	"github.com/skosovsky/toolsy/exectool"
)

// WithCleanupError appends a secondary cleanup diagnostic without replacing the
// primary execution outcome. Callers retain the original RunResult. A failed
// cleanup after a completed guest still returns an infrastructure error.
func WithCleanupError(primary error, backend, operation string, cleanupErr error) error {
	if cleanupErr == nil {
		return primary
	}
	return errors.Join(primary, &exectool.CleanupError{
		Backend: backend, Operation: operation, ResourceID: "", Cause: cleanupErr,
	})
}
