package grpc

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

const jsonSchemaMinimumKey = "minimum"
const jsonSchemaMaximumKey = "maximum"
const jsonSchemaPatternKey = "pattern"
const jsonSchemaInteger = "integer"
const maxProjectionDepth = 64
const maxProjectionFields = 2048

// UnsupportedError identifies a descriptor outside the published adapter subset.
type UnsupportedError struct {
	Descriptor string
	Reason     string
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("grpc: unsupported %s: %s", e.Descriptor, e.Reason)
}
func unsupported(name, reason string) error {
	return &UnsupportedError{Descriptor: name, Reason: reason}
}

type projection struct {
	active map[protoreflect.FullName]bool
	fields int
}

func descriptorToJSONSchema(md protoreflect.MessageDescriptor) ([]byte, error) {
	schema, err := projectMessage(md)
	if err != nil {
		return nil, err
	}
	return json.Marshal(schema)
}
func projectMessage(md protoreflect.MessageDescriptor) (map[string]any, error) {
	p := projection{active: make(map[protoreflect.FullName]bool), fields: 0}
	return p.message(md, 0)
}
func (p *projection) message(md protoreflect.MessageDescriptor, depth int) (map[string]any, error) {
	name := string(md.FullName())
	if depth > maxProjectionDepth || p.fields > maxProjectionFields {
		return nil, unsupported(name, "projection limit")
	}
	if p.active[md.FullName()] {
		return nil, unsupported(name, "recursive message")
	}
	if md.Syntax() != protoreflect.Proto3 || strings.HasPrefix(name, "google.protobuf.") ||
		md.ExtensionRanges().Len() > 0 {
		return nil, unsupported(name, "syntax, extensions or well-known type")
	}
	for i := range md.Oneofs().Len() {
		if !md.Oneofs().Get(i).IsSynthetic() {
			return nil, unsupported(name, "oneof")
		}
	}
	p.active[md.FullName()] = true
	defer delete(p.active, md.FullName())
	props := make(map[string]any)
	for i := range md.Fields().Len() {
		p.fields++
		if p.fields > maxProjectionFields {
			return nil, unsupported(name, "projection limit")
		}
		fd := md.Fields().Get(i)
		field, err := p.field(fd, depth+1)
		if err != nil {
			return nil, err
		}
		if _, exists := props[fd.JSONName()]; exists {
			return nil, unsupported(name, "duplicate JSON field name")
		}
		props[fd.JSONName()] = field
	}
	return map[string]any{jsonSchemaTypeKey: jsonSchemaObject, "properties": props, "additionalProperties": false}, nil
}
func (p *projection) field(fd protoreflect.FieldDescriptor, depth int) (map[string]any, error) {
	if fd.IsMap() {
		if fd.MapKey().Kind() != protoreflect.StringKind {
			return nil, unsupported(string(fd.FullName()), "non-string map key")
		}
		value, err := p.scalar(fd.MapValue(), depth)
		if err != nil {
			return nil, err
		}
		return map[string]any{jsonSchemaTypeKey: jsonSchemaObject, "additionalProperties": value}, nil
	}
	value, err := p.scalar(fd, depth)
	if err != nil {
		return nil, err
	}
	if fd.IsList() {
		return map[string]any{jsonSchemaTypeKey: "array", "items": value}, nil
	}
	return value, nil
}
func (p *projection) scalar(fd protoreflect.FieldDescriptor, depth int) (map[string]any, error) {
	switch fd.Kind() {
	case protoreflect.MessageKind:
		return p.message(fd.Message(), depth)
	case protoreflect.BoolKind:
		return map[string]any{jsonSchemaTypeKey: "boolean"}, nil
	case protoreflect.StringKind:
		return map[string]any{jsonSchemaTypeKey: jsonSchemaString}, nil
	case protoreflect.BytesKind:
		return map[string]any{
			jsonSchemaTypeKey:    jsonSchemaString,
			jsonSchemaPatternKey: "^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$",
		}, nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return map[string]any{
			jsonSchemaTypeKey:    jsonSchemaInteger,
			jsonSchemaMinimumKey: int64(math.MinInt32),
			jsonSchemaMaximumKey: int64(math.MaxInt32),
		}, nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return map[string]any{
			jsonSchemaTypeKey:    jsonSchemaInteger,
			jsonSchemaMinimumKey: 0,
			jsonSchemaMaximumKey: uint64(math.MaxUint32),
		}, nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return map[string]any{
			jsonSchemaTypeKey: jsonSchemaString,
			jsonSchemaPatternKey: "^(?:" + decimalRange(
				"9223372036854775807",
			) + "|-(?:" + decimalRange(
				"9223372036854775808",
			) + "))$",
		}, nil
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return map[string]any{
			jsonSchemaTypeKey:    jsonSchemaString,
			jsonSchemaPatternKey: "^(?:" + decimalRange("18446744073709551615") + ")$",
		}, nil
	case protoreflect.EnumKind:
		if string(fd.Enum().FullName()) == "google.protobuf.NullValue" {
			return nil, unsupported(string(fd.FullName()), "NullValue")
		}
		// Charge each expanded enum entry before allocating its schema.
		entries := fd.Enum().Values().Len()
		if entries > maxProjectionFields-p.fields {
			return nil, unsupported(string(fd.FullName()), "projection limit")
		}
		p.fields += entries
		names := make([]any, 0, entries)
		for i := range fd.Enum().Values().Len() {
			names = append(names, string(fd.Enum().Values().Get(i).Name()))
		}
		return map[string]any{
			"anyOf": []any{
				map[string]any{jsonSchemaTypeKey: jsonSchemaString, "enum": names},
				map[string]any{
					jsonSchemaTypeKey:    jsonSchemaInteger,
					jsonSchemaMinimumKey: int64(math.MinInt32),
					jsonSchemaMaximumKey: int64(math.MaxInt32),
				},
			},
		}, nil
	case protoreflect.FloatKind:
		return nil, unsupported(string(fd.FullName()), "float32 rounding contract")
	case protoreflect.DoubleKind:
		limit := math.MaxFloat64
		return map[string]any{
			"anyOf": []any{
				map[string]any{jsonSchemaTypeKey: "number", jsonSchemaMinimumKey: -limit, jsonSchemaMaximumKey: limit},
				map[string]any{jsonSchemaTypeKey: jsonSchemaString, "enum": []any{"NaN", "Infinity", "-Infinity"}},
			},
		}, nil
	default:
		return nil, unsupported(string(fd.FullName()), "field kind "+fd.Kind().String())
	}
}

// decimalRange matches canonical nonnegative decimals at most limit without losing integer precision.
func decimalRange(limit string) string {
	parts := []string{"0"}
	for length := 1; length < len(limit); length++ {
		parts = append(parts, fmt.Sprintf("[1-9][0-9]{%d}", length-1))
	}
	for i := range len(limit) {
		low := byte('0')
		if i == 0 {
			low = '1'
		}
		if limit[i] > low {
			parts = append(parts, fmt.Sprintf("%s[%c-%c][0-9]{%d}", limit[:i], low, limit[i]-1, len(limit)-i-1))
		}
	}
	return strings.Join(append(parts, limit), "|")
}
