package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"time"

	"github.com/skosovsky/toolsy"
)

const (
	callerTimeout  = 5 * time.Second
	collectedParts = 10
)

type counter struct{}

func (counter) ExecuteStream(ctx context.Context, input ProgressInput) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		n, err := input.Count.Int64()
		if err != nil {
			yield("", err)
			return
		}
		for i := int64(1); i <= n; i++ {
			if err := ctx.Err(); err != nil {
				yield("", err)
				return
			}
			if !yield(fmt.Sprintf("part %d", i), nil) {
				return
			}
		}
	}
}
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), callerTimeout)
	defer cancel()
	base, err := NewProgressTool(counter{})
	if err != nil {
		panic(err)
	}
	done := make(chan error, 1)
	async := toolsy.AsAsyncTool(
		base,
		toolsy.WithBackgroundTimeout(time.Second),
		toolsy.WithMaxCollectedChunks(collectedParts),
		toolsy.WithOnComplete(func(_ context.Context, taskID string, chunks []toolsy.Chunk, err error) {
			for _, chunk := range chunks {
				fmt.Printf("%s: %s %s\n", taskID, chunk.Event, chunk.Data)
			}
			done <- err
		}),
	)
	registry, err := toolsy.NewRegistryBuilder().Add(async).Build()
	if err != nil {
		panic(err)
	}
	var accepted toolsy.AsyncAccepted
	err = registry.Execute(
		ctx,
		toolsy.ToolCall{ToolName: "progress", Input: toolsy.ToolInput{ArgsJSON: []byte(`{"count":3}`)}},
		func(chunk toolsy.Chunk) error { return json.Unmarshal(chunk.Data, &accepted) },
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("scheduled %s\n", accepted.TaskID)
	if err := <-done; err != nil {
		panic(err)
	}
	if err := registry.Shutdown(ctx); err != nil {
		panic(err)
	}
}
