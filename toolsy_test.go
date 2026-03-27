package toolsy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// assertChunkJSONMime checks Chunk MIME for JSON payloads. Do not use JSONEq:
// "application/json" is a media-type string, not a JSON document.
func assertChunkJSONMime(tb testing.TB, got string) {
	tb.Helper()
	//nolint:testifylint // MimeTypeJSON is a media-type token, not JSON; JSONEq would parse it as JSON and fail.
	assert.Equal(tb, MimeTypeJSON, got)
}

func TestToolCall_Chunk(t *testing.T) {
	call := ToolCall{
		ToolName: "weather",
		Input: ToolInput{
			CallID:   "call_1",
			ArgsJSON: []byte(`{"location":"Moscow"}`),
		},
	}
	assert.Equal(t, "call_1", call.Input.CallID)
	assert.Equal(t, "weather", call.ToolName)
	assert.JSONEq(t, `{"location":"Moscow"}`, string(call.Input.ArgsJSON))

	chunk := Chunk{
		CallID:   call.Input.CallID,
		ToolName: call.ToolName,
		Event:    EventResult,
		Data:     []byte(`{"temp":22.5}`),
		MimeType: MimeTypeJSON,
	}
	assert.Equal(t, "call_1", chunk.CallID)
	assert.Equal(t, "weather", chunk.ToolName)
	assert.Equal(t, EventResult, chunk.Event)
	assert.Equal(t, []byte(`{"temp":22.5}`), chunk.Data)
	assertChunkJSONMime(t, chunk.MimeType)
}

func TestChunk_EventIsErrorMetadata(t *testing.T) {
	assert.Equal(t, EventProgress, EventType("progress"))
	assert.Equal(t, EventResult, EventType("result"))

	c := Chunk{
		Event:    EventResult,
		IsError:  false,
		Metadata: map[string]any{"percent": 50},
	}
	assert.Equal(t, EventResult, c.Event)
	assert.False(t, c.IsError)
	assert.Equal(t, 50, c.Metadata["percent"])

	cErr := Chunk{Event: EventResult, Data: []byte("fail"), MimeType: MimeTypeText, IsError: true}
	assert.Equal(t, EventResult, cErr.Event)
	assert.True(t, cErr.IsError)
	assert.Equal(t, []byte("fail"), cErr.Data)
	assert.Equal(t, MimeTypeText, cErr.MimeType)
}

type minTool struct {
	manifest ToolManifest
	execute  func(context.Context, RunContext, ToolInput, func(Chunk) error) error
}

func (m minTool) Manifest() ToolManifest { return m.manifest }

func (m minTool) Execute(ctx context.Context, run RunContext, input ToolInput, yield func(Chunk) error) error {
	if m.execute != nil {
		return m.execute(ctx, run, input, yield)
	}
	return nil
}

func TestMinTool_ImplementsTool(_ *testing.T) {
	var _ Tool = minTool{}
}

func ExampleNewTool() {
	type Args struct {
		City string `json:"city" jsonschema:"City name"`
	}
	type Out struct {
		Temp float64 `json:"temp"`
	}
	tool, err := NewTool(
		"weather",
		"Get temperature for a city",
		func(_ context.Context, _ RunContext, _ Args) (Out, error) {
			return Out{Temp: 22.5}, nil
		},
	)
	if err != nil {
		return
	}
	m := tool.Manifest()
	_ = m.Name
	_ = m.Description
	_ = m.Parameters
	// Output:
}

func ExampleRegistry_Execute() {
	type Args struct {
		X int `json:"x"`
	}
	type Out struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("add_one", "Add one", func(_ context.Context, _ RunContext, a Args) (Out, error) {
		return Out{Y: a.X + 1}, nil
	})
	if err != nil {
		return
	}
	reg, err := NewRegistryBuilder().Add(tool).Build()
	if err != nil {
		return
	}

	var out Out
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "add_one",
		Input:    ToolInput{CallID: "1", ArgsJSON: []byte(`{"x": 5}`)},
	}, func(c Chunk) error {
		return json.Unmarshal(c.Data, &out)
	})
	if err != nil {
		panic(err)
	}
	_ = out
	// Output:
}

func ExampleRegistry_ExecuteBatchStream() {
	type Args struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type Out struct {
		Sum int `json:"sum"`
	}
	tool, err := NewTool("add", "Add two numbers", func(_ context.Context, _ RunContext, a Args) (Out, error) {
		return Out{Sum: a.A + a.B}, nil
	})
	if err != nil {
		return
	}
	reg, err := NewRegistryBuilder().Add(tool).Build()
	if err != nil {
		return
	}
	calls := []ToolCall{
		{ToolName: "add", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{"a": 1, "b": 2}`)}},
		{ToolName: "add", Input: ToolInput{CallID: "2", ArgsJSON: []byte(`{"a": 10, "b": 20}`)}},
	}
	err = reg.ExecuteBatchStream(context.Background(), calls, func(_ Chunk) error { return nil })
	if err != nil {
		panic(err)
	}
	// Output:
}
