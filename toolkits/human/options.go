package human

// Option configures AsTools (tool names and descriptions).
type Option func(*options)

type options struct {
	approvalName      string
	approvalDesc      string
	clarificationName string
	clarificationDesc string
}

const (
	defaultApprovalName      = "request_approval"
	defaultApprovalDesc      = "Request human approval for a dangerous action"
	defaultClarificationName = "ask_human_clarification"
	defaultClarificationDesc = "Ask a human for clarification"
)

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
