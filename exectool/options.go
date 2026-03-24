package exectool

import (
	"time"

	"github.com/skosovsky/toolsy"
)

// Option configures the generic exec_code tool.
type Option func(*options)

type options struct {
	name             string
	description      string
	timeout          time.Duration
	allowedLanguages []string
	toolOptions      []toolsy.ToolOption
}

func applyDefaults(o *options) {
	if o.name == "" {
		o.name = "exec_code"
	}
	if o.description == "" {
		o.description = "Run code in a configured sandbox and return stdout, stderr, exit code, and duration"
	}
}

// WithName overrides the public tool name.
func WithName(name string) Option {
	return func(o *options) {
		o.name = name
	}
}

// WithDescription overrides the public tool description.
func WithDescription(description string) Option {
	return func(o *options) {
		o.description = description
	}
}

// WithTimeout sets the infrastructure-enforced execution timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.timeout = timeout
	}
}

// WithAllowedLanguages constrains the LLM-facing schema to the intersection of
// sandbox capabilities and the provided language allowlist.
func WithAllowedLanguages(languages ...string) Option {
	return func(o *options) {
		o.allowedLanguages = append([]string(nil), languages...)
	}
}

// WithToolOptions forwards toolsy metadata/options to the generated tool.
func WithToolOptions(opts ...toolsy.ToolOption) Option {
	return func(o *options) {
		o.toolOptions = append(o.toolOptions, opts...)
	}
}
