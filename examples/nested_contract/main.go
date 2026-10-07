package main

import (
	"context"
	"fmt"

	"github.com/skosovsky/toolsy"
)

type request struct {
	Payload struct {
		Tags []string `json:"tags"`
	} `json:"payload"`
}
type response struct {
	Count int `json:"count"`
}

func buildTool(handler func(context.Context, *toolsy.RunEnv, request) (response, error)) (toolsy.Tool, error) {
	return toolsy.NewTool[request, response](
		"nested_tags",
		"Count tags in a nested typed object",
		handler,
		toolsy.WithReadOnly(),
	)
}

func main() {
	tool, err := buildTool(func(_ context.Context, _ *toolsy.RunEnv, in request) (response, error) {
		return response{Count: len(in.Payload.Tags)}, nil
	})
	if err != nil {
		panic(err)
	}
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"payload":{"tags":["one","two"]}}`)},
		func(chunk toolsy.Chunk) error { fmt.Println(string(chunk.Data)); return nil },
	)
	if err != nil {
		panic(err)
	}
}
