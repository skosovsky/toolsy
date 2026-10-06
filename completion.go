package toolsy

// CompletionPolicy defines how the orchestrator should treat successful tool completion.
type CompletionPolicy string

const (
	// CompletionContinue is the default host routing hint: continue after success.
	CompletionContinue CompletionPolicy = "continue"
	// CompletionSilentYield asks the host to end its continuation quietly after success.
	CompletionSilentYield CompletionPolicy = "silent_yield"
	// CompletionHalt asks the host to stop its continuation after success.
	// Core does not enforce these hints or cancel other calls.
	CompletionHalt CompletionPolicy = "halt"
)

// WithCompletionPolicy sets manifest completion policy for orchestrator routing.
func WithCompletionPolicy(policy CompletionPolicy) ToolOption {
	return func(c *ToolConfig) {
		c.Manifest.CompletionPolicy = policy
	}
}
