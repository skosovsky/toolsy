package agents

import "context"

// CancellationStage identifies where interrupted-task cleanup completed or failed.
type CancellationStage string

const (
	// CancellationCredentials means cancellation credentials could not be resolved.
	CancellationCredentials CancellationStage = "credentials"
	// CancellationRequest means CancelTask was called; acknowledgement is not remote completion.
	CancellationRequest CancellationStage = "request"
)

// CancellationDiagnostic is a host-only observation, never retry authorization.
// TaskID and Cause can contain untrusted/sensitive observations; inspect deliberately.
type CancellationDiagnostic struct {
	TaskID          string
	ParentInterrupt error
	ParentCause     error
	Stage           CancellationStage
	Acknowledged    bool
	Cause           error
}

// CancellationObserver observes one best-effort cleanup synchronously. It must be
// safe for concurrent client calls, cooperate with ctx and not panic. Its lifetime
// is host-owned; the adapter cannot preempt a blocking opaque observer.
type CancellationObserver func(ctx context.Context, diagnostic CancellationDiagnostic)

// WithCancellationObserver installs optional host diagnostics for parent-triggered
// cancellation attempts. Nil disables diagnostics. Observations do not replace the
// primary execution error or appear in tool output. No observer is invoked on
// stream-only timeout or consumer stop while the parent remains active.
func WithCancellationObserver(observer CancellationObserver) func(*ClientOptions) {
	return func(o *ClientOptions) { o.cancellationObserver = observer }
}
