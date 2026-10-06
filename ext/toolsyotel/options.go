package toolsyotel

import "go.opentelemetry.io/otel/trace"

const defaultMaxPayloadSize = 4096

type config struct {
	tracerProvider        trace.TracerProvider
	contentCapture        bool
	langfuseCompatibility bool
	maxPayloadSize        int
	redactor              ContentRedactor
}

// Option configures tracing middleware behavior.
type Option func(*config)

func defaultConfig() config {
	return config{
		tracerProvider:        nil,
		contentCapture:        false,
		langfuseCompatibility: false,
		maxPayloadSize:        defaultMaxPayloadSize,
		redactor:              nil,
	}
}

func (c config) effectiveMaxPayloadSize() int {
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

// WithContentCapture enables capture of tool input/output and all error/panic payloads in span attributes.
// Disabled by default because payloads may contain PII or be very large.
func WithContentCapture(enabled bool) Option {
	return func(c *config) {
		c.contentCapture = enabled
	}
}

// WithMaxPayloadSize sets the maximum captured payload size in bytes, including truncation markers, for every captured field.
// Defaults to 4096. Values <= 0 fall back to the default.
func WithMaxPayloadSize(bytes int) Option {
	return func(c *config) {
		c.maxPayloadSize = bytes
	}
}

// ContentKind identifies a captured payload before redaction.
type ContentKind string

const (
	ContentInput  ContentKind = "input"
	ContentOutput ContentKind = "output"
	ContentError  ContentKind = "error"
	ContentPanic  ContentKind = "panic"
)

// ContentRedactor transforms captured content before any byte limit is applied.
// It may be invoked concurrently. Output streams are redacted per delivered chunk;
// hosts must not rely on matching secrets split across chunk boundaries. A panic
// in a redactor fails closed and emits no original content.
type ContentRedactor func(kind ContentKind, content string) string

// WithContentRedactor sets a vendor-neutral host redactor. It does not enable capture.
func WithContentRedactor(redactor ContentRedactor) Option {
	return func(c *config) { c.redactor = redactor }
}

//nolint:nonamedreturns // Recovery must replace the return value if the host redactor panics.
func (c config) captured(kind ContentKind, content string) (out string) {
	if !c.contentCapture {
		return ""
	}
	defer func() {
		if recover() != nil {
			out = truncatePayload("[redaction failed]", c.effectiveMaxPayloadSize())
		}
	}()
	if c.redactor != nil {
		content = c.redactor(kind, content)
	}
	return truncatePayload(content, c.effectiveMaxPayloadSize())
}

// WithLangfuseCompatibility adds explicit vendor observation attributes. Disabled
// by default; it never enables content capture or bypasses redaction/byte limits.
func WithLangfuseCompatibility(enabled bool) Option {
	return func(c *config) { c.langfuseCompatibility = enabled }
}
