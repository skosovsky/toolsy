package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

const maxDiagnosticBytes = 256

func boundedDiagnostic(value string) string {
	if len(value) <= maxDiagnosticBytes {
		return value
	}
	cut := maxDiagnosticBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return fmt.Sprintf("%s...(truncated, %d bytes total)", value[:cut], len(value))
}

var (
	ErrTransportClosed = errors.New("mcp: transport closed")
	ErrSessionExpired  = errors.New("mcp: HTTP session expired")
)

type ProtocolVersionError struct {
	Requested string
	Selected  string
}

func (e *ProtocolVersionError) Error() string {
	return fmt.Sprintf(
		"mcp: unsupported protocol version: requested %q, server selected %q",
		boundedDiagnostic(e.Requested),
		boundedDiagnostic(e.Selected),
	)
}

type CapabilityError struct{ Capability string }

func (e *CapabilityError) Error() string {
	return fmt.Sprintf("mcp: capability %q was not negotiated", e.Capability)
}

type UnsupportedFeatureError struct{ Feature string }

func (e *UnsupportedFeatureError) Error() string {
	return fmt.Sprintf("mcp: unsupported feature %q", e.Feature)
}

type StaleDiscoveryError struct {
	Kind       InvalidationKind
	Discovered uint64
	Current    uint64
}

func (e *StaleDiscoveryError) Error() string {
	return fmt.Sprintf(
		"mcp: stale %s discovery snapshot: discovered at generation %d, current generation %d",
		e.Kind,
		e.Discovered,
		e.Current,
	)
}

type InvalidPayloadError struct {
	Subject string
	Err     error
}

func (e *InvalidPayloadError) Error() string {
	detail := "unknown error"
	if e.Err != nil {
		detail = e.Err.Error()
	}
	return fmt.Sprintf("mcp: invalid %s payload: %s", boundedDiagnostic(e.Subject), boundedDiagnostic(detail))
}

func (e *InvalidPayloadError) Unwrap() error { return e.Err }

type RPCError struct {
	Code    JSONNumber
	Message string
	Data    json.RawMessage
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("mcp: JSON-RPC error %s: %s", boundedDiagnostic(e.Code.String()), boundedDiagnostic(e.Message))
}

type HTTPError struct {
	StatusCode int
	Operation  string
}

type TransportCrashError struct {
	Transport string
	Err       error
}

func (e *TransportCrashError) Error() string {
	detail := "unknown error"
	if e.Err != nil {
		detail = e.Err.Error()
	}
	return fmt.Sprintf(
		"mcp: %s transport crashed: %s",
		boundedDiagnostic(e.Transport),
		boundedDiagnostic(detail),
	)
}

func (e *TransportCrashError) Unwrap() error { return e.Err }

func (e *HTTPError) Error() string {
	return fmt.Sprintf("mcp: HTTP %s failed with status %d", e.Operation, e.StatusCode)
}

type RemoteToolError struct {
	ToolName string
	Result   CallToolResult
}

func (e *RemoteToolError) Error() string {
	if e.ToolName == "" {
		return "mcp: remote tool execution failed"
	}
	return fmt.Sprintf("mcp: remote tool %q execution failed", e.ToolName)
}
