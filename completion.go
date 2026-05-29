package toolsy

// CompletionPolicy defines how the orchestrator should treat successful tool completion.
type CompletionPolicy string

const (
	// CompletionContinue is the default: proceed to the next agent step normally.
	CompletionContinue CompletionPolicy = "continue"
	// CompletionSilentYield stops the current chain quietly after this tool (no hard error).
	CompletionSilentYield CompletionPolicy = "silent_yield"
	// CompletionHalt stops the agent track after this tool.
	CompletionHalt CompletionPolicy = "halt"
)

// WithCompletionPolicy sets manifest completion policy for orchestrator routing.
func WithCompletionPolicy(policy CompletionPolicy) ToolOption {
	return func(c *ToolConfig) {
		c.Manifest.CompletionPolicy = policy
	}
}
