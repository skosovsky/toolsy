package toolsy

import (
	"context"
	"reflect"
)

// MemoryAccess describes how a tool uses session in-memory state.
type MemoryAccess string

const (
	MemoryAccessNone      MemoryAccess = "none"
	MemoryAccessRead      MemoryAccess = "read"
	MemoryAccessReadWrite MemoryAccess = "readwrite"
)

// Permission is a host-defined capability label declared on a tool manifest.
type Permission string

// ToolRequirements holds typed declarative requirements for authorization and routing.
// Registry/session policy can enforce these requirements before execution with [NewRequirementsPolicy].
type ToolRequirements struct {
	MemoryAccess MemoryAccess
	NeedsSession bool
	Permissions  []Permission
}

// RequirementsPolicyRequest is the typed request passed to requirements policy.
type RequirementsPolicyRequest[TSubject, TScope any] struct {
	Manifest     ToolManifest
	Requirements ToolRequirements
	Input        ToolInput
	Context      TypedCallContext[TSubject, TScope]
	View         RegistryViewSnapshot
}

// RequirementsDecisionFunc evaluates manifest requirements against a typed subject/scope.
type RequirementsDecisionFunc[TSubject, TScope any] func(
	context.Context,
	RequirementsPolicyRequest[TSubject, TScope],
) Decision

// NewRequirementsPolicy converts manifest requirements into fail-closed registry/session enforcement.
func NewRequirementsPolicy[TSubject, TScope any](
	fn RequirementsDecisionFunc[TSubject, TScope],
) Policy {
	return requirementsPolicy[TSubject, TScope]{fn: fn}
}

type requirementsPolicy[TSubject, TScope any] struct {
	fn RequirementsDecisionFunc[TSubject, TScope]
}

func (p requirementsPolicy[TSubject, TScope]) Decide(ctx context.Context, req PolicyRequest) Decision {
	if p.fn == nil {
		return DenyDecision("requirements policy function is nil")
	}
	if !hasRequirements(req.Manifest.Requirements) {
		return AllowDecision()
	}
	typed, err := TypedContext[TSubject, TScope](req.CallContext)
	if err != nil {
		return DenyDecision(err.Error())
	}
	return p.fn(ctx, RequirementsPolicyRequest[TSubject, TScope]{
		Manifest:     cloneManifestForPolicy(req.Manifest),
		Requirements: cloneRequirements(req.Manifest.Requirements),
		Input:        req.Input.Clone(),
		Context:      typed,
		View:         cloneRegistryViewSnapshot(req.View),
	})
}

func (p requirementsPolicy[TSubject, TScope]) enforcesRequirements() bool {
	return true
}

func hasRequirements(r ToolRequirements) bool {
	return r.NeedsSession ||
		(r.MemoryAccess != "" && r.MemoryAccess != MemoryAccessNone) ||
		len(r.Permissions) > 0
}

// cloneRequirements returns a defensive copy of requirements.
func cloneRequirements(r ToolRequirements) ToolRequirements {
	out := r
	if len(r.Permissions) > 0 {
		out.Permissions = append([]Permission(nil), r.Permissions...)
	}
	return out
}

func cloneManifestForPolicy(m ToolManifest) ToolManifest {
	out := m
	out.Parameters = deepCloneMap(m.Parameters)
	out.OutputSchema = deepCloneMap(m.OutputSchema)
	out.Tags = append([]string(nil), m.Tags...)
	out.Requirements = cloneRequirements(m.Requirements)
	return out
}

func deepCloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out, ok := cloneMutableValue(in).(map[string]any)
	if !ok {
		return nil
	}
	return out
}

func deepCloneValue(v any) any {
	return cloneMutableValue(v)
}

func cloneMutableValue(v any) any {
	if v == nil {
		return v
	}
	cloned := cloneReflectValue(reflect.ValueOf(v))
	if !cloned.IsValid() || !cloned.CanInterface() {
		return v
	}
	return cloned.Interface()
}

func cloneReflectValue(v reflect.Value) reflect.Value {
	return (&valueCloner{seen: make(map[cloneReference]reflect.Value)}).clone(v)
}

type cloneReference struct {
	typeOf  reflect.Type
	pointer uintptr
	length  int
}

type valueCloner struct {
	seen map[cloneReference]reflect.Value
}

// Exported data fields are snapshotted without JSON type erasure. Opaque private
// state, functions and channels remain host-owned and must be immutable during
// execution. Cycles are preserved rather than recursed indefinitely.
func (c *valueCloner) clone(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(c.clone(v.Elem()))
		return out
	case reflect.Pointer, reflect.Map, reflect.Slice:
		return c.cloneReference(v)
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := range v.Len() {
			out.Index(i).Set(c.clone(v.Index(i)))
		}
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				out.Field(i).Set(c.clone(v.Field(i)))
			}
		}
		return out
	case reflect.Invalid,
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.Chan,
		reflect.Func,
		reflect.String,
		reflect.UnsafePointer:
		return v
	default:
		return v
	}
}

func (c *valueCloner) cloneReference(v reflect.Value) reflect.Value {
	if v.IsNil() {
		return reflect.Zero(v.Type())
	}
	key := cloneReference{typeOf: v.Type(), pointer: uintptr(v.UnsafePointer()), length: 0}
	if v.Kind() == reflect.Slice {
		key.length = v.Len()
	}
	if out, exists := c.seen[key]; exists {
		return out
	}
	var out reflect.Value
	switch v.Kind() {
	case reflect.Pointer:
		out = reflect.New(v.Type().Elem())
		c.seen[key] = out
		out.Elem().Set(c.clone(v.Elem()))
	case reflect.Map:
		out = reflect.MakeMapWithSize(v.Type(), v.Len())
		c.seen[key] = out
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), c.clone(iter.Value()))
		}
	case reflect.Slice:
		out = reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		c.seen[key] = out
		for i := range v.Len() {
			out.Index(i).Set(c.clone(v.Index(i)))
		}
	case reflect.Invalid,
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.Array,
		reflect.Chan,
		reflect.Func,
		reflect.Interface,
		reflect.String,
		reflect.Struct,
		reflect.UnsafePointer:
		panic("toolsy: invalid internal snapshot reference kind")
	default:
		panic("toolsy: invalid internal snapshot reference kind")
	}
	return out
}
