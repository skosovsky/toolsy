package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

const maxDiagnosticBytes = 256

const (
	missingCapabilityErrorDataSubject  = "missing-required-client-capability error data"
	unsupportedVersionErrorDataSubject = "unsupported-protocol-version error data"
)

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
	ErrTransportClosed   = errors.New("mcp: transport closed")
	ErrProtocolViolation = errors.New("mcp: protocol violation")
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

func (e *InvalidPayloadError) Is(target error) bool { return target == ErrProtocolViolation }

type RPCError struct {
	Code    JSONNumber
	Message string
	Data    json.RawMessage
}

// HeaderMismatchError is the typed form of MCP error -32020.
type HeaderMismatchError struct{ Message string }

func (e *HeaderMismatchError) Error() string {
	return fmt.Sprintf("mcp: request headers do not match the request body: %s", boundedDiagnostic(e.Message))
}

// MissingRequiredClientCapabilityError is the typed form of MCP error -32021.
type MissingRequiredClientCapabilityError struct {
	Message              string
	RequiredCapabilities ClientCapabilities
}

func (e *MissingRequiredClientCapabilityError) Error() string {
	return fmt.Sprintf("mcp: request requires an undeclared client capability: %s", boundedDiagnostic(e.Message))
}

// UnsupportedProtocolVersionError is the typed form of MCP error -32022.
type UnsupportedProtocolVersionError struct {
	Message   string
	Requested string
	Supported []string
}

func (e *UnsupportedProtocolVersionError) Error() string {
	return fmt.Sprintf(
		"mcp: unsupported protocol version %q: %s",
		boundedDiagnostic(e.Requested),
		boundedDiagnostic(e.Message),
	)
}

// TypedRPCError converts MCP-reserved JSON-RPC failures into stable error
// types. Malformed reserved error data remains a protocol violation.
func TypedRPCError(rpc *RPCError) error {
	if rpc == nil {
		return nil
	}
	switch rpc.Code {
	case JSONRPCHeaderMismatch:
		return &HeaderMismatchError{Message: rpc.Message}
	case JSONRPCMissingRequiredClientCapability:
		fields, err := decodeObjectFields(rpc.Data)
		if err != nil {
			return &InvalidPayloadError{Subject: missingCapabilityErrorDataSubject, Err: err}
		}
		if err = rejectUnknownFields(fields, "requiredCapabilities"); err != nil {
			return &InvalidPayloadError{Subject: missingCapabilityErrorDataSubject, Err: err}
		}
		if err = required(fields, "requiredCapabilities"); err != nil {
			return &InvalidPayloadError{Subject: missingCapabilityErrorDataSubject, Err: err}
		}
		var data struct {
			RequiredCapabilities ClientCapabilities `json:"requiredCapabilities"`
		}
		if err = json.Unmarshal(rpc.Data, &data); err != nil {
			return &InvalidPayloadError{Subject: missingCapabilityErrorDataSubject, Err: err}
		}
		return &MissingRequiredClientCapabilityError{
			Message:              rpc.Message,
			RequiredCapabilities: data.RequiredCapabilities,
		}
	case JSONRPCUnsupportedProtocolVersion:
		fields, err := decodeObjectFields(rpc.Data)
		if err != nil {
			return &InvalidPayloadError{Subject: unsupportedVersionErrorDataSubject, Err: err}
		}
		if err = rejectUnknownFields(fields, "requested", "supported"); err != nil {
			return &InvalidPayloadError{Subject: unsupportedVersionErrorDataSubject, Err: err}
		}
		var data struct {
			Requested string   `json:"requested"`
			Supported []string `json:"supported"`
		}
		if err = json.Unmarshal(rpc.Data, &data); err != nil || data.Requested == "" || len(data.Supported) == 0 {
			if err == nil {
				err = errors.New("requested and supported are required")
			}
			return &InvalidPayloadError{Subject: unsupportedVersionErrorDataSubject, Err: err}
		}
		return &UnsupportedProtocolVersionError{
			Message:   rpc.Message,
			Requested: data.Requested,
			Supported: append([]string(nil), data.Supported...),
		}
	case JSONRPCParseError,
		JSONRPCInvalidRequest,
		JSONRPCMethodNotFound,
		JSONRPCInvalidParams,
		JSONRPCInternalError:
		return rpc
	default:
		return rpc
	}
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("mcp: JSON-RPC error %s: %s", boundedDiagnostic(e.Code.String()), boundedDiagnostic(e.Message))
}

type HTTPError struct {
	StatusCode          int
	Operation           string
	Challenges          []HTTPAuthChallenge
	ChallengesTruncated bool
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

func (e HTTPError) Error() string {
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
