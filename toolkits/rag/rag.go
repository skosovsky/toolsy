package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	if r == nil {
		return nil, errors.New("toolkit/rag: DocumentRetriever is nil")
	}
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	o.applyDefaults()

	if wantsJSONTool(&o) {
		return buildJSONSearchTool(r, &o)
	}
	if wantsMarkdownIoCTool(&o) {
		return buildMarkdownIoCTool(r, &o)
	}
	return buildMarkdownSearchTool(r, &o)
}

func wantsJSONTool(o *options) bool {
	return o.resultFormatter != nil || o.resultShape == ShapeDocumentsJSON
}

func wantsMarkdownIoCTool(o *options) bool {
	return o.hostResultValidator != nil
}

func buildJSONSearchTool(r DocumentRetriever, o *options) (toolsy.Tool, error) {
	return toolsy.NewTool[searchArgs, format.JSONResult](
		o.name,
		o.description,
		func(ctx context.Context, _ *toolsy.RunEnv, args searchArgs) (format.JSONResult, error) {
			docs, err := retrieveAndFilter(ctx, r, o, args.Query)
			if err != nil {
				return format.JSONResult{}, err
			}
			raw, applyErr := encodeSearchJSON(docs, o)
			if applyErr != nil {
				return format.JSONResult{}, applyErr
			}
			return format.JSONResult{Raw: raw}, nil
		},
		toolsy.WithReadOnly(),
	)
}

func encodeSearchJSON(docs []Document, o *options) (json.RawMessage, error) {
	return format.ApplyWithEnvelope(
		docs,
		func(d []Document) SearchDocumentsWire { return SearchDocumentsWire{Documents: d} },
		o.resultFormatter,
		o.hostResultValidator,
		o.maxBytes,
	)
}

func buildMarkdownIoCTool(r DocumentRetriever, o *options) (toolsy.Tool, error) {
	return toolsy.NewTool[searchArgs, format.JSONResult](
		o.name,
		o.description,
		func(ctx context.Context, _ *toolsy.RunEnv, args searchArgs) (format.JSONResult, error) {
			docs, err := retrieveAndFilter(ctx, r, o, args.Query)
			if err != nil {
				return format.JSONResult{}, err
			}
			raw, applyErr := format.ApplyWithEnvelope(
				docs,
				func(d []Document) SearchMarkdownWire {
					return SearchMarkdownWire{Results: FormatDocumentsMarkdown(d)}
				},
				o.resultFormatter,
				o.hostResultValidator,
				o.maxBytes,
			)
			if applyErr != nil {
				return format.JSONResult{}, applyErr
			}
			return format.JSONResult{Raw: raw}, nil
		},
		toolsy.WithReadOnly(),
	)
}

func buildMarkdownSearchTool(r DocumentRetriever, o *options) (toolsy.Tool, error) {
	return toolsy.NewTool[searchArgs, format.JSONResult](
		o.name,
		o.description,
		func(ctx context.Context, _ *toolsy.RunEnv, args searchArgs) (format.JSONResult, error) {
			docs, err := retrieveAndFilter(ctx, r, o, args.Query)
			if err != nil {
				return format.JSONResult{}, err
			}
			return format.ToJSONResult(SearchMarkdownWire{Results: FormatDocumentsMarkdown(docs)}, o.maxBytes)
		},
		toolsy.WithReadOnly(),
	)
}

func retrieveAndFilter(
	ctx context.Context,
	r DocumentRetriever,
	o *options,
	query string,
) ([]Document, error) {
	docs, err := r.Retrieve(ctx, query)
	if err != nil {
		return nil, toolsy.NewInternalError(fmt.Errorf("toolkit/rag: retrieve failed: %w", err))
	}
	if o.scopeFilter != nil {
		docs = o.scopeFilter(ctx, docs)
	}
	if o.maxResults > 0 && len(docs) > o.maxResults {
		docs = docs[:o.maxResults]
	}
	if wantsJSONTool(o) {
		docs, err = capDocumentsForWire(ctx, docs, o)
		if err != nil {
			return nil, err
		}
	}
	return docs, nil
}
