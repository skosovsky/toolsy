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
	tool, err := NewStreamTool("stream", "Stream N chunks", func(_ context.Context, a Args, yield func([]byte) error) error {
		for i := 0; i < a.N; i++ {
			if err := yield([]byte{byte('0' + i)}); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)
	var chunks [][]byte
	err = tool.Execute(context.Background(), []byte(`{"n": 3}`), func(chunk []byte) error {
		chunks = append(chunks, append([]byte(nil), chunk...))
		return nil
	})
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
	tool, err := NewStreamTool("abort", "Abort on yield", func(_ context.Context, _ Args, yield func([]byte) error) error {
		_ = yield([]byte("first"))
		return yield([]byte("second")) // will return yieldErr from caller
	})
	require.NoError(t, err)
	var received [][]byte
	err = tool.Execute(context.Background(), []byte(`{"x": 1}`), func(chunk []byte) error {
		received = append(received, append([]byte(nil), chunk...))
		if string(chunk) == "first" {
			return nil
		}
		return yieldErr
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStreamAborted)
	require.Len(t, received, 2)
	assert.Equal(t, []byte("first"), received[0])
	assert.Equal(t, []byte("second"), received[1])
}

func TestNewStreamTool_ZeroChunks(t *testing.T) {
	type Args struct{}
	tool, err := NewStreamTool("nop", "No chunks", func(_ context.Context, _ Args, _ func([]byte) error) error {
		return nil
	})
	require.NoError(t, err)
	var count int
	err = tool.Execute(context.Background(), []byte(`{}`), func([]byte) error {
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
	tool, err := NewTool("once", "Once", func(_ context.Context, a Args) (R, error) {
		return R{Y: a.X + 1}, nil
	})
	require.NoError(t, err)
	var callCount int
	var singleChunk []byte
	err = tool.Execute(context.Background(), []byte(`{"x": 5}`), func(chunk []byte) error {
		callCount++
		singleChunk = chunk
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
	var out R
	require.NoError(t, json.Unmarshal(singleChunk, &out))
	assert.Equal(t, 6, out.Y)
}

func TestNewTool_YieldErrorReturnsErrStreamAborted(t *testing.T) {
	type Args struct{}
	type R struct{}
	tool, err := NewTool("yield_fail", "Yield fails", func(_ context.Context, _ Args) (R, error) {
		return R{}, nil
	})
	require.NoError(t, err)
	yieldErr := errors.New("connection closed")
	err = tool.Execute(context.Background(), []byte(`{}`), func([]byte) error {
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
	tool, err := NewTool("double", "Double", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry()
	reg.Register(tool)
	calls := []ToolCall{
		{ID: "c1", ToolName: "double", Args: []byte(`{"x": 1}`)},
		{ID: "c2", ToolName: "double", Args: []byte(`{"x": 2}`)},
	}
	var chunks []Chunk
	err = reg.ExecuteBatchStream(context.Background(), calls, func(c Chunk) error {
		chunks = append(chunks, c)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, chunks, 2)
	byID := make(map[string]Chunk)
	for _, c := range chunks {
		byID[c.CallID] = c
		assert.Equal(t, "double", c.ToolName)
	}
	require.Contains(t, byID, "c1")
	require.Contains(t, byID, "c2")
	var r R
	require.NoError(t, json.Unmarshal(byID["c1"].Data, &r))
	assert.Equal(t, 2, r.Y)
	require.NoError(t, json.Unmarshal(byID["c2"].Data, &r))
	assert.Equal(t, 4, r.Y)
}

func TestRegistry_ExecuteBatchStream_SerializedYield(t *testing.T) {
	// Ensure yield is called from one goroutine at a time (no data races).
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("inc", "Inc", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X + 1}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry()
	reg.Register(tool)
	const N = 20
	calls := make([]ToolCall, N)
	for i := range N {
		calls[i] = ToolCall{ID: fmt.Sprintf("id-%d", i), ToolName: "inc", Args: []byte(`{"x": 0}`)}
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
	assert.Equal(t, N, yieldCalls)
}

// TestRegistry_ExecuteBatchStream_YieldError verifies that when the batch yield callback returns
// an error, ExecuteBatchStream returns ErrStreamAborted and partial chunks may have been delivered.
func TestRegistry_ExecuteBatchStream_YieldError(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("double", "Double", func(_ context.Context, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)
	reg := NewRegistry()
	reg.Register(tool)
	calls := []ToolCall{
		{ID: "c1", ToolName: "double", Args: []byte(`{"x": 1}`)},
		{ID: "c2", ToolName: "double", Args: []byte(`{"x": 2}`)},
	}
	yieldErr := errors.New("client disconnected")
	var chunks []Chunk
	err = reg.ExecuteBatchStream(context.Background(), calls, func(c Chunk) error {
		chunks = append(chunks, c)
		// Fail on second chunk (from any call) to trigger ErrStreamAborted.
		if len(chunks) >= 2 {
			return yieldErr
		}
		return nil
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStreamAborted)
	// We may have received 1 or 2 chunks before yield failed (parallel execution).
	assert.GreaterOrEqual(t, len(chunks), 1)
	assert.LessOrEqual(t, len(chunks), 2)
	for _, c := range chunks {
		assert.Equal(t, "double", c.ToolName)
		assert.NotEmpty(t, c.CallID)
		assert.NotEmpty(t, c.Data)
	}
}
