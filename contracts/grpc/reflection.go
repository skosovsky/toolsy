package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/skosovsky/toolsy"
)

// Reflect uses gRPC server reflection on an existing client connection to discover services/methods
// and returns one toolsy.Tool per RPC method. The caller must create cc (e.g. grpc.NewClient) and
// is responsible for closing it. cc must not be nil.
func Reflect(ctx context.Context, cc grpc.ClientConnInterface, opts Options) ([]toolsy.Tool, error) {
	if cc == nil {
		return nil, errors.New("grpc: ClientConn is nil")
	}
	refClient := reflectionpb.NewServerReflectionClient(cc)
	stream, err := refClient.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("grpc: reflection stream: %w", err)
	}
	svcNames, files, err := listServicesAndBuildFiles(stream)
	if err != nil {
		_ = stream.CloseSend()
		return nil, err
	}
	_ = stream.CloseSend()
	return buildToolsFromRegistry(cc, svcNames, files, opts)
}

func buildToolsFromRegistry(
	cc grpc.ClientConnInterface,
	svcNames []string,
	files *protoregistry.Files,
	opts Options,
) ([]toolsy.Tool, error) {
	allowedServices := make(map[string]bool)
	for _, s := range opts.Services {
		allowedServices[s] = true
	}

	var tools []toolsy.Tool
	usedNames := make(map[string]bool)

	for _, svcName := range svcNames {
		if strings.HasPrefix(svcName, "grpc.") {
			continue
		}
		if len(allowedServices) > 0 && !allowedServices[svcName] {
			continue
		}
		desc, err := files.FindDescriptorByName(protoreflect.FullName(svcName))
		if err != nil {
			continue
		}
		svcDesc, ok := desc.(protoreflect.ServiceDescriptor)
		if !ok {
			continue
		}
		for i, n := 0, svcDesc.Methods().Len(); i < n; i++ {
			m := svcDesc.Methods().Get(i)
			name := methodToolName(svcName, string(m.Name()), usedNames)
			inputDesc := m.Input()
			schemaBytes, err := descriptorToJSONSchema(inputDesc)
			if err != nil {
				return nil, fmt.Errorf("grpc: schema %s/%s: %w", svcName, m.Name(), err)
			}
			descStr := "gRPC " + svcName + "/" + string(m.Name())
			mCopy := m
			optsCopy := opts
			ccCopy := cc
			tool, err := toolsy.NewProxyTool(
				name,
				descStr,
				schemaBytes,
				func(ctx context.Context, _ toolsy.RunContext, argsJSON []byte, yield func(toolsy.Chunk) error) error {
					return invokeRPC(ctx, ccCopy, mCopy, argsJSON, &optsCopy, yield)
				},
			)
			if err != nil {
				return nil, fmt.Errorf("grpc: tool %s: %w", name, err)
			}
			tools = append(tools, tool)
		}
	}
	if len(tools) == 0 {
		return nil, errors.New("grpc: reflection: no tools generated (check service allowlist and reflection)")
	}
	return tools, nil
}

// listServicesAndBuildFiles sends ListServicesRequest, then FileContainingSymbol for each service, and returns service names and a merged Files registry.
func listServicesAndBuildFiles(
	stream reflectionpb.ServerReflection_ServerReflectionInfoClient,
) ([]string, *protoregistry.Files, error) {
	// Request list of services.
	req := &reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{
			ListServices: "",
		},
	}
	if err := stream.Send(req); err != nil {
		return nil, nil, err
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, nil, err
	}
	listResp := resp.GetListServicesResponse()
	if listResp == nil {
		if errResp := resp.GetErrorResponse(); errResp != nil {
			return nil, nil, fmt.Errorf("grpc: reflection list_services error: %s", errResp.GetErrorMessage())
		}
		return nil, nil, errors.New("grpc: reflection: unexpected response type")
	}
	var svcNames []string
	for _, s := range listResp.GetService() {
		if s != nil && s.GetName() != "" {
			svcNames = append(svcNames, s.GetName())
		}
	}
	if len(svcNames) == 0 {
		return nil, nil, errors.New("grpc: reflection: no services")
	}
	// Build registry from FileContainingSymbol for each service.
	files, err := buildRegistry(stream, svcNames)
	if err != nil {
		return nil, nil, err
	}
	return svcNames, files, nil
}

// buildRegistry collects FileDescriptorProto for each service via FileContainingSymbol,
// deduplicates by file name, and returns a merged protoregistry.Files.
func buildRegistry(
	stream reflectionpb.ServerReflection_ServerReflectionInfoClient,
	services []string,
) (*protoregistry.Files, error) {
	seenFiles := make(map[string]bool)
	var allFiles []*descriptorpb.FileDescriptorProto

	for _, svc := range services {
		req := &reflectionpb.ServerReflectionRequest{
			MessageRequest: &reflectionpb.ServerReflectionRequest_FileContainingSymbol{
				FileContainingSymbol: svc,
			},
		}
		if err := stream.Send(req); err != nil {
			return nil, err
		}
		resp, err := stream.Recv()
		if err != nil {
			return nil, err
		}
		fdResp := resp.GetFileDescriptorResponse()
		if fdResp == nil {
			if errResp := resp.GetErrorResponse(); errResp != nil {
				return nil, fmt.Errorf("grpc: reflection file_containing_symbol error: %s", errResp.GetErrorMessage())
			}
			continue
		}
		for _, fdBytes := range fdResp.GetFileDescriptorProto() {
			fd := &descriptorpb.FileDescriptorProto{}
			if err := proto.Unmarshal(fdBytes, fd); err != nil {
				return nil, err
			}
			if !seenFiles[fd.GetName()] {
				seenFiles[fd.GetName()] = true
				allFiles = append(allFiles, fd)
			}
		}
	}
	fdSet := &descriptorpb.FileDescriptorSet{File: allFiles}
	return protodesc.NewFiles(fdSet)
}

func descriptorToJSONSchema(md protoreflect.MessageDescriptor) ([]byte, error) {
	schemaMap := buildMessageSchema(md)
	return json.Marshal(schemaMap)
}

// buildMessageSchema recursively builds a JSON Schema map for a message (no marshal/unmarshal).
func buildMessageSchema(md protoreflect.MessageDescriptor) map[string]any {
	props := make(map[string]any)
	for i, n := 0, md.Fields().Len(); i < n; i++ {
		f := md.Fields().Get(i)
		props[string(f.Name())] = fieldToJSONSchema(f)
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
	}
}

// fieldToJSONSchema builds JSON Schema for one field (repeated -> array, map -> additionalProperties).
func fieldToJSONSchema(fd protoreflect.FieldDescriptor) map[string]any {
	var baseSchema map[string]any
	switch fd.Kind() {
	case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Uint32Kind, protoreflect.Uint64Kind,
		protoreflect.Sint32Kind, protoreflect.Sint64Kind, protoreflect.Fixed32Kind, protoreflect.Fixed64Kind,
		protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
		baseSchema = map[string]any{"type": "integer"}
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		baseSchema = map[string]any{"type": "number"}
	case protoreflect.BoolKind:
		baseSchema = map[string]any{"type": "boolean"}
	case protoreflect.StringKind, protoreflect.BytesKind, protoreflect.EnumKind:
		baseSchema = map[string]any{"type": "string"}
	case protoreflect.MessageKind, protoreflect.GroupKind:
		subMd := fd.Message()
		if subMd == nil {
			baseSchema = map[string]any{"type": "string"}
		} else {
			baseSchema = buildMessageSchema(subMd)
		}
	default:
		baseSchema = map[string]any{"type": "string"}
	}
	if fd.IsMap() {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": fieldToJSONSchema(fd.MapValue()),
		}
	}
	if fd.IsList() {
		return map[string]any{
			"type":  "array",
			"items": baseSchema,
		}
	}
	return baseSchema
}
