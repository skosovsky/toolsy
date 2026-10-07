// Package exectool provides a generic LLM-facing code execution tool plus the
// low-level contracts used by sandbox adapters.
//
// The generic tool created by New exposes one tool, typically named
// "exec_code", with a dynamic JSON Schema derived from the sandbox's supported
// languages. Caller cancellation/deadlines use the [context.Context] passed to
// [Sandbox.Run]. Backends can additionally enforce owned collection/cleanup
// deadlines and computation/resource budgets; see each backend's capability policy.
// RunRequest and the LLM-facing schema have no timeout field. A caller deadline
// is not a universal hard bound on cleanup or synchronous host operations.
//
// Low-level sandbox adapters exchange only strings, bytes, and durations via
// RunRequest and RunResult.
package exectool
