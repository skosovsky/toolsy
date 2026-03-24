package wazero

import "github.com/tetratelabs/wazero"

// Option configures the wazero-backed sandbox.
type Option func(*options)

type options struct {
	runtimeConfig wazero.RuntimeConfig
}

// WithRuntimeConfig overrides the default wazero runtime configuration.
func WithRuntimeConfig(config wazero.RuntimeConfig) Option {
	return func(o *options) {
		o.runtimeConfig = config
	}
}
