package rag

import (
	"context"
	"encoding/json"

	"github.com/skosovsky/toolsy"
)

func capDocumentsForWire(ctx context.Context, docs []Document, o *options) ([]Document, error) {
	if o.maxBytes <= 0 || len(docs) == 0 {
		return docs, nil
	}
	out := cloneDocuments(docs)
	for len(out) > 1 && wireByteSize(out, o) > o.maxBytes {
		out = out[:len(out)-1]
	}
	if len(out) > 0 && wireByteSize(out, o) > o.maxBytes {
		return nil, toolsy.MapToolkitCapError(
			ctx,
			"toolkit/rag: wire cap",
			o.maxBytes,
			"search results",
			"reduce result count or raise wire budget",
		)
	}
	return out, nil
}

func wireByteSize(docs []Document, o *options) int {
	raw, err := json.Marshal(SearchDocumentsWire{Documents: docs})
	if err != nil {
		return o.maxBytes + 1
	}
	return len(raw)
}

func cloneDocuments(docs []Document) []Document {
	out := make([]Document, len(docs))
	copy(out, docs)
	return out
}
