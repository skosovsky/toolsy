package toolsy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
)

func ExampleRegistryBuilder_Add() {
	type Args struct {
		X int `json:"x"`
	}
	type Out struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("with_opts", "Tool with options", func(_ context.Context, _ RunContext, a Args) (Out, error) {
		return Out{Y: a.X * 2}, nil
	}, WithStrict())
	if err != nil {
		return
	}
	reg, err := NewRegistryBuilder().Add(tool).Build()
	if err != nil {
		return
	}
	_, ok := reg.GetTool("with_opts")
	if ok {
		fmt.Println("registered")
	}
	// Output:
	// registered
}

func ExampleExtractor_ParseAndValidate() {
	type WeatherInput struct {
		City string `json:"city" jsonschema:"City name"`
	}
	ext, err := NewExtractor[WeatherInput](false)
	if err != nil {
		return
	}
	args, err := ext.ParseAndValidate([]byte(`{"city":"London"}`))
	if err != nil {
		return
	}
	fmt.Println(args.City)
	// Output:
	// London
}

func ExampleExtractor_Schema() {
	type Params struct {
		Q string `json:"q" jsonschema:"Query"`
	}
	ext, err := NewExtractor[Params](false)
	if err != nil {
		return
	}
	schema := ext.Schema()
	fmt.Println(schema["type"])
	// Output:
	// object
}

func ExampleRegistryBuilder_Use() {
	type Args struct {
		N int `json:"n"`
	}
	type Out struct {
		Double int `json:"double"`
	}
	tool, err := NewTool("double", "Double the number", func(_ context.Context, _ RunContext, a Args) (Out, error) {
		return Out{Double: a.N * 2}, nil
	})
	if err != nil {
		return
	}
	reg, err := NewRegistryBuilder().Use(
		WithLogging(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))),
	).Add(tool).Build()
	if err != nil {
		return
	}

	var out Out
	_ = reg.Execute(context.Background(), ToolCall{
		ToolName: "double",
		Input:    ToolInput{CallID: "1", ArgsJSON: []byte(`{"n": 21}`)},
	}, func(c Chunk) error {
		return json.Unmarshal(c.Data, &out)
	})
	b, _ := json.Marshal(out)
	fmt.Printf("result: %s", b)
	// Output:
	// result: {"double":42}
}
