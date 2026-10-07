package docker

import "context"

// A phase-triggered deadline signal avoids expiring before resource acquisition
// when the test machine is slow. This is a test context, not an elapsed-time
// claim. Actual collection-clock expiry is tested separately with LogTimeout.
type triggeredDeadlineContext struct{ context.Context }

func (c triggeredDeadlineContext) Err() error {
	if c.Context.Err() != nil {
		return context.DeadlineExceeded
	}
	return nil
}

func newTriggeredDeadlineContext() (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(context.Background())
	return triggeredDeadlineContext{Context: ctx}, func() { cancel(context.DeadlineExceeded) }
}
