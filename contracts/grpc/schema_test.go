package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/test/bufconn"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

func fixture(t *testing.T, recursive, streaming bool) protoreflect.FileDescriptor {
	t.Helper()
	fields := []*descriptorpb.FieldDescriptorProto{
		{
			Name:   new("large_count"),
			Number: new(int32(1)),
			Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
		},
		{
			Name:     new("status"),
			Number:   new(int32(2)),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
			TypeName: new(".fixture.Status"),
		},
	}
	if recursive {
		fields = append(
			fields,
			&descriptorpb.FieldDescriptorProto{
				Name:     new("next"),
				Number:   new(int32(3)),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				TypeName: new(".fixture.Record"),
			},
		)
	}
	fd, err := protodesc.NewFile(
		&descriptorpb.FileDescriptorProto{
			Syntax:  new("proto3"),
			Name:    new("fixture.proto"),
			Package: new("fixture"),
			EnumType: []*descriptorpb.EnumDescriptorProto{
				{
					Name: new("Status"),
					Value: []*descriptorpb.EnumValueDescriptorProto{
						{Name: new("UNKNOWN"), Number: new(int32(0))},
						{Name: new("READY"), Number: new(int32(1))},
					},
				},
			},
			MessageType: []*descriptorpb.DescriptorProto{
				{Name: new("Record"), Field: fields},
			},
			Service: []*descriptorpb.ServiceDescriptorProto{
				{
					Name: new("Service"),
					Method: []*descriptorpb.MethodDescriptorProto{
						{
							Name:            new("Call"),
							InputType:       new(".fixture.Record"),
							OutputType:      new(".fixture.Record"),
							ServerStreaming: new(streaming),
						},
					},
				},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return fd
}

//nolint:gocognit // Conformance compares independent schema, decoder and encoder outcomes for each fixture.
func TestProjectionProtoJSONConformance(t *testing.T) {
	// Arrange: real descriptors, published schema and real ProtoJSON decoder.
	md := fixture(t, false, false).Messages().Get(0)
	schema, err := projectMessage(md)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := jsonschemax.Compile(schema)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		input string
		valid bool
	}{
		{`{"largeCount":"9223372036854775807","status":"READY"}`, true},
		{`{"largeCount":"-9223372036854775808","status":123}`, true},
		{`{"largeCount":"9223372036854775808"}`, false},
		{`{"largeCount":"-9223372036854775809"}`, false},
		{`{"status":"MISSING"}`, false}, {`{"extra":true}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			// Act.
			value, decodeErr := jsonschemax.Decode([]byte(tc.input))
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			schemaErr := validator.Validate(value)
			msg := dynamicpb.NewMessage(md)
			protoErr := protojson.Unmarshal([]byte(tc.input), msg)
			// Assert.
			if (schemaErr == nil) != tc.valid || (protoErr == nil) != tc.valid {
				t.Fatalf("schema=%v protobuf=%v want valid=%v", schemaErr, protoErr, tc.valid)
			}
			if tc.valid {
				output, marshalErr := protojson.Marshal(msg)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				decoded, decodeErr := jsonschemax.Decode(output)
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				if err := validator.Validate(decoded); err != nil {
					t.Fatalf("actual output %s: %v", output, err)
				}
			}
		})
	}
}
func TestProjectionUnsupportedTypes(t *testing.T) {
	// Arrange/Act/Assert: no WKT is silently projected as an ordinary object.
	_, err := projectMessage(timestamppb.File_google_protobuf_timestamp_proto.Messages().Get(0))
	if _, ok := errors.AsType[*UnsupportedError](err); !ok {
		t.Fatalf("expected typed rejection: %v", err)
	}
}
func TestRecursiveDiscoveryBoundedSubprocess(t *testing.T) {
	if os.Getenv("TOOLSY_GRPC_RECURSIVE_CHILD") == "1" {
		files := new(protoregistry.Files)
		if err := files.RegisterFile(fixture(t, true, false)); err != nil {
			t.Fatal(err)
		}
		conn := reflectionFixture(t, files)
		_, err := Reflect(t.Context(), conn, Options{Services: []string{"fixture.Service"}})
		if _, ok := errors.AsType[*UnsupportedError](err); !ok {
			t.Fatalf("recursive descriptor: %v", err)
		}
		return
	}
	// Arrange: isolate any accidental stack overflow in the child process.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRecursiveDiscoveryBoundedSubprocess$")
	cmd.Env = append(os.Environ(), "TOOLSY_GRPC_RECURSIVE_CHILD=1")
	// Act.
	output, err := cmd.CombinedOutput()
	// Assert.
	if err != nil {
		t.Fatalf("bounded discovery: %v: %s", err, output)
	}
}
func TestDiscoveryStreamingAndOutputContract(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		// Arrange.
		files := new(protoregistry.Files)
		if err := files.RegisterFile(fixture(t, false, streaming)); err != nil {
			t.Fatal(err)
		}
		// Act.
		tools, err := buildToolsFromRegistry(nil, []string{"fixture.Service"}, files, Options{})
		// Assert.
		if streaming {
			if _, ok := errors.AsType[*UnsupportedError](err); !ok {
				t.Fatalf("streaming published: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(tools[0].Manifest().OutputSchema) == 0 {
			t.Fatal("missing executable output schema")
		}
	}
}

func TestProjectionMapsSharedMessagesAndOneof(t *testing.T) {
	// Arrange: a real descriptor with a map and two references to the same ordinary message.
	source := protodesc.ToFileDescriptorProto(fixture(t, false, false))
	record := source.GetMessageType()[0]
	record.NestedType = []*descriptorpb.DescriptorProto{
		{
			Name:    new("LabelsEntry"),
			Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("key"), Number: new(int32(1)), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
				{Name: new("value"), Number: new(int32(2)), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
			},
		},
	}
	record.Field = append(
		record.Field,
		&descriptorpb.FieldDescriptorProto{
			Name:     new("labels"),
			Number:   new(int32(3)),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
			TypeName: new(".fixture.Record.LabelsEntry"),
		},
		&descriptorpb.FieldDescriptorProto{
			Name:     new("left"),
			Number:   new(int32(4)),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
			TypeName: new(".fixture.Shared"),
		},
		&descriptorpb.FieldDescriptorProto{
			Name:     new("right"),
			Number:   new(int32(5)),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
			TypeName: new(".fixture.Shared"),
		},
		&descriptorpb.FieldDescriptorProto{
			Name:   new("payload"),
			Number: new(int32(6)),
			Type:   descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(),
		},
	)
	source.MessageType = append(
		source.MessageType,
		&descriptorpb.DescriptorProto{
			Name: new("Shared"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("value"), Number: new(int32(1)), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
			},
		},
	)
	fd, err := protodesc.NewFile(source, nil)
	if err != nil {
		t.Fatal(err)
	}
	md := fd.Messages().Get(0)
	schema, err := projectMessage(md)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := jsonschemax.Compile(schema)
	if err != nil {
		t.Fatal(err)
	}
	for index, input := range []string{`{"labels":{"key":"value"},"left":{"value":"a"},"right":{"value":"b"},"payload":"YQ=="}`, `{"labels":{"key":1}}`, `{"payload":"%bad"}`} {
		// Act.
		decoded, decodeErr := jsonschemax.Decode([]byte(input))
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		schemaErr := validator.Validate(decoded)
		protoErr := protojson.Unmarshal([]byte(input), dynamicpb.NewMessage(md))
		// Assert: the repeated ordinary reference is not mistaken for a cycle.
		if (schemaErr == nil) != (protoErr == nil) || (protoErr == nil) != (index == 0) {
			t.Fatalf("%s schema=%v protobuf=%v", input, schemaErr, protoErr)
		}
	}
	// Arrange/Act/Assert: real oneofs are rejected before publication.
	record.OneofDecl = []*descriptorpb.OneofDescriptorProto{{Name: new("choice")}}
	record.Field[0].OneofIndex = new(int32(0))
	record.Field[1].OneofIndex = new(int32(0))
	fd, err = protodesc.NewFile(source, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = projectMessage(fd.Messages().Get(0))
	if _, ok := errors.AsType[*UnsupportedError](err); !ok {
		t.Fatalf("oneof: %v", err)
	}
}

func reflectionFixture(t *testing.T, files *protoregistry.Files) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(testBufconnSize)
	server := grpc.NewServer()
	server.RegisterService(
		&grpc.ServiceDesc{ServiceName: "fixture.Service", HandlerType: (*any)(nil), Metadata: "fixture.proto"},
		struct{}{},
	)
	reflectionpb.RegisterServerReflectionServer(
		server,
		reflection.NewServerV1(reflection.ServerOptions{Services: server, DescriptorResolver: files}),
	)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestProjectionTraversalLimits(t *testing.T) {
	// Arrange: reuse a legal descriptor but start at an exhausted projection budget.
	md := fixture(t, false, false).Messages().Get(0)
	for _, p := range []projection{{active: make(map[protoreflect.FullName]bool), fields: maxProjectionFields}, {active: make(map[protoreflect.FullName]bool), fields: 0}} {
		depth := 0
		if p.fields == 0 {
			depth = maxProjectionDepth + 1
		}
		// Act.
		_, err := p.message(md, depth)
		// Assert.
		if _, ok := errors.AsType[*UnsupportedError](err); !ok {
			t.Fatalf("limit did not reject: %v", err)
		}
	}
}

func TestProjectionEnumExpansionBudget(t *testing.T) {
	for _, tc := range []struct {
		name            string
		entries, fields int
	}{
		{name: "one oversized enum", entries: maxProjectionFields + 1, fields: 1},
		{name: "repeated shared enum", entries: 128, fields: 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: enum entries are expanded per reference, even when the descriptor is shared.
			source := protodesc.ToFileDescriptorProto(fixture(t, false, false))
			values := make([]*descriptorpb.EnumValueDescriptorProto, tc.entries)
			for i := range tc.entries {
				values[i] = &descriptorpb.EnumValueDescriptorProto{
					Name:   new(fmt.Sprintf("VALUE_%d", i)),
					Number: new(int32(i)),
				}
			}
			source.EnumType[0].Value = values
			fields := make([]*descriptorpb.FieldDescriptorProto, tc.fields)
			for i := range tc.fields {
				fields[i] = &descriptorpb.FieldDescriptorProto{
					Name:     new(fmt.Sprintf("value_%d", i)),
					Number:   new(int32(i + 1)),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
					TypeName: new(".fixture.Status"),
				}
			}
			source.MessageType[0].Field = fields
			fd, err := protodesc.NewFile(source, nil)
			if err != nil {
				t.Fatal(err)
			}
			// Act.
			_, err = projectMessage(fd.Messages().Get(0))
			// Assert: rejection occurs in the translator, before schema compilation.
			if rejection, ok := errors.AsType[*UnsupportedError](err); !ok || rejection.Reason != "projection limit" {
				t.Fatalf("enum expansion was not bounded: %v", err)
			}
		})
	}
}
