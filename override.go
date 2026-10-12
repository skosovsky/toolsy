package toolsy

import (
	"context"
)

type overrideOptions struct {
	name        *string
	description *string
	parameters  map[string]any
}

// OverrideOption configures OverrideTool.
type OverrideOption func(*overrideOptions)

// WithNewName overrides the tool manifest name.
func WithNewName(name string) OverrideOption {
	return func(o *overrideOptions) {
		o.name = &name
	}
}

// WithNewDescription overrides the tool manifest description.
func WithNewDescription(desc string) OverrideOption {
	return func(o *overrideOptions) {
		o.description = &desc
	}
}

// WithNewParameters overrides the JSON Schema parameters map. Pass nil to keep the base schema.
func WithNewParameters(params map[string]any) OverrideOption {
	return func(o *overrideOptions) {
		if params != nil {
			o.parameters = deepCopySchema(params)
		} else {
			o.parameters = nil
		}
	}
}

// deepCopySchema returns a deep copy of a JSON Schema shaped map.
func deepCopySchema(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out, _ := cloneMutableValue(m).(map[string]any)
	return out
}

// OverrideTool returns a tool that delegates to base but overrides manifest fields.
func OverrideTool(base Tool, opts ...OverrideOption) Tool {
	var o overrideOptions
	for _, opt := range opts {
		opt(&o)
	}
	return &overriddenTool{
		next: base,
		opts: &o,
	}
}

type overriddenTool struct {
	toolBase

	opts *overrideOptions
}

func (t *overriddenTool) Manifest() ToolManifest {
	manifest := t.next.Manifest()
	if t.opts.name != nil {
		manifest.Name = *t.opts.name
	}
	if t.opts.description != nil {
		manifest.Description = *t.opts.description
	}
	if t.opts.parameters != nil {
		manifest.Parameters = deepCopySchema(t.opts.parameters)
	}
	return manifest
}

func (t *overriddenTool) Execute(ctx context.Context, run *RunEnv, input ToolInput, yield func(Chunk) error) error {
	if run == nil {
		run = NewRunEnv(nil)
	}
	if run.executionManifest == nil {
		privateRun := *run
		manifest := t.Manifest()
		privateRun.executionManifest = &manifest
		run = &privateRun
	}
	if t.opts.name != nil {
		alias := *t.opts.name
		origYield := yield
		yield = func(c Chunk) error {
			c.ToolName = alias
			return origYield(c)
		}
	}
	return t.next.Execute(ctx, run, input, yield)
}
