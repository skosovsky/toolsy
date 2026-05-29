// Package human provides suspend-first human-in-the-loop (HITL) tools.
// Each execution yields a typed PauseSignal (EventControl) and returns toolsy.ErrPause,
// leaving checkpointing and resume mechanics to the orchestrator.
package human
