// Package grpc provides a gRPC Server Reflection to toolsy.Tool translator: given an existing
// [grpc.ClientConnInterface], use reflection to return one toolsy.Tool per RPC method (no .proto at runtime).
//
// RPC responses are encoded as complete protojson documents. MaxResponseBytes limits
// encoded bytes; an oversized response returns a validation error without yielding
// a result. The remote RPC may already have performed side effects before this check.
package grpc
