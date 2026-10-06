package wazero

import "github.com/tetratelabs/wazero"

// Option configures the wazero-backed sandbox.
type Option func(*options)

type options struct {
	runtimeConfig    wazero.RuntimeConfig
	memoryLimitPages uint32
}

// WithRuntimeConfig selects the runtime configuration. Mandatory cancellation
// and the adapter memory limit are applied after this configuration.
func WithRuntimeConfig(config wazero.RuntimeConfig) Option {
	return func(o *options) {
		o.runtimeConfig = config
	}
}

// DefaultMemoryLimitPages bounds each guest linear memory to 64 MiB.
const DefaultMemoryLimitPages uint32 = 1024

// WithMemoryLimitPages sets the maximum number of 64 KiB pages per guest memory.
// Values outside 1–65536 are rejected by NewInterpreter.
func WithMemoryLimitPages(pages uint32) Option {
	return func(o *options) { o.memoryLimitPages = pages }
}
