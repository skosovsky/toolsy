package memory

const (
	defaultMaxStoreBytes  = 512 * 1024
	defaultMaxOutputBytes = 1024 * 1024
)

// Option rejects nil at construction. Host ports/callbacks are borrowed; the host owns
// their lifetime and synchronization. It configures a Scratchpad. Zero restores a finite default; negative limits
// are configuration errors reported by NewScratchpad.
type Option func(*options)

type options struct {
	maxFacts       int
	maxKeyBytes    int
	maxValueBytes  int
	maxStoreBytes  int
	maxOutputBytes int
}

// WithMaxFacts bounds the number of stored facts (default 128).
func WithMaxFacts(n int) Option { return func(o *options) { o.maxFacts = n } }

// WithMaxKeyBytes bounds each key (default 256 UTF-8 bytes).
func WithMaxKeyBytes(n int) Option { return func(o *options) { o.maxKeyBytes = n } }

// WithMaxValueBytes bounds each value (default 4096 UTF-8 bytes).
func WithMaxValueBytes(n int) Option { return func(o *options) { o.maxValueBytes = n } }

// WithMaxStoreBytes bounds the serialized stored state (default 512 KiB).
func WithMaxStoreBytes(n int) Option { return func(o *options) { o.maxStoreBytes = n } }

// WithMaxOutputBytes bounds each final JSON response (default 1 MiB).
func WithMaxOutputBytes(n int) Option { return func(o *options) { o.maxOutputBytes = n } }

func (o *options) applyDefaults() {
	if o.maxFacts == 0 {
		o.maxFacts = 128
	}
	if o.maxKeyBytes == 0 {
		o.maxKeyBytes = 256
	}
	if o.maxValueBytes == 0 {
		o.maxValueBytes = 4096
	}
	if o.maxStoreBytes == 0 {
		o.maxStoreBytes = defaultMaxStoreBytes
	}
	if o.maxOutputBytes == 0 {
		o.maxOutputBytes = defaultMaxOutputBytes
	}
}
