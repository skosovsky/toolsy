package memory

// Option configures a Scratchpad.
type Option func(*options)

type options struct {
	maxFacts int
}

// WithMaxFacts sets the maximum number of facts the scratchpad can hold.
// Zero means no limit.
func WithMaxFacts(n int) Option {
	return func(o *options) {
		o.maxFacts = n
	}
}
