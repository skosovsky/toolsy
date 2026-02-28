package toolsy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestToolCall_ToolResult(t *testing.T) {
	call := ToolCall{ID: "call_1", ToolName: "weather", Args: []byte(`{"location":"Moscow"}`)}
	assert.Equal(t, "call_1", call.ID)
	assert.Equal(t, "weather", call.ToolName)
	assert.JSONEq(t, `{"location":"Moscow"}`, string(call.Args))

	res := ToolResult{CallID: call.ID, ToolName: call.ToolName, Result: []byte(`{"temp":22.5}`), Error: nil}
	assert.Equal(t, "call_1", res.CallID)
	assert.Equal(t, "weather", res.ToolName)
	assert.NoError(t, res.Error)
}

// Ensure Tool interface is satisfied by a minimal impl (used in tests later).
type minTool struct {
	name, desc string
	params     map[string]any
	execute    func(context.Context, []byte) ([]byte, error)
}

func (m minTool) Name() string               { return m.name }
func (m minTool) Description() string        { return m.desc }
func (m minTool) Parameters() map[string]any { return m.params }
func (m minTool) Execute(ctx context.Context, args []byte) ([]byte, error) {
	if m.execute != nil {
		return m.execute(ctx, args)
	}
	return nil, nil
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
	tool, err := NewTool("weather", "Get temperature for a city", func(_ context.Context, _ Args) (Out, error) {
		return Out{Temp: 22.5}, nil
	})
	if err != nil {
		return
	}
	_ = tool.Name()
	_ = tool.Description()
	_ = tool.Parameters()
	// Output:
}

func ExampleRegistry_Execute() {
	type Args struct {
		X int `json:"x"`
	}
	type Out struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("add_one", "Add one", func(_ context.Context, a Args) (Out, error) {
		return Out{Y: a.X + 1}, nil
	})
	if err != nil {
		return
	}
	reg := NewRegistry()
	reg.Register(tool)
	result := reg.Execute(context.Background(), ToolCall{
		ID: "1", ToolName: "add_one", Args: []byte(`{"x": 5}`),
	})
	if result.Error != nil {
		panic(result.Error)
	}
	// result.Result is []byte(`{"y":6}`)
	_ = result
	// Output:
}

func ExampleRegistry_ExecuteBatch() {
	type Args struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type Out struct {
		Sum int `json:"sum"`
	}
	tool, err := NewTool("add", "Add two numbers", func(_ context.Context, a Args) (Out, error) {
		return Out{Sum: a.A + a.B}, nil
	})
	if err != nil {
		return
	}
	reg := NewRegistry()
	reg.Register(tool)
	calls := []ToolCall{
		{ID: "1", ToolName: "add", Args: []byte(`{"a": 1, "b": 2}`)},
		{ID: "2", ToolName: "add", Args: []byte(`{"a": 10, "b": 20}`)},
	}
	results := reg.ExecuteBatch(context.Background(), calls)
	// Partial success: each result is independent; failed calls have result.Error set
	_ = results
	// Output:
}
