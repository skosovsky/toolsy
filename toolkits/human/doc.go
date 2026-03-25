// Package human provides suspend-first human-in-the-loop (HITL) tools.
// Each execution yields one JSON suspend payload and then returns toolsy.ErrSuspend,
// leaving checkpointing and resume mechanics to the orchestrator.
package human
