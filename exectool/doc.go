// Package exectool provides a generic LLM-facing code execution tool plus the
// low-level contracts used by sandbox adapters.
//
// The generic tool created by New exposes one tool, typically named
// "exec_code", with a dynamic JSON Schema derived from the sandbox's supported
// languages. Execution time limits come only from the [context.Context] passed to
// [Sandbox.Run] (e.g. caller deadlines or wrappers such as routery.Timeout).
// Time limits are never exposed to the LLM-facing schema.
//
// Low-level sandbox adapters exchange only strings, bytes, and durations via
// RunRequest and RunResult.
package exectool
