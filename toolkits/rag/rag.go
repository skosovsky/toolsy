package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/format"
)

type searchArgs struct {
	Query string `json:"query"`
}

type SearchMarkdownWire struct {
	Results string `json:"results"`
}

type SearchDocumentsWire struct {
	Documents []Document `json:"documents"`
}

// AsSearchTool builds a toolsy.Tool that calls r.Retrieve and formats results per options.
func AsSearchTool(r DocumentRetriever, opts ...Option) (toolsy.Tool, error) {
	if nilRetriever(r) {
		return nil, errors.New("toolkit/rag: DocumentRetriever is nil")
	}
	var o options
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.New("toolkit/rag: nil option")
		}
		opt(&o)
	}
	if o.maxBytes < 0 || o.maxResults < 0 || o.maxItemBytes < 0 || o.maxSourceBytes < 0 {
		return nil, errors.New("toolkit/rag: limits must not be negative")
	}
	o.applyDefaults()

	return toolsy.NewTool[searchArgs, format.JSONResult](
		o.name,
		o.description,
		func(ctx context.Context, _ *toolsy.RunEnv, args searchArgs) (format.JSONResult, error) {
			docs, err := retrieveAndFilter(ctx, r, &o, args.Query)
			if err != nil {
				return format.JSONResult{}, err
			}
			raw, err := encodeSearchResult(ctx, docs, &o)
			if err != nil {
				return format.JSONResult{}, err
			}
			return format.JSONResult{Raw: raw}, nil
		},
		toolsy.WithReadOnly(),
	)
}

func searchCancellation(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return toolsy.NewInternalError(errors.Join(err, context.Cause(ctx)))
	}
	return nil
}

func encodeSearchResult(ctx context.Context, docs []Document, o *options) (json.RawMessage, error) {
	if canceled := searchCancellation(ctx); canceled != nil {
		return nil, canceled
	}
	formatter := o.resultFormatter
	if formatter != nil {
		formatter = func(d []Document) (any, error) {
			if canceled := searchCancellation(ctx); canceled != nil {
				return nil, canceled
			}
			out, err := o.resultFormatter(d)
			if canceled := searchCancellation(ctx); canceled != nil {
				return nil, canceled
			}
			return out, err
		}
	}
	validator := o.hostResultValidator
	if validator != nil {
		validator = func(out any) error {
			if canceled := searchCancellation(ctx); canceled != nil {
				return canceled
			}
			err := o.hostResultValidator(out)
			if canceled := searchCancellation(ctx); canceled != nil {
				return canceled
			}
			return err
		}
	}
	raw, err := format.ApplyWithEnvelope(docs, func(d []Document) any {
		if o.resultShape == ShapeDocumentsJSON {
			return SearchDocumentsWire{Documents: d}
		}
		return SearchMarkdownWire{Results: FormatDocumentsMarkdown(d)}
	}, formatter, validator, o.maxBytes)
	if canceled := searchCancellation(ctx); canceled != nil {
		return nil, canceled
	}
	return raw, err
}

func retrieveAndFilter(
	ctx context.Context,
	r DocumentRetriever,
	o *options,
	query string,
) ([]Document, error) {
	if err := searchCancellation(ctx); err != nil {
		return nil, err
	}
	docs, err := r.Retrieve(ctx, query)
	if canceled := searchCancellation(ctx); canceled != nil {
		return nil, canceled
	}
	if err != nil {
		return nil, toolsy.NewInternalError(fmt.Errorf("toolkit/rag: retrieve failed: %w", err))
	}
	if boundsErr := checkDocuments(ctx, docs, o); boundsErr != nil {
		return nil, boundsErr
	}
	if o.scopeFilter != nil {
		docs = o.scopeFilter(ctx, docs)
		if canceled := searchCancellation(ctx); canceled != nil {
			return nil, canceled
		}
		if boundsErr := checkDocuments(ctx, docs, o); boundsErr != nil {
			return nil, boundsErr
		}
	}
	if o.maxResults > 0 && len(docs) > o.maxResults {
		return nil, toolsy.NewValidationError(
			"retrieval result count exceeds limit; provider has no continuation contract",
		)
	}
	return docs, nil
}

func checkDocuments(ctx context.Context, docs []Document, o *options) error {
	err := validateDocuments(docs, o)
	if canceled := searchCancellation(ctx); canceled != nil {
		return canceled
	}
	return err
}

func nilRetriever(r DocumentRetriever) bool {
	if r == nil {
		return true
	}
	v := reflect.ValueOf(r)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
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
		reflect.String,
		reflect.Struct,
		reflect.UnsafePointer:
		return false
	default:
		return false
	}
}
