package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStreamTool_MultipleChunks(t *testing.T) {
	type Args struct {
		N int `json:"n"`
	}
	tool, err := NewStreamTool(
		"stream",
		"Stream N chunks",
		func(_ context.Context, _ RunContext, a Args, yield func(Chunk) error) error {
			for i := range a.N {
				if err := yield(
					Chunk{Event: EventProgress, Data: []byte{byte('0' + i)}, MimeType: MimeTypeText},
				); err != nil {
					return err
				}
			}
			return nil
		},
	)
	require.NoError(t, err)

	var chunks [][]byte
	err = tool.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{"n": 3}`)},
		func(c Chunk) error {
			chunks = append(chunks, append([]byte(nil), c.Data...))
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, chunks, 3)
	assert.Equal(t, []byte("0"), chunks[0])
	assert.Equal(t, []byte("1"), chunks[1])
	assert.Equal(t, []byte("2"), chunks[2])
}

func TestNewStreamTool_YieldError(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	yieldErr := errors.New("client closed")
	tool, err := NewStreamTool(
		"abort",
		"Abort on yield",
		func(_ context.Context, _ RunContext, _ Args, yield func(Chunk) error) error {
			_ = yield(Chunk{Event: EventProgress, Data: []byte("first"), MimeType: MimeTypeText})
			return yield(Chunk{Event: EventResult, Data: []byte("second"), MimeType: MimeTypeText})
		},
	)
	require.NoError(t, err)

	var received [][]byte
	err = tool.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{"x": 1}`)},
		func(c Chunk) error {
			received = append(received, append([]byte(nil), c.Data...))
			if string(c.Data) == "first" {
				return nil
			}
			return yieldErr
		},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStreamAborted)
	require.Len(t, received, 2)
	assert.Equal(t, []byte("first"), received[0])
	assert.Equal(t, []byte("second"), received[1])
}

func TestNewStreamTool_ZeroChunks(t *testing.T) {
	type Args struct{}
	tool, err := NewStreamTool(
		"nop",
		"No chunks",
		func(_ context.Context, _ RunContext, _ Args, _ func(Chunk) error) error {
			return nil
		},
	)
	require.NoError(t, err)

	var count int
	err = tool.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error {
		count++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestNewTool_YieldCalledOnce(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("once", "Once", func(_ context.Context, _ RunContext, a Args) (R, error) {
		return R{Y: a.X + 1}, nil
	})
	require.NoError(t, err)

	var callCount int
	var out R
	err = tool.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{"x": 5}`)},
		func(c Chunk) error {
			callCount++
			return json.Unmarshal(c.Data, &out)
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
	assert.Equal(t, 6, out.Y)
}

func TestNewTool_YieldErrorReturnsErrStreamAborted(t *testing.T) {
	type Args struct{}
	type R struct{}
	tool, err := NewTool("yield_fail", "Yield fails", func(_ context.Context, _ RunContext, _ Args) (R, error) {
		return R{}, nil
	})
	require.NoError(t, err)

	yieldErr := errors.New("connection closed")
	err = tool.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error {
		return yieldErr
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStreamAborted)
}

func TestRegistry_ExecuteBatchStream_ChunkTags(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("double", "Double", func(_ context.Context, _ RunContext, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)

	calls := []ToolCall{
		{ToolName: "double", Input: ToolInput{CallID: "c1", ArgsJSON: []byte(`{"x": 1}`)}},
		{ToolName: "double", Input: ToolInput{CallID: "c2", ArgsJSON: []byte(`{"x": 2}`)}},
	}
	var chunks []Chunk
	err = reg.ExecuteBatchStream(context.Background(), calls, func(c Chunk) error {
		chunks = append(chunks, c)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, chunks, 2)

	byID := make(map[string]R)
	for _, c := range chunks {
		assert.Equal(t, "double", c.ToolName)
		var out R
		require.NoError(t, json.Unmarshal(c.Data, &out))
		byID[c.CallID] = out
	}
	require.Contains(t, byID, "c1")
	require.Contains(t, byID, "c2")
	assert.Equal(t, 2, byID["c1"].Y)
	assert.Equal(t, 4, byID["c2"].Y)
}

func TestRegistry_ExecuteBatchStream_SerializedYield(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("inc", "Inc", func(_ context.Context, _ RunContext, a A) (R, error) {
		return R{Y: a.X + 1}, nil
	})
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)

	const n = 20
	calls := make([]ToolCall, n)
	for i := range n {
		calls[i] = ToolCall{
			ToolName: "inc",
			Input:    ToolInput{CallID: fmt.Sprintf("id-%d", i), ArgsJSON: []byte(`{"x": 0}`)},
		}
	}
	var mu sync.Mutex
	var yieldCalls int
	err = reg.ExecuteBatchStream(context.Background(), calls, func(_ Chunk) error {
		mu.Lock()
		yieldCalls++
		mu.Unlock()
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, n, yieldCalls)
}

func TestRegistry_ExecuteBatchStream_YieldError(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("double", "Double", func(_ context.Context, _ RunContext, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)

	calls := []ToolCall{
		{ToolName: "double", Input: ToolInput{CallID: "c1", ArgsJSON: []byte(`{"x": 1}`)}},
		{ToolName: "double", Input: ToolInput{CallID: "c2", ArgsJSON: []byte(`{"x": 2}`)}},
	}
	yieldErr := errors.New("client disconnected")
	var chunks []Chunk
	err = reg.ExecuteBatchStream(context.Background(), calls, func(c Chunk) error {
		chunks = append(chunks, c)
		if len(chunks) >= 2 {
			return yieldErr
		}
		return nil
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(chunks), 1)
	for _, c := range chunks {
		assert.Equal(t, "double", c.ToolName)
		assert.NotEmpty(t, c.CallID)
		if !c.IsError {
			assert.NotEmpty(t, c.Data)
			assert.NotEmpty(t, c.MimeType)
		}
	}
}
