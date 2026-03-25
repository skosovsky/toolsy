package grpc

import (
	"bytes"
	"context"
	"net"
	"testing"
	"unicode/utf8"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	health "google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/skosovsky/toolsy"
)

const testBufconnSize = 1024 * 1024

func TestInvokeRPCTruncatesOversizedResponse(t *testing.T) {
	listener := bufconn.Listen(testBufconnSize)
	server := grpc.NewServer()
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Stop()

	//nolint:staticcheck // grpc.NewClient is unavailable in the current grpc version used by this module.
	conn, err := grpc.DialContext(
		context.Background(),
		"bufconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	service := grpc_health_v1.File_grpc_health_v1_health_proto.Services().ByName(protoreflect.Name("Health"))
	if service == nil {
		t.Fatal("health service descriptor not found")
	}
	method := service.Methods().ByName(protoreflect.Name("Check"))
	if method == nil {
		t.Fatal("health check method descriptor not found")
	}

	var got toolsy.Chunk
	err = invokeRPC(
		context.Background(),
		conn,
		method,
		[]byte(`{"service":""}`),
		&Options{MaxResponseBytes: 5},
		func(c toolsy.Chunk) error {
			got = c
			return nil
		},
	)
	if err != nil {
		t.Fatalf("invokeRPC returned error: %v", err)
	}

	full, err := protojson.Marshal(&grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	expected := append(append([]byte(nil), full[:5]...), []byte(truncationSuffix)...)
	if !bytes.Equal(got.Data, expected) {
		t.Fatalf("unexpected body: got %q want %q", got.Data, expected)
	}
	if got.Event != toolsy.EventResult {
		t.Fatalf("unexpected event: %s", got.Event)
	}
}

func TestInvokeRPCTruncatesOversizedResponseUTF8Safely(t *testing.T) {
	conn, method := newDynamicBufconnMethod(t, "приветмир")
	defer func() { _ = conn.Close() }()

	var got toolsy.Chunk
	err := invokeRPC(
		context.Background(),
		conn,
		method,
		[]byte(`{"name":"demo"}`),
		&Options{MaxResponseBytes: 14},
		func(c toolsy.Chunk) error {
			got = c
			return nil
		},
	)
	if err != nil {
		t.Fatalf("invokeRPC returned error: %v", err)
	}

	if !utf8.Valid(got.Data) {
		t.Fatalf("response must remain valid UTF-8: %q", got.Data)
	}
	expected := []byte(`{"message":"п` + truncationSuffix)
	if !bytes.Equal(got.Data, expected) {
		t.Fatalf("unexpected body: got %q want %q", got.Data, expected)
	}
}

func newDynamicBufconnMethod(t *testing.T, reply string) (*grpc.ClientConn, protoreflect.MethodDescriptor) {
	t.Helper()

	syntax := "proto3"
	fileName := "dynamic_demo.proto"
	packageName := "dynamicdemo"
	requestName := "EchoRequest"
	requestFieldName := "name"
	responseName := "EchoResponse"
	responseFieldName := "message"
	serviceName := "DynamicDemo"
	methodName := "Echo"
	inputType := ".dynamicdemo.EchoRequest"
	outputType := ".dynamicdemo.EchoResponse"
	fieldNumber := int32(1)

	fileDesc, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Syntax:  &syntax,
		Name:    &fileName,
		Package: &packageName,
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: &requestName,
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   &requestFieldName,
						Number: &fieldNumber,
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
				},
			},
			{
				Name: &responseName,
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   &responseFieldName,
						Number: &fieldNumber,
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: &serviceName,
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       &methodName,
						InputType:  &inputType,
						OutputType: &outputType,
					},
				},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("build descriptor: %v", err)
	}

	service := fileDesc.Services().ByName("DynamicDemo")
	if service == nil {
		t.Fatal("dynamic service descriptor not found")
	}
	method := service.Methods().ByName("Echo")
	if method == nil {
		t.Fatal("dynamic method descriptor not found")
	}
	inputDesc := method.Input()
	outputDesc := method.Output()

	listener := bufconn.Listen(testBufconnSize)
	server := grpc.NewServer()
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: string(service.FullName()),
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: string(method.Name()),
				Handler: func(
					_ any,
					ctx context.Context,
					dec func(any) error,
					interceptor grpc.UnaryServerInterceptor,
				) (any, error) {
					decodeReq := func() (*dynamicpb.Message, error) {
						req := dynamicpb.NewMessage(inputDesc)
						if decodeErr := dec(req); decodeErr != nil {
							return nil, decodeErr
						}
						return req, nil
					}
					buildResponse := func() *dynamicpb.Message {
						resp := dynamicpb.NewMessage(outputDesc)
						resp.Set(outputDesc.Fields().ByName("message"), protoreflect.ValueOfString(reply))
						return resp
					}
					if interceptor == nil {
						_, decodeErr := decodeReq()
						if decodeErr != nil {
							return nil, decodeErr
						}
						return buildResponse(), nil
					}
					info := &grpc.UnaryServerInfo{
						Server:     struct{}{},
						FullMethod: "/" + string(service.FullName()) + "/" + string(method.Name()),
					}
					req, decodeErr := decodeReq()
					if decodeErr != nil {
						return nil, decodeErr
					}
					return interceptor(ctx, req, info, func(context.Context, any) (any, error) {
						return buildResponse(), nil
					})
				},
			},
		},
	}, struct{}{})
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	//nolint:staticcheck // grpc.NewClient is unavailable in the current grpc version used by this module.
	conn, err := grpc.DialContext(
		context.Background(),
		"bufconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn, method
}
