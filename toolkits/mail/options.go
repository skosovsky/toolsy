package mail

// Option configures AsTools (read-only mode, body limit, tool names).
type Option func(*options)

type options struct {
	readOnly     bool
	maxBodyBytes int
	sendName     string
	sendDesc     string
	searchName   string
	searchDesc   string
	readName     string
	readDesc     string
}

const defaultMaxBodyBytes = 256 * 1024

func applyDefaults(o *options) {
	if o.maxBodyBytes <= 0 {
		o.maxBodyBytes = defaultMaxBodyBytes
	}
	if o.sendName == "" {
		o.sendName = "mail_send"
	}
	if o.sendDesc == "" {
		o.sendDesc = "Send an email to the given recipients"
	}
	if o.searchName == "" {
		o.searchName = "mail_search_inbox"
	}
	if o.searchDesc == "" {
		o.searchDesc = "Search inbox by query; returns list of messages (ID, From, Subject, Date)"
	}
	if o.readName == "" {
		o.readName = "mail_read_message"
	}
	if o.readDesc == "" {
		o.readDesc = "Read a single message by message_id"
	}
}

// WithReadOnly disables mail_send even when sender is non-nil.
func WithReadOnly(readOnly bool) Option {
	return func(o *options) {
		o.readOnly = readOnly
	}
}

// WithMaxBodyBytes sets the maximum body size for send and read (default 256KB).
func WithMaxBodyBytes(n int) Option {
	return func(o *options) {
		o.maxBodyBytes = n
	}
}

// WithSendName sets the name of the mail_send tool.
func WithSendName(name string) Option {
	return func(o *options) {
		o.sendName = name
	}
}

// WithSendDescription sets the description of the mail_send tool.
func WithSendDescription(desc string) Option {
	return func(o *options) {
		o.sendDesc = desc
	}
}

// WithSearchName sets the name of the mail_search_inbox tool.
func WithSearchName(name string) Option {
	return func(o *options) {
		o.searchName = name
	}
}

// WithSearchDescription sets the description of the mail_search_inbox tool.
func WithSearchDescription(desc string) Option {
	return func(o *options) {
		o.searchDesc = desc
	}
}

// WithReadName sets the name of the mail_read_message tool.
func WithReadName(name string) Option {
	return func(o *options) {
		o.readName = name
	}
}

// WithReadDescription sets the description of the mail_read_message tool.
func WithReadDescription(desc string) Option {
	return func(o *options) {
		o.readDesc = desc
	}
}
