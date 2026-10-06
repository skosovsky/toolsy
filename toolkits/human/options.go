package human

// Option configures AsTools (tool names and descriptions).
type Option func(*options)

type options struct {
	approvalName      string
	approvalDesc      string
	clarificationName string
	clarificationDesc string
	maxPayloadBytes   int
}

const (
	defaultApprovalName      = "request_approval"
	defaultApprovalDesc      = "Request human review; this conversation does not authorize an action"
	defaultClarificationName = "ask_human_clarification"
	defaultClarificationDesc = "Ask a human for clarification"
	defaultMaxPayloadBytes   = 16 * 1024
)

// WithMaxPayloadBytes sets the complete encoded pause payload limit.
// An explicitly supplied zero or negative limit is invalid.
func WithMaxPayloadBytes(limit int) Option {
	return func(o *options) { o.maxPayloadBytes = limit }
}

func applyDefaults(o *options) {
	if o.approvalName == "" {
		o.approvalName = defaultApprovalName
	}
	if o.approvalDesc == "" {
		o.approvalDesc = defaultApprovalDesc
	}
	if o.clarificationName == "" {
		o.clarificationName = defaultClarificationName
	}
	if o.clarificationDesc == "" {
		o.clarificationDesc = defaultClarificationDesc
	}
}

// WithApprovalName sets the name of the approval tool.
func WithApprovalName(name string) Option {
	return func(o *options) {
		o.approvalName = name
	}
}

// WithApprovalDescription sets the description of the approval tool.
func WithApprovalDescription(desc string) Option {
	return func(o *options) {
		o.approvalDesc = desc
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
