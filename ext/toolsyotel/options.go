package toolsyotel

import "go.opentelemetry.io/otel/trace"

const defaultMaxPayloadSize = 4096

type config struct {
	tracerProvider trace.TracerProvider
	contentCapture bool
	maxPayloadSize int
}

// Option configures tracing middleware behavior.
type Option func(*config)

func defaultConfig() config {
	return config{
		tracerProvider: nil,
		contentCapture: false,
		maxPayloadSize: defaultMaxPayloadSize,
	}
}

func (c *config) effectiveMaxPayloadSize() int {
	if c.maxPayloadSize <= 0 {
		return defaultMaxPayloadSize
	}
	return c.maxPayloadSize
}

// WithTracerProvider sets the tracer provider used by middleware.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *config) {
		if tp != nil {
			c.tracerProvider = tp
		}
	}
}

// WithContentCapture enables capture of tool input/output payloads in span attributes.
// Disabled by default because payloads may contain PII or be very large.
func WithContentCapture(enabled bool) Option {
	return func(c *config) {
		c.contentCapture = enabled
	}
}

// WithMaxPayloadSize sets the maximum captured payload size in bytes for input and output.
// Defaults to 4096. Values <= 0 fall back to the default.
func WithMaxPayloadSize(bytes int) Option {
	return func(c *config) {
		c.maxPayloadSize = bytes
	}
}
