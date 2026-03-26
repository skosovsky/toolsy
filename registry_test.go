package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustBuildRegistry(t *testing.T, tools []Tool, opts ...RegistryOption) *Registry {
	t.Helper()
	reg, err := NewRegistryBuilder(opts...).Add(tools...).Build()
	require.NoError(t, err)
	return reg
}

func TestRegistryBuilder_BuildAndExecute(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("double", "Double x", func(_ context.Context, _ RunContext, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)

	reg := mustBuildRegistry(t, []Tool{tool})
	all := reg.GetAllTools()
	require.Len(t, all, 1)
	assert.Equal(t, "double", all[0].Manifest().Name)

	var out R
	err = reg.Execute(context.Background(), ToolCall{
		ID:       "1",
		ToolName: "double",
		Input:    ToolInput{ArgsJSON: []byte(`{"x": 7}`)},
	}, func(c Chunk) error {
		return json.Unmarshal(c.Data, &out)
	})
	require.NoError(t, err)
	assert.Equal(t, 14, out.Y)
}

func TestRegistryBuilder_DuplicateToolName(t *testing.T) {
	type A struct{}
	type R struct{}
	t1, err := NewTool("same", "First", func(_ context.Context, _ RunContext, _ A) (R, error) { return R{}, nil })
	require.NoError(t, err)
	t2, err := NewTool("same", "Second", func(_ context.Context, _ RunContext, _ A) (R, error) { return R{}, nil })
	require.NoError(t, err)

	_, err = NewRegistryBuilder().Add(t1, t2).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate tool name")
}

func TestRegistry_Execute_ToolNotFound(t *testing.T) {
	reg := mustBuildRegistry(t, nil)
	err := reg.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "missing", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrToolNotFound)
}

func TestRegistry_Execute_PanicRecovery_OnAfterSummary(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct{}
	tool, err := NewTool("panic", "Panics", func(_ context.Context, _ RunContext, _ A) (R, error) {
		panic("oops")
	})
	require.NoError(t, err)

	var lastSummary ExecutionSummary
	reg := mustBuildRegistry(
		t,
		[]Tool{tool},
		WithRecoverPanics(true),
		WithOnAfterExecute(func(_ context.Context, _ ToolCall, summary ExecutionSummary, _ time.Duration) {
			lastSummary = summary
		}),
	)

	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "panic", Input: ToolInput{ArgsJSON: []byte(`{"x": 1}`)}},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	var panicSE *SystemError
	require.ErrorAs(t, err, &panicSE)
	assert.Equal(t, "1", lastSummary.CallID)
	assert.Equal(t, "panic", lastSummary.ToolName)
	require.Error(t, lastSummary.Error)
}

func TestRegistry_Execute_Timeout(t *testing.T) {
	type A struct{}
	type R struct{}
	tool, err := NewTool("slow", "Slow", func(ctx context.Context, _ RunContext, _ A) (R, error) {
		<-ctx.Done()
		return R{}, ctx.Err()
	})
	require.NoError(t, err)

	reg := mustBuildRegistry(t, []Tool{tool}, WithDefaultTimeout(20*time.Millisecond))
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "slow", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrTimeout)
}

func TestRegistry_ExecuteIter(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	tool, err := NewStreamTool(
		"iter_stream",
		"Stream",
		func(_ context.Context, _ RunContext, a A, yield func(Chunk) error) error {
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

	reg := mustBuildRegistry(t, []Tool{tool})
	var seen int
	for chunk, iterErr := range reg.ExecuteIter(context.Background(), ToolCall{
		ID:       "iter1",
		ToolName: "iter_stream",
		Input:    ToolInput{ArgsJSON: []byte(`{"n": 5}`)},
	}) {
		require.NoError(t, iterErr)
		require.Equal(t, EventProgress, chunk.Event)
		seen++
		if seen == 3 {
			break
		}
	}
	assert.GreaterOrEqual(t, seen, 1)
}

func TestRegistry_ExecuteBatchStream_ChunkTagsAndErrors(t *testing.T) {
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
	reg := mustBuildRegistry(t, []Tool{tool})

	calls := []ToolCall{
		{ID: "1", ToolName: "double", Input: ToolInput{ArgsJSON: []byte(`{"x": 1}`)}},
		{ID: "2", ToolName: "missing", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		{ID: "3", ToolName: "double", Input: ToolInput{ArgsJSON: []byte(`{"x": 3}`)}},
	}
	var chunks []Chunk
	err = reg.ExecuteBatchStream(context.Background(), calls, func(c Chunk) error {
		chunks = append(chunks, c)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, chunks, 3)

	errCount := 0
	okCount := 0
	for _, c := range chunks {
		require.NotEmpty(t, c.CallID)
		require.NotEmpty(t, c.ToolName)
		if c.IsError {
			errCount++
			require.Equal(t, MimeTypeText, c.MimeType)
		} else {
			okCount++
			assertChunkJSONMime(t, c.MimeType)
		}
	}
	require.Equal(t, 1, errCount)
	require.Equal(t, 2, okCount)
}

func TestRegistry_ValidatorFailClosed(t *testing.T) {
	type A struct {
		Query string `json:"query"`
	}
	type R struct {
		OK bool `json:"ok"`
	}
	tool, err := NewTool("sql", "SQL", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{OK: true}, nil
	})
	require.NoError(t, err)

	reg := mustBuildRegistry(t, []Tool{tool}, WithValidator(&testValidator{
		validateFn: func(_ context.Context, _ string, argsJSON string) error {
			if argsJSON == `{"query":"drop table users"}` {
				return errors.New("blocked by policy")
			}
			return nil
		},
	}))

	err = reg.Execute(
		context.Background(),
		ToolCall{
			ID:       "v1",
			ToolName: "sql",
			Input:    ToolInput{ArgsJSON: []byte(`{"query":"drop table users"}`)},
		},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorIs(t, err, ErrValidation)
}

func TestRegistry_ShutdownRejectsNewCalls(t *testing.T) {
	reg := mustBuildRegistry(t, nil)
	require.NoError(t, reg.Shutdown(context.Background()))
	err := reg.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "noop", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrShutdown)
}

func TestRegistry_MaxConcurrencyTimeout(t *testing.T) {
	type A struct{}
	type R struct{}
	blocked := make(chan struct{})
	release := make(chan struct{})
	tool, err := NewTool("blocker", "Blocks", func(_ context.Context, _ RunContext, _ A) (R, error) {
		close(blocked)
		<-release
		return R{}, nil
	})
	require.NoError(t, err)

	reg := mustBuildRegistry(t, []Tool{tool}, WithMaxConcurrency(1))

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- reg.Execute(
			context.Background(),
			ToolCall{ID: "1", ToolName: "blocker", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
			func(Chunk) error { return nil },
		)
	}()
	<-blocked

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = reg.Execute(
		ctx,
		ToolCall{ID: "2", ToolName: "blocker", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrTimeout)

	close(release)
	require.NoError(t, <-firstDone)
}

func TestRegistry_OnChunkCountsOnlySuccess(t *testing.T) {
	type A struct{}

	var chunkCount atomic.Int32
	tool, err := NewStreamTool(
		"stream",
		"stream",
		func(_ context.Context, _ RunContext, _ A, yield func(Chunk) error) error {
			_ = yield(Chunk{Event: EventProgress, Data: []byte("chunk1"), MimeType: MimeTypeText})
			_ = yield(Chunk{Event: EventProgress, Data: []byte("chunk2"), MimeType: MimeTypeText})
			return yield(Chunk{Event: EventResult, Data: []byte(`{"ok":true}`), MimeType: MimeTypeJSON})
		},
	)
	require.NoError(t, err)

	reg := mustBuildRegistry(
		t,
		[]Tool{tool},
		WithOnChunk(func(_ context.Context, _ Chunk) { chunkCount.Add(1) }),
	)
	err = reg.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "stream", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	assert.Equal(t, int32(3), chunkCount.Load())
}

type testValidator struct {
	validateFn func(ctx context.Context, toolName, argsJSON string) error
}

func (v *testValidator) Validate(ctx context.Context, toolName, argsJSON string) error {
	if v.validateFn != nil {
		return v.validateFn(ctx, toolName, argsJSON)
	}
	return nil
}

func TestRegistry_ExecuteBatchStream_YieldIsSerialized(t *testing.T) {
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
	reg := mustBuildRegistry(t, []Tool{tool})

	const n = 20
	calls := make([]ToolCall, n)
	for i := range n {
		calls[i] = ToolCall{
			ID:       time.Now().Add(time.Duration(i) * time.Nanosecond).Format("150405.000000000"),
			ToolName: "inc",
			Input:    ToolInput{ArgsJSON: []byte(`{"x": 0}`)},
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
