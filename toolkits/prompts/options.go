package prompts

const defaultMaxBytes = 512 * 1024

// Option configures the prompts tool.
type Option func(*options)

type options struct {
	name        string
	description string
	maxBytes    int
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

// WithMaxBytes sets the maximum length of the returned instructions in bytes (default: 512KB).
// Truncation is UTF-8 safe. Zero means use default.
func WithMaxBytes(n int) Option {
	return func(o *options) {
		o.maxBytes = n
	}
}

func (o *options) applyDefaults() {
	if o.name == "" {
		o.name = "get_agent_instructions"
	}
	if o.description == "" {
		o.description = "Get system prompt instructions for a given role"
	}
	if o.maxBytes <= 0 {
		o.maxBytes = defaultMaxBytes
	}
}
