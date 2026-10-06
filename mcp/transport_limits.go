package mcp

import (
	"errors"
	"fmt"
	"math"

	"github.com/skosovsky/toolsy/textprocessor"
)

const (
	defaultTransportMaxQueueBytes = 16 * 1024 * 1024
	defaultTransportMaxInFlight   = 64
)

// TransportLimits separates message/storage limits from cumulative traffic.
// Zero chooses bounded frame/queue/in-flight defaults; lifetime zero is unlimited.
// Lifetime covers stdout for stdio, and each response body for HTTP SSE.
type TransportLimits struct {
	MaxFrameBytes    int
	MaxQueueBytes    int
	MaxInFlight      int
	MaxLifetimeBytes int
}

func (l TransportLimits) normalized() (TransportLimits, error) {
	if l.MaxFrameBytes < 0 || l.MaxQueueBytes < 0 || l.MaxInFlight < 0 || l.MaxLifetimeBytes < 0 {
		return l, errors.New("mcp: transport limits cannot be negative")
	}
	if l.MaxFrameBytes == 0 {
		l.MaxFrameBytes = defaultTransportMaxFrameBytes
	}
	if l.MaxQueueBytes == 0 {
		l.MaxQueueBytes = defaultTransportMaxQueueBytes
	}
	if l.MaxInFlight == 0 {
		l.MaxInFlight = defaultTransportMaxInFlight
	}
	if l.MaxFrameBytes >= math.MaxInt-2 {
		return l, errors.New("mcp: frame limit cannot accommodate framing overhead")
	}
	return l, nil
}

func WithStdioLimits(limits TransportLimits) StdioTransportOption {
	return func(t *StdioTransport) { t.limits = limits }
}

func WithStreamableHTTPLimits(limits TransportLimits) StreamableHTTPOption {
	return func(t *StreamableHTTPTransport) { t.limits = limits }
}

func frameLimitError(subject string, limit int) error {
	return textprocessor.NewReadLimitError(subject, limit, textprocessor.ErrReadLimitExceeded)
}

// ErrTransportLimitExceeded identifies refusal to retain more transport work.
var ErrTransportLimitExceeded = errors.New("mcp: transport resource limit exceeded")

// TransportLimitError reports the configured resource and bound at admission.
// Frame/read byte failures instead retain textprocessor.ErrReadLimitExceeded.
type TransportLimitError struct {
	Resource string
	Limit    int
}

func (e *TransportLimitError) Error() string {
	return fmt.Sprintf("mcp: %s limit exceeded (%d)", e.Resource, e.Limit)
}

func (e *TransportLimitError) Unwrap() error { return ErrTransportLimitExceeded }

func retainedWorkLimit(count, retained, incoming int, limits TransportLimits) error {
	if count >= limits.MaxInFlight {
		return &TransportLimitError{Resource: "in-flight requests", Limit: limits.MaxInFlight}
	}
	if incoming > limits.MaxQueueBytes-retained {
		return &TransportLimitError{Resource: "queued bytes", Limit: limits.MaxQueueBytes}
	}
	return nil
}
