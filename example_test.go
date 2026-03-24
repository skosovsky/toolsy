package toolsy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"
)

// ExampleRegistry_Register shows building a tool with options and registering it.
func ExampleRegistry_Register() {
	type Args struct {
		X int `json:"x"`
	}
	type Out struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("with_opts", "Tool with options", func(_ context.Context, a Args) (Out, error) {
		return Out{Y: a.X * 2}, nil
	}, WithTimeout(2*time.Second), WithStrict())
	if err != nil {
		return
	}
	reg := NewRegistry(WithDefaultTimeout(5 * time.Second))
	reg.Register(tool)
	_, ok := reg.GetTool("with_opts")
	if ok {
		fmt.Println("registered")
	}
	// Output:
	// registered
}

// ExampleExtractor_ParseAndValidate shows schema + validation without Execute; parses JSON into a typed struct.
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

// ExampleExtractor_Schema shows that Extractor produces a JSON Schema (e.g. type "object") for the struct.
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

// ExampleRegistry_Use shows a chain of two middlewares (logging + timeout) applied via Use.
func ExampleRegistry_Use() {
	type Args struct {
		N int `json:"n"`
	}
	type Out struct {
		Double int `json:"double"`
	}
	tool, err := NewTool("double", "Double the number", func(_ context.Context, a Args) (Out, error) {
		return Out{Double: a.N * 2}, nil
	})
	if err != nil {
		return
	}
	reg := NewRegistry(WithDefaultTimeout(5 * time.Second))
	reg.Use(
		WithLogging(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))),
		WithTimeoutMiddleware(2*time.Second),
	)
	reg.Register(tool)
	var out Out
	_ = reg.Execute(context.Background(), ToolCall{
		ID: "1", ToolName: "double", Args: []byte(`{"n": 21}`),
	}, func(c Chunk) error {
		out = c.RawData.(Out)
		return nil
	})
	b, _ := json.Marshal(out)
	fmt.Printf("result: %s", b)
	// Output:
	// result: {"double":42}
}
