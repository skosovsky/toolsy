package rag

import (
	"encoding/json"

	"github.com/skosovsky/toolsy/textprocessor"
)

func capDocumentsForWire(docs []Document, o *options) []Document {
	if o.maxBytes <= 0 || len(docs) == 0 {
		return docs
	}
	out := cloneDocuments(docs)
	for len(out) > 1 && wireByteSize(out, o) > o.maxBytes {
		out = out[:len(out)-1]
	}
	for len(out) > 0 && wireByteSize(out, o) > o.maxBytes {
		last := len(out) - 1
		content := out[last].Content
		if content == "" {
			out = out[:last]
			continue
		}
		nextLen := len(content) - 1
		if nextLen < 1 {
			out = out[:last]
			continue
		}
		out[last].Content = textprocessor.TruncateStringUTF8NoSuffix(content, nextLen)
	}
	return out
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
