package mail

// Option rejects nil at construction. Host ports/callbacks are borrowed; the host owns
// their lifetime and synchronization. It configures AsTools (read-only mode, body limit, tool names).
type Option func(*options)

type options struct {
	readOnly         bool
	maxBodyBytes     int
	maxSourceBytes   int
	maxItemBytes     int
	maxSearchResults int
	maxWireBytes     int
	sendName         string
	sendDesc         string
	searchName       string
	searchDesc       string
	readName         string
	readDesc         string
}

const (
	defaultMaxBodyBytes     = 256 * 1024
	defaultMaxSourceBytes   = 1024 * 1024
	defaultMaxItemBytes     = 256 * 1024
	defaultMaxWireBytes     = 1024 * 1024
	defaultMaxSearchResults = 100
	defaultSearchLimit      = 10
)

func applyDefaults(o *options) {
	if o.maxBodyBytes == 0 {
		o.maxBodyBytes = defaultMaxBodyBytes
	}
	if o.maxSourceBytes == 0 {
		o.maxSourceBytes = defaultMaxSourceBytes
	}
	if o.maxItemBytes == 0 {
		o.maxItemBytes = defaultMaxItemBytes
	}
	if o.maxSearchResults == 0 {
		o.maxSearchResults = defaultMaxSearchResults
	}
	if o.maxWireBytes == 0 {
		o.maxWireBytes = defaultMaxWireBytes
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

// WithMaxBodyBytes caps unchanged outgoing and raw incoming body bytes (default 256 KiB).
// Zero selects the default; negative is a configuration error.
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

// WithMaxSourceBytes caps aggregate provider string bytes before formatting (default 1 MiB).
// Zero selects the default; negative is a configuration error.
func WithMaxSourceBytes(n int) Option { return func(o *options) { o.maxSourceBytes = n } }

// WithMaxItemBytes caps all string fields of one provider message (default 256 KiB).
// Zero selects the default; negative is a configuration error.
func WithMaxItemBytes(n int) Option { return func(o *options) { o.maxItemBytes = n } }

// WithMaxSearchResults caps requested and actual search counts (default 100).
// Zero selects the default; negative is a configuration error.
func WithMaxSearchResults(n int) Option { return func(o *options) { o.maxSearchResults = n } }

// WithMaxWireBytes caps final encoded JSON bytes (default 1 MiB).
// Zero selects the default; negative is a configuration error.
func WithMaxWireBytes(n int) Option { return func(o *options) { o.maxWireBytes = n } }
