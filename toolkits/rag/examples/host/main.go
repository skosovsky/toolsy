// This runnable host recipe owns retrieval policy; it is not a rag routing API.
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/rag"
)

type fetch func(context.Context, string) ([]rag.Document, error)

type knowledgeBase struct {
	primary       fetch
	secondary     fetch
	supplemental  fetch
	allowFallback func([]rag.Document, error) bool
}

func newKnowledgeBase(
	primary, secondary, supplemental fetch,
	policy func([]rag.Document, error) bool,
) (*knowledgeBase, error) {
	if primary == nil || secondary == nil || supplemental == nil || policy == nil {
		return nil, errors.New("host knowledge base: required provider/policy is nil")
	}
	return &knowledgeBase{
		primary:       primary,
		secondary:     secondary,
		supplemental:  supplemental,
		allowFallback: policy,
	}, nil
}

func interrupted(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), context.Cause(ctx))
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func (k *knowledgeBase) retrievePrimary(ctx context.Context, query string) ([]rag.Document, error) {
	if err := interrupted(ctx, nil); err != nil {
		return nil, err
	}
	docs, primaryErr := k.primary(ctx, query)
	if err := interrupted(ctx, primaryErr); err != nil {
		return nil, err
	}
	// The host predicate consciously permits fallback; no blanket any-error rule.
	allow := k.allowFallback(docs, primaryErr)
	if err := interrupted(ctx, nil); err != nil {
		return nil, errors.Join(primaryErr, err)
	}
	if !allow {
		return docs, primaryErr
	}
	if err := interrupted(ctx, nil); err != nil {
		return nil, err
	}
	fallback, secondaryErr := k.secondary(ctx, query)
	if err := interrupted(ctx, secondaryErr); err != nil {
		return nil, errors.Join(primaryErr, err)
	}
	if secondaryErr != nil {
		return nil, errors.Join(primaryErr, fmt.Errorf("host secondary: %w", secondaryErr))
	}
	return fallback, nil
}

func (k *knowledgeBase) Retrieve(ctx context.Context, query string) ([]rag.Document, error) {
	docs, err := k.retrievePrimary(ctx, query)
	if err != nil {
		return nil, err
	}
	if err = interrupted(ctx, nil); err != nil {
		return nil, err
	}
	extra, err := k.supplemental(ctx, query)
	if stop := interrupted(ctx, err); stop != nil {
		return nil, stop
	}
	if err != nil {
		return nil, fmt.Errorf("host supplemental: %w", err)
	}
	// Aggregate and dedup are explicit host choices. Stable public chunk IDs here
	// are unique per source/version; unidentified units are always retained.
	out := append(append([]rag.Document(nil), docs...), extra...)
	return uniquePublicChunks(out), nil
}

type publicUnitKey struct{ source, id string }

func uniquePublicChunks(docs []rag.Document) []rag.Document {
	seen := make(map[publicUnitKey]struct{}, len(docs))
	out := make([]rag.Document, 0, len(docs))
	for _, doc := range docs {
		key := publicUnitKey{source: doc.SourceURI, id: doc.ID}
		if doc.ID != "" {
			if _, found := seen[key]; found {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, doc)
	}
	return out
}

const demoSourceURI = "doc://manual"

var errIndexUnavailable = errors.New("host index unavailable")

func main() {
	primary := fetch(func(context.Context, string) ([]rag.Document, error) { return nil, errIndexUnavailable })
	secondary := fetch(func(context.Context, string) ([]rag.Document, error) {
		return []rag.Document{{ID: "chunk-a", Content: "answer", SourceURI: demoSourceURI}}, nil
	})
	supplemental := fetch(func(context.Context, string) ([]rag.Document, error) {
		return []rag.Document{
			{ID: "chunk-a", Content: "answer", SourceURI: demoSourceURI},
			{ID: "chunk-b", Content: "detail", SourceURI: demoSourceURI},
		}, nil
	})
	policy := func(docs []rag.Document, err error) bool {
		return errors.Is(err, errIndexUnavailable) || (err == nil && len(docs) == 0)
	}
	retriever, err := newKnowledgeBase(primary, secondary, supplemental, policy)
	if err != nil {
		panic(err)
	}
	tool, err := rag.AsSearchTool(retriever, rag.WithResultShape(rag.ShapeDocumentsJSON))
	if err != nil {
		panic(err)
	}
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"manual"}`)},
		func(chunk toolsy.Chunk) error { fmt.Println(string(chunk.Data)); return nil },
	)
	if err != nil {
		panic(err)
	}
}
