package toolsy

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestExecutionCount(t *testing.T) {
    ctx := context.Background()
    assert.Equal(t, int64(0), ExecutionCount(ctx))

    ctx = WithExecutionCounter(ctx)
    assert.Equal(t, int64(0), ExecutionCount(ctx))

    type A struct{}
    type R struct{}
    tool, err := NewTool("noop_count", "Noop", func(_ context.Context, _ A) (R, error) {
        return R{}, nil
    })
    require.NoError(t, err)
    reg := NewRegistry(WithMaxSteps(0))
    reg.Register(tool)

    for i := 0; i < 2; i++ {
        err = reg.Execute(ctx, ToolCall{ID: "c", ToolName: "noop_count", Args: raw(`{}`)}, func(Chunk) error { return nil })
        require.NoError(t, err)
    }
    assert.Equal(t, int64(2), ExecutionCount(ctx))
}
