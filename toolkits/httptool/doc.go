// Package httptool provides HTTP GET/POST tools for agents with SSRF protection:
// allowed domains whitelist, optional private IP checks, and configurable response body limits.
//
// Library primitives (ReadBodyLimited, DrainResponseBody, LimitStreamReaderWithContext) return
// textprocessor.ErrReadLimitExceeded on exceed (fail-closed, nil data).
// Probe tools map limit errors to toolsy.CodeValidationFailed — use AsToolError in tool mode;
// the sentinel is not preserved on the tool error chain.
package httptool
