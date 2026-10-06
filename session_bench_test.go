package toolsy

import (
	"context"
	"strconv"
	"testing"
)

func BenchmarkSessionConcurrentExecute(b *testing.B) {
	tool, err := NewTool[struct{}, string](
		"value",
		"Return a value",
		func(context.Context, *RunEnv, struct{}) (string, error) { return "value", nil },
	)
	if err != nil {
		b.Fatal(err)
	}
	registry, err := NewRegistry(tool)
	if err != nil {
		b.Fatal(err)
	}
	session, err := NewSession(registry)
	if err != nil {
		b.Fatal(err)
	}
	call := ToolCall{ToolName: "value", Input: ToolInput{ArgsJSON: []byte(`{}`)}}
	yield := func(Chunk) error { return nil }
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(iterator *testing.PB) {
		for iterator.Next() {
			if err := session.Execute(b.Context(), call, yield); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

func BenchmarkSessionExportSnapshot(b *testing.B) {
	session, err := NewSession(nil)
	if err != nil {
		b.Fatal(err)
	}
	for index := range 32 {
		SetSessionState(session, strconv.Itoa(index), index)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err = session.ExportSnapshot(); err != nil {
			b.Fatal(err)
		}
	}
}
