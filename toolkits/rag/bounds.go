package rag

import (
	"encoding/json"

	"github.com/skosovsky/toolsy"
)

func validateDocuments(docs []Document, o *options) error {
	total := 2
	for i, doc := range docs {
		if len(doc.Content) > o.maxItemBytes || len(doc.SourceURI) > o.maxItemBytes || len(doc.ID) > o.maxItemBytes ||
			len(doc.Category) > o.maxItemBytes ||
			len(doc.Metadata) > o.maxItemBytes {
			return toolsy.NewValidationError("retrieval unit exceeds item byte limit")
		}
		metadataBytes := 0
		for k, v := range doc.Metadata {
			if len(k) > o.maxItemBytes-metadataBytes {
				return toolsy.NewValidationError("retrieval metadata exceeds item byte limit")
			}
			metadataBytes += len(k)
			if len(v) > o.maxItemBytes-metadataBytes {
				return toolsy.NewValidationError("retrieval metadata exceeds item byte limit")
			}
			metadataBytes += len(v)
		}
		raw, err := json.Marshal(doc)
		if err != nil {
			return toolsy.NewInternalError(err)
		}
		if len(raw) > o.maxItemBytes {
			return toolsy.NewValidationError("retrieval unit exceeds item byte limit")
		}
		separator := 0
		if i > 0 {
			separator = 1
		}
		if len(raw)+separator > o.maxSourceBytes-total {
			return toolsy.NewValidationError("retrieval collection exceeds source byte limit")
		}
		total += len(raw) + separator
	}
	return nil
}
