package human

// Option rejects nil at construction. Host ports/callbacks are borrowed; the host owns
// their lifetime and synchronization. It configures conversational intent tools, not approval authority.
type Option func(*options)

type options struct {
	reviewName        string
	reviewDesc        string
	clarificationName string
	clarificationDesc string
	maxPayloadBytes   int
}

const (
	defaultReviewName        = "request_human_review"
	defaultReviewDesc        = "Request human review; this conversation does not authorize an action"
	defaultClarificationName = "ask_human_clarification"
	defaultClarificationDesc = "Ask a human for clarification"
	defaultMaxPayloadBytes   = 16 * 1024
)

// WithMaxPayloadBytes sets the complete encoded pause payload limit.
// Limits must be within 1..toolsy.MaxControlBytes; larger budgets cannot be delivered.
func WithMaxPayloadBytes(limit int) Option {
	return func(o *options) { o.maxPayloadBytes = limit }
}

func applyDefaults(o *options) {
	if o.reviewName == "" {
		o.reviewName = defaultReviewName
	}
	if o.reviewDesc == "" {
		o.reviewDesc = defaultReviewDesc
	}
	if o.clarificationName == "" {
		o.clarificationName = defaultClarificationName
	}
	if o.clarificationDesc == "" {
		o.clarificationDesc = defaultClarificationDesc
	}
}

// WithReviewName sets the name of the human review intent tool.
func WithReviewName(name string) Option {
	return func(o *options) {
		o.reviewName = name
	}
}

// WithReviewDescription sets the description of the human review intent tool.
func WithReviewDescription(desc string) Option {
	return func(o *options) {
		o.reviewDesc = desc
	}
}

// WithClarificationName sets the name of the clarification tool.
func WithClarificationName(name string) Option {
	return func(o *options) {
		o.clarificationName = name
	}
}

// WithClarificationDescription sets the description of the clarification tool.
func WithClarificationDescription(desc string) Option {
	return func(o *options) {
		o.clarificationDesc = desc
	}
}
