package grpc

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/skosovsky/toolsy"
)

const truncationSuffix = "\n[Truncated. Use pagination or filters.]"

func invokeRPC(
	ctx context.Context,
	cc *grpc.ClientConn,
	method protoreflect.MethodDescriptor,
	argsJSON []byte,
	opts *Options,
	yield func(toolsy.Chunk) error,
) error {
	req := dynamicpb.NewMessage(method.Input())
	unmarshaler := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshaler.Unmarshal(argsJSON, req); err != nil {
		return fmt.Errorf("grpc: unmarshal request: %w", err)
	}
	fullName := "/" + string(method.Parent().FullName()) + "/" + string(method.Name())
	resp := dynamicpb.NewMessage(method.Output())
	if err := cc.Invoke(ctx, fullName, req, resp); err != nil {
		return fmt.Errorf("grpc: invoke: %w", err)
	}
	data, err := protojson.Marshal(resp)
	if err != nil {
		return fmt.Errorf("grpc: marshal response: %w", err)
	}
	maxBytes := opts.maxResponseBytes()
	if maxBytes > 0 && len(data) > maxBytes {
		truncated := make([]byte, maxBytes, maxBytes+len(truncationSuffix))
		copy(truncated, data[:maxBytes])
		truncated = append(truncated, truncationSuffix...)
		data = truncated
	}
	return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: data})
}
