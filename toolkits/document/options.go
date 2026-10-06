package document

import (
	"context"
	"io"
	"net/http"
	"os"

	"github.com/skosovsky/toolsy/toolkits/httptool"

	"github.com/skosovsky/toolsy"
)

// Option configures AsTool (limits, remote fetch, tool name).
type Option func(*options)

type options struct {
	maxBytes            int
	limits              Limits
	localSource         LocalSource
	inProcessPDF        bool
	allowRemote         bool
	allowPrivateIPs     bool
	httpClient          *http.Client
	httpSettings        httptool.ClientSettings
	toolName            string
	toolDesc            string
	resultFormatter     func(ExtractWireResult) (any, error)
	hostResultValidator func(any) error
}

const (
	defaultMaxItems  = 1024
	defaultItemBytes = 64 * 1024
	defaultMaxBytes  = 2 * 1024 * 1024 // 2 MB
	defaultToolName  = "document_extract_text"
	defaultToolDesc  = "Extract text from a document (PDF, CSV, DOCX) by file path or URL"
)

func applyDefaults(o *options) {
	if o.maxBytes <= 0 {
		o.maxBytes = defaultMaxBytes
	}
	if o.toolName == "" {
		o.toolName = defaultToolName
	}
	if o.toolDesc == "" {
		o.toolDesc = defaultToolDesc
	}
}

// WithMaxBytes sets the final JSON byte budget (default 2 MiB). Zero selects the default;
// negative values fail construction. Source and parser budgets are configured with WithLimits.
func WithMaxBytes(n int) Option {
	return func(o *options) {
		o.maxBytes = n
	}
}

// WithAllowRemote enables fetching documents by URL (default false).
func WithAllowRemote(allow bool) Option {
	return func(o *options) {
		o.allowRemote = allow
	}
}

// WithAllowPrivateIPs allows fetching from loopback/link-local/private IPs (default false).
// Use only for tests (e.g. httptest); production should keep false for SSRF safety.
func WithAllowPrivateIPs(allow bool) Option {
	return func(o *options) {
		o.allowPrivateIPs = allow
	}
}

// WithHTTPSettings applies explicit timeout/TLS settings to the owned safe pool.
func WithHTTPSettings(settings httptool.ClientSettings) Option {
	return func(o *options) { o.httpSettings = settings }
}

// WithToolName sets the name of the extract tool.
func WithToolName(name string) Option {
	return func(o *options) {
		o.toolName = name
	}
}

// WithToolDescription sets the description of the extract tool.
func WithToolDescription(desc string) Option {
	return func(o *options) {
		o.toolDesc = desc
	}
}

// WithResultFormatter overrides JSON output for document extraction.
func WithResultFormatter(f func(ExtractWireResult) (any, error)) Option {
	return func(o *options) {
		o.resultFormatter = f
	}
}

// WithHostResultValidator validates formatted tool output before JSON marshal.
func WithHostResultValidator(v func(any) error) Option {
	return func(o *options) {
		o.hostResultValidator = v
	}
}

// LocalSource opens a host-authorized reference. The toolkit closes and bounds the reader.
// The host owns authorization, cancellation of blocking Open/Read calls, and reference semantics.
type LocalSource func(context.Context, string) (io.ReadCloser, error)

// Limits separates source, parsed data, collection count and individual item budgets.
// Zero selects finite defaults; negative values are configuration errors.
type Limits struct {
	SourceBytes int
	ParsedBytes int
	MaxItems    int
	ItemBytes   int
}

// WithLimits sets parser budgets independently of final JSON bytes.
func WithLimits(limits Limits) Option { return func(o *options) { o.limits = limits } }

// WithLocalSource selects a host-authorized source opener. No local source is enabled by default.
func WithLocalSource(open LocalSource) Option { return func(o *options) { o.localSource = open } }

// WithLocalRoot opens references relative to a host-owned root handle, enforcing containment at access.
// The host must keep root open for the lifetime of executions.
func WithLocalRoot(root *os.Root) Option {
	return func(o *options) {
		if root == nil {
			o.localSource = nil
			return
		}
		o.localSource = func(ctx context.Context, ref string) (io.ReadCloser, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			f, err := root.OpenFile(ref, os.O_RDONLY|nonblockFlag, 0)
			if err != nil {
				return nil, err
			}
			info, err := f.Stat()
			if err != nil || !info.Mode().IsRegular() {
				_ = f.Close()
				if err != nil {
					return nil, err
				}
				return nil, toolsy.NewValidationError("document: local source must be a regular file")
			}
			return f, nil
		}
	}
}

// WithInProcessPDF explicitly enables a parser that cannot bound page decode allocations or CPU.
// Use only when the host accepts this limitation or isolates the entire execution.
func WithInProcessPDF(allow bool) Option { return func(o *options) { o.inProcessPDF = allow } }

func parserLimits(o *options) Limits {
	l := o.limits
	if l.SourceBytes == 0 {
		l.SourceBytes = defaultMaxBytes
	}
	if l.ParsedBytes == 0 {
		l.ParsedBytes = defaultMaxBytes
	}
	if l.MaxItems == 0 {
		l.MaxItems = defaultMaxItems
	}
	if l.ItemBytes == 0 {
		l.ItemBytes = defaultItemBytes
	}
	return l
}
