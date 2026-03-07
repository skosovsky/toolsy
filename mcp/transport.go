package mcp

import "context"

// Transport defines the abstraction for MCP JSON-RPC 2.0 communication.
// It does not know about tools, resources, or prompts—only JSON-RPC method and params/result.
// Implementations must be safe for concurrent use (e.g. parallel Call from ExecuteBatchStream).
type Transport interface {
	// Start starts the transport (e.g. launches child process or opens SSE connection).
	// Must be called before Call or Notify. Idempotent.
	Start(ctx context.Context) error

	// Call sends a JSON-RPC request and blocks until the response is received or context is done.
	// method is the JSON-RPC method name; params is serialized as JSON for the "params" field.
	// Returns the raw "result" body ([]byte), the request ID used (for notifications/cancelled), or an error.
	Call(ctx context.Context, method string, params any) (result []byte, requestID string, err error)

	// Notify sends a one-way JSON-RPC notification (no response expected).
	Notify(ctx context.Context, method string, params any) error

	// OnNotification registers a handler for incoming notifications with the given method name.
	// Handlers are invoked from the transport's read goroutine; they must not block excessively.
	OnNotification(method string, handler func(params []byte))

	// Close shuts down the transport and releases resources. Unblocks any pending Call.
	Close() error
}
