package prompts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"unicode/utf8"

	"github.com/skosovsky/toolsy"
)

// Document is rendered text with optional host-provider provenance. Provenance
// conveys neither verified identity nor authority to execute the instructions.
type Document struct {
	Instructions string `json:"instructions"`
	Source       string `json:"source,omitempty"`
	Version      string `json:"version,omitempty"`
}

// Provider is a trusted host-selected port. Implementations enforce source access,
// transport/render bounds and context cancellation before returning a document.
type Provider interface {
	Get(ctx context.Context, roleID string, variables map[string]any) (Document, error)
}

type getArgs struct {
	RoleID    string         `json:"role_id"`
	Variables map[string]any `json:"variables,omitempty"`
}

// AsTool exposes provider documents as data; it does not install system messages.
func AsTool(p Provider, opts ...Option) (toolsy.Tool, error) {
	if nilProvider(p) {
		return nil, errors.New("toolkit/prompts: provider is required")
	}
	var o options
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.New("toolkit/prompts: nil option")
		}
		opt(&o)
	}
	o.applyDefaults()
	if o.maxBytes <= 0 || o.maxSourceBytes <= 0 || o.maxProvenanceBytes <= 0 || o.maxOutputBytes <= 0 {
		return nil, errors.New("toolkit/prompts: limits must be positive")
	}
	handler := func(ctx context.Context, _ *toolsy.RunEnv, args getArgs) (Document, error) {
		document, err := p.Get(ctx, args.RoleID, args.Variables)
		if err != nil {
			return Document{}, toolsy.NewInternalError(fmt.Errorf("toolkit/prompts: get failed: %w", err))
		}
		if err := validateDocument(document, o); err != nil {
			return Document{}, err
		}
		return document, nil
	}
	return toolsy.NewTool(o.name, o.description, handler, toolsy.WithReadOnly())
}

func nilProvider(p Provider) bool {
	if p == nil {
		return true
	}
	value := reflect.ValueOf(p)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func validateDocument(document Document, o options) error {
	remaining := o.maxSourceBytes
	for _, field := range []string{document.Instructions, document.Source, document.Version} {
		if !utf8.ValidString(field) {
			return toolsy.NewInternalError(errors.New("toolkit/prompts: provider returned invalid UTF-8"))
		}
		if len(field) > remaining {
			return toolsy.NewValidationError("prompt source byte limit exceeded")
		}
		remaining -= len(field)
	}
	if len(document.Instructions) > o.maxBytes || len(document.Source) > o.maxProvenanceBytes ||
		len(document.Version) > o.maxProvenanceBytes {
		return toolsy.NewValidationError("prompt field byte limit exceeded")
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return toolsy.NewInternalError(err)
	}
	if len(raw) > o.maxOutputBytes {
		return toolsy.NewValidationError("prompt response byte limit exceeded")
	}
	return nil
}
