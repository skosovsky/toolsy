// Package grpc provides a gRPC Server Reflection to toolsy.Tool translator: given an existing
// [grpc.ClientConnInterface], use reflection to return one toolsy.Tool per supported unary RPC method (no .proto at runtime).
//
// Discovery rejects unsupported descriptor shapes with UnsupportedError. See README.md
// for the bounded canonical ProtoJSON subset and construction limits.
//
// RPC responses are encoded as complete protojson documents. MaxResponseBytes limits
// encoded bytes; an oversized response returns a validation error without yielding
// a result. The remote RPC may already have performed side effects before this check.
package grpc
