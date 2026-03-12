package toolsy

import (
	"context"
	"maps"
)

type overrideOptions struct {
	name        *string
	description *string
	parameters  map[string]any
}

// OverrideOption configures OverrideTool.
type OverrideOption func(*overrideOptions)

// WithNewName overrides the tool name.
func WithNewName(name string) OverrideOption {
	return func(o *overrideOptions) {
		o.name = &name
	}
}

// WithNewDescription overrides the tool description.
func WithNewDescription(desc string) OverrideOption {
	return func(o *overrideOptions) {
		o.description = &desc
	}
}

// WithNewParameters overrides the JSON Schema (Parameters). Pass nil to keep the base tool's schema.
// The map is stored as a defensive deep copy so that later mutations of the caller's map (including nested
// properties/required) do not affect the wrapper. Parameters() still returns a shallow copy of the stored schema.
func WithNewParameters(params map[string]any) OverrideOption {
	return func(o *overrideOptions) {
		if params != nil {
			o.parameters = deepCopySchema(params)
		} else {
			o.parameters = nil
		}
	}
}

// deepCopySchema returns a deep copy of a JSON Schema–shaped map (nested maps and slices of primitives/maps).
func deepCopySchema(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopySchemaValue(v)
	}
	return out
}

func deepCopySchemaValue(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case map[string]any:
		return deepCopySchema(x)
	case map[string]string:
		return cloneStringMap(x)
	case []any:
		return deepCopySchemaSlice(x)
	case []string:
		return cloneStringSlice(x)
	case []map[string]any:
		return deepCopySchemaMapSlice(x)
	default:
		return v
	}
}

func cloneStringSlice(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}

func cloneStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		out[k] = val
	}
	return out
}

func deepCopySchemaMapSlice(s []map[string]any) []map[string]any {
	if s == nil {
		return nil
	}
	out := make([]map[string]any, len(s))
	for i, m := range s {
		out[i] = deepCopySchema(m)
	}
	return out
}

func deepCopySchemaSlice(s []any) []any {
	if s == nil {
		return nil
	}
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = deepCopySchemaValue(v)
	}
	return out
}

// OverrideTool returns a tool that delegates to base but overrides Name, Description, and/or Parameters.
// The base tool is not mutated; Execute and ToolMetadata are delegated via embedded toolBase.
func OverrideTool(base Tool, opts ...OverrideOption) Tool {
	var o overrideOptions
	for _, opt := range opts {
		opt(&o)
	}
	return &overriddenTool{
		toolBase: toolBase{next: base},
		opts:     &o,
	}
}

type overriddenTool struct {
	toolBase
	opts *overrideOptions
}

func (t *overriddenTool) Name() string {
	if t.opts.name != nil {
		return *t.opts.name
	}
	return t.next.Name()
}

func (t *overriddenTool) Description() string {
	if t.opts.description != nil {
		return *t.opts.description
	}
	return t.next.Description()
}

// Parameters returns a shallow copy of the override schema when set; otherwise delegates to base (which follows the same contract).
func (t *overriddenTool) Parameters() map[string]any {
	if t.opts.parameters != nil {
		return maps.Clone(t.opts.parameters)
	}
	return t.next.Parameters()
}

func (t *overriddenTool) Execute(ctx context.Context, argsJSON []byte, yield func(Chunk) error) error {
	if t.opts.name != nil {
		alias := *t.opts.name
		origYield := yield
		yield = func(c Chunk) error {
			c.ToolName = alias
			return origYield(c)
		}
	}
	return t.next.Execute(ctx, argsJSON, yield)
}
