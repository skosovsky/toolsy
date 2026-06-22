package toolsy

import (
	"context"
	"maps"
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
	out := maps.Clone(in)
	for k, v := range out {
		out[k] = deepCloneValue(v)
	}
	return out
}

func deepCloneValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		return deepCloneMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = deepCloneValue(typed[i])
		}
		return out
	default:
		return cloneMutableValue(v)
	}
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
	if !v.IsValid() {
		return v
	}
	switch v.Kind() {
	case reflect.Interface:
		return cloneReflectInterface(v)
	case reflect.Pointer:
		return cloneReflectPointer(v)
	case reflect.Map:
		return cloneReflectMap(v)
	case reflect.Slice:
		return cloneReflectSlice(v)
	case reflect.Array:
		return cloneReflectArray(v)
	default:
		return v
	}
}

func cloneReflectInterface(v reflect.Value) reflect.Value {
	if v.IsNil() {
		return reflect.Zero(v.Type())
	}
	elem := cloneReflectValue(v.Elem())
	if !elem.IsValid() || !elem.Type().AssignableTo(v.Type()) {
		return v
	}
	out := reflect.New(v.Type()).Elem()
	out.Set(elem)
	return out
}

func cloneReflectPointer(v reflect.Value) reflect.Value {
	if v.IsNil() {
		return reflect.Zero(v.Type())
	}
	elem := cloneReflectValue(v.Elem())
	out := reflect.New(v.Type().Elem())
	if elem.IsValid() && elem.Type().AssignableTo(v.Type().Elem()) {
		out.Elem().Set(elem)
	}
	return out
}

func cloneReflectMap(v reflect.Value) reflect.Value {
	if v.IsNil() {
		return reflect.Zero(v.Type())
	}
	out := reflect.MakeMapWithSize(v.Type(), v.Len())
	iter := v.MapRange()
	for iter.Next() {
		out.SetMapIndex(iter.Key(), cloneReflectValue(iter.Value()))
	}
	return out
}

func cloneReflectSlice(v reflect.Value) reflect.Value {
	if v.IsNil() {
		return reflect.Zero(v.Type())
	}
	out := reflect.MakeSlice(v.Type(), v.Len(), v.Cap())
	for i := range v.Len() {
		out.Index(i).Set(cloneReflectValue(v.Index(i)))
	}
	return out
}

func cloneReflectArray(v reflect.Value) reflect.Value {
	out := reflect.New(v.Type()).Elem()
	for i := range v.Len() {
		out.Index(i).Set(cloneReflectValue(v.Index(i)))
	}
	return out
}
