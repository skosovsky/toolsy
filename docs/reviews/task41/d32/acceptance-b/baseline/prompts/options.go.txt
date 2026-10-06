package prompts

const (
	defaultMaxBytes       = 512 * 1024
	defaultMaxSourceBytes = 1024 * 1024
	defaultMaxOutputBytes = 1024 * 1024
)

// Option configures the prompts tool.
type Option func(*options)

type options struct {
	name               string
	description        string
	maxBytes           int
	maxSourceBytes     int
	maxProvenanceBytes int
	maxOutputBytes     int
}

// WithName sets the tool name (default: "get_agent_instructions").
func WithName(name string) Option {
	return func(o *options) {
		o.name = name
	}
}

// WithDescription sets the tool description.
func WithDescription(description string) Option {
	return func(o *options) {
		o.description = description
	}
}

// WithMaxBytes bounds rendered instructions (default 512 KiB).
// Zero selects the default; negative values fail construction. Oversize fails explicitly.
func WithMaxBytes(n int) Option {
	return func(o *options) {
		o.maxBytes = n
	}
}

func (o *options) applyDefaults() {
	if o.maxSourceBytes == 0 {
		o.maxSourceBytes = defaultMaxSourceBytes
	}
	if o.maxProvenanceBytes == 0 {
		o.maxProvenanceBytes = 4096
	}
	if o.maxOutputBytes == 0 {
		o.maxOutputBytes = defaultMaxOutputBytes
	}
	if o.name == "" {
		o.name = "get_agent_instructions"
	}
	if o.description == "" {
		o.description = "Get system prompt instructions for a given role"
	}
	if o.maxBytes == 0 {
		o.maxBytes = defaultMaxBytes
	}
}

// WithMaxSourceBytes bounds total returned source field bytes (default 1 MiB).
// Zero selects the default; negative values fail construction.
func WithMaxSourceBytes(n int) Option { return func(o *options) { o.maxSourceBytes = n } }

// WithMaxProvenanceBytes bounds each provenance field (default 4096 bytes).
// Zero selects the default; negative values fail construction.
func WithMaxProvenanceBytes(n int) Option { return func(o *options) { o.maxProvenanceBytes = n } }

// WithMaxOutputBytes bounds final JSON bytes after escaping (default 1 MiB).
// Zero selects the default; negative values fail construction.
func WithMaxOutputBytes(n int) Option { return func(o *options) { o.maxOutputBytes = n } }
