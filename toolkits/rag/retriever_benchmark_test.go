package rag_test

import (
	"context"
	"strings"
	"testing"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/rag"
)

type benchmarkRetriever struct{ docs []rag.Document }

func (r benchmarkRetriever) Retrieve(context.Context, string) ([]rag.Document, error) {
	return r.docs, nil
}

func BenchmarkSearchDocuments(b *testing.B) {
	docs := make([]rag.Document, 5)
	for i := range docs {
		docs[i] = rag.Document{
			ID:        "chunk",
			Content:   strings.Repeat("<界", 32),
			SourceURI: "doc://manual",
			Metadata:  map[string]string{"category": "public"},
		}
	}
	tool, err := rag.AsSearchTool(benchmarkRetriever{docs: docs}, rag.WithResultShape(rag.ShapeDocumentsJSON))
	if err != nil {
		b.Fatal(err)
	}
	run := toolsy.NewRunEnv(nil)
	input := toolsy.ToolInput{ArgsJSON: []byte(`{"query":"manual"}`)}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err = tool.Execute(b.Context(), run, input, func(toolsy.Chunk) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}
