package toolsy

import (
	"context"
	"errors"
	"net"
	"time"
)

const (
	defaultRetryMaxAttempts = 3
	defaultRetryBaseDelay   = 100 * time.Millisecond
	defaultRetryMaxDelay    = time.Second
)

// RetryOption configures idempotent retry middleware.
type RetryOption func(*retryConfig)

type retryConfig struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	shouldRetry func(error) bool
	sleep       func(context.Context, time.Duration) error
}

// WithRetryMaxAttempts sets the maximum number of execution attempts (including the first attempt).
func WithRetryMaxAttempts(n int) RetryOption {
	return func(c *retryConfig) {
		if n > 0 {
			c.maxAttempts = n
		}
	}
}

// WithRetryBaseDelay sets the initial exponential backoff delay.
func WithRetryBaseDelay(d time.Duration) RetryOption {
	return func(c *retryConfig) {
		if d >= 0 {
			c.baseDelay = d
		}
	}
}

// WithRetryMaxDelay sets the upper bound for exponential backoff delay.
func WithRetryMaxDelay(d time.Duration) RetryOption {
	return func(c *retryConfig) {
		if d >= 0 {
			c.maxDelay = d
		}
	}
}

// WithRetryShouldRetry overrides retryable error classification logic.
func WithRetryShouldRetry(fn func(error) bool) RetryOption {
	return func(c *retryConfig) {
		if fn != nil {
			c.shouldRetry = fn
		}
	}
}

// WithIdempotentRetry retries transient execution failures for read-only tools.
// Once any chunk is successfully delivered, retries stop to avoid duplicating partial output.
func WithIdempotentRetry(opts ...RetryOption) Middleware {
	cfg := retryConfig{
		maxAttempts: defaultRetryMaxAttempts,
		baseDelay:   defaultRetryBaseDelay,
		maxDelay:    defaultRetryMaxDelay,
		shouldRetry: defaultShouldRetry,
		sleep:       sleepWithContext,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(next Tool) Tool {
		return &retryTool{
			toolBase: toolBase{next: next},
			cfg:      cfg,
		}
	}
}

type retryTool struct {
	toolBase

	cfg retryConfig
}

func (t *retryTool) Execute(
	ctx context.Context,
	run RunContext,
	input ToolInput,
	yield func(Chunk) error,
) error {
	if !isReadOnlyTool(t.next.Manifest()) {
		return t.next.Execute(ctx, run, input, yield)
	}

	attempts := t.cfg.maxAttempts
	if attempts <= 0 {
		attempts = 1
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		deliveredChunk := false
		yieldWrapped := func(c Chunk) error {
			if err := yield(c); err != nil {
				return err
			}
			deliveredChunk = true
			return nil
		}

		err := t.next.Execute(ctx, run, input, yieldWrapped)
		if err == nil {
			return nil
		}
		if deliveredChunk {
			return err
		}
		if attempt == attempts || !t.cfg.shouldRetry(err) {
			return err
		}

		delay := retryDelay(t.cfg.baseDelay, t.cfg.maxDelay, attempt)
		if delay <= 0 {
			continue
		}
		if sleepErr := t.cfg.sleep(ctx, delay); sleepErr != nil {
			return sleepErr
		}
	}

	return nil
}

func isReadOnlyTool(manifest ToolManifest) bool {
	if len(manifest.Metadata) == 0 {
		return false
	}
	readOnly, ok := manifest.Metadata["read_only"].(bool)
	return ok && readOnly
}

// maxDurationShiftBits caps exponential backoff doubling before [time.Duration] overflow on typical platforms.
const maxDurationShiftBits = 62

func retryDelay(base, maxDelay time.Duration, attempt int) time.Duration {
	if base <= 0 || attempt <= 0 {
		return 0
	}

	overflowGuard := time.Duration(1) << maxDurationShiftBits
	delay := base
	for i := 1; i < attempt; i++ {
		if maxDelay > 0 && delay >= maxDelay {
			return maxDelay
		}
		if delay > overflowGuard {
			if maxDelay > 0 {
				return maxDelay
			}
			return delay
		}
		delay *= 2
	}
	if maxDelay > 0 && delay > maxDelay {
		return maxDelay
	}
	return delay
}

func defaultShouldRetry(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrSuspend) ||
		errors.Is(err, ErrStreamAborted) ||
		errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var clientErr *ClientError
	if errors.As(err, &clientErr) {
		return clientErr.Retryable
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
		type temporary interface {
			Temporary() bool
		}
		var tempErr temporary
		if errors.As(err, &tempErr) && tempErr.Temporary() {
			return true
		}
	}

	return false
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
