package toolsy

import (
	"fmt"
	"maps"
)

// NoSubject marks calls that intentionally do not bind a policy subject.
type NoSubject struct{}

// NoScope marks calls that intentionally do not bind a policy scope.
type NoScope struct{}

// CallMetadata is immutable metadata attached to one tool call.
type CallMetadata struct {
	CallID   string
	ToolName string
	ViewID   string
	Tags     []string
}

// CallContext carries typed subject/scope values through the execution pipeline.
// It is intentionally type-erased at the Registry boundary; typed handlers recover
// compile-time types with TypedContext.
type CallContext struct {
	Subject  any
	Scope    any
	Metadata CallMetadata
	Values   map[string]any
}

// TypedCallContext is the compile-time view used by typed tools and typed policy.
type TypedCallContext[TSubject, TScope any] struct {
	Subject  TSubject
	Scope    TScope
	Metadata CallMetadata
	Values   map[string]any
}

// CallContextOption configures a CallContext.
type CallContextOption func(*CallContext)

// WithCallValue attaches request-local metadata that must not be persisted as session state.
func WithCallValue(key string, value any) CallContextOption {
	return func(c *CallContext) {
		if c.Values == nil {
			c.Values = make(map[string]any)
		}
		c.Values[key] = value
	}
}

// WithCallMetadata sets immutable metadata on a CallContext.
func WithCallMetadata(meta CallMetadata) CallContextOption {
	return func(c *CallContext) {
		c.Metadata = cloneCallMetadata(meta)
	}
}

// NewCallContext creates a type-erased execution context from host-owned types.
func NewCallContext[TSubject, TScope any](subject TSubject, scope TScope, opts ...CallContextOption) CallContext {
	c := CallContext{
		Subject: subject,
		Scope:   scope,
		Metadata: CallMetadata{
			CallID:   "",
			ToolName: "",
			ViewID:   "",
			Tags:     nil,
		},
		Values: nil,
	}
	for _, opt := range opts {
		opt(&c)
	}
	return cloneCallContext(c)
}

// TypedContext converts a type-erased CallContext into a compile-time view.
func TypedContext[TSubject, TScope any](c CallContext) (TypedCallContext[TSubject, TScope], error) {
	var out TypedCallContext[TSubject, TScope]
	subject, ok := c.Subject.(TSubject)
	if !ok {
		var zero TSubject
		if _, zeroOK := any(zero).(NoSubject); zeroOK && c.Subject == nil {
			subject = zero
			ok = true
		}
	}
	if !ok {
		return out, NewValidationError(fmt.Sprintf("call context subject is not %T", out.Subject))
	}
	scope, ok := c.Scope.(TScope)
	if !ok {
		var zero TScope
		if _, zeroOK := any(zero).(NoScope); zeroOK && c.Scope == nil {
			scope = zero
			ok = true
		}
	}
	if !ok {
		return out, NewValidationError(fmt.Sprintf("call context scope is not %T", out.Scope))
	}
	out.Subject = subject
	out.Scope = scope
	out.Metadata = cloneCallMetadata(c.Metadata)
	out.Values = maps.Clone(c.Values)
	return out, nil
}

// CallContext returns the typed execution context bound to this environment.
func (e *RunEnv) CallContext() CallContext {
	if e == nil {
		return CallContext{}
	}
	return cloneCallContext(e.callContext)
}

func cloneCallContext(c CallContext) CallContext {
	return CallContext{
		Subject:  c.Subject,
		Scope:    c.Scope,
		Metadata: cloneCallMetadata(c.Metadata),
		Values:   maps.Clone(c.Values),
	}
}

func cloneCallMetadata(m CallMetadata) CallMetadata {
	out := m
	out.Tags = append([]string(nil), m.Tags...)
	return out
}

func bindCallMetadata(c CallContext, call ToolCall) CallContext {
	c = cloneCallContext(c)
	c.Metadata.CallID = call.Input.CallID
	c.Metadata.ToolName = call.ToolName
	c.Metadata.ViewID = ""
	return c
}

func bindViewMetadata(c CallContext, viewID string) CallContext {
	c = cloneCallContext(c)
	c.Metadata.ViewID = viewID
	return c
}
