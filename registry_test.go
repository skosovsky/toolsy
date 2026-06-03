package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustBuildRegistry(t *testing.T, tools []Tool, opts ...RegistryOption) *Registry {
	t.Helper()
	reg, err := NewRegistryBuilder(opts...).Add(tools...).Build()
	require.NoError(t, err)
	return reg
}

type invalidUTF8Error struct{}

func (invalidUTF8Error) Error() string { return string([]byte{0xff, 0xfe, 'x'}) }

func TestRegistryBuilder_BuildAndExecute(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("double", "Double x", func(_ context.Context, _ *RunEnv, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)

	reg := mustBuildRegistry(t, []Tool{tool})
	all := reg.GetAllTools()
	require.Len(t, all, 1)
	assert.Equal(t, "double", all[0].Manifest().Name)

	var out R
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "double",
		Input:    ToolInput{CallID: "1", ArgsJSON: []byte(`{"x": 7}`)},
	}, func(c Chunk) error {
		return json.Unmarshal(c.Data, &out)
	})
	require.NoError(t, err)
	assert.Equal(t, 14, out.Y)
}

func TestRegistryBuilder_DuplicateToolName(t *testing.T) {
	type A struct{}
	type R struct{}
	t1, err := NewTool("same", "First", func(_ context.Context, _ *RunEnv, _ A) (R, error) { return R{}, nil })
	require.NoError(t, err)
	t2, err := NewTool("same", "Second", func(_ context.Context, _ *RunEnv, _ A) (R, error) { return R{}, nil })
	require.NoError(t, err)

	_, err = NewRegistryBuilder().Add(t1, t2).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate tool name")
}

func mustNamedTool(t *testing.T, name string) Tool {
	t.Helper()
	type A struct{}
	type R struct{}
	tool, err := NewTool(name, name, func(_ context.Context, _ *RunEnv, _ A) (R, error) {
		return R{}, nil
	})
	require.NoError(t, err)
	return tool
}

func TestRegistry_Has(t *testing.T) {
	reg := mustBuildRegistry(t, []Tool{mustNamedTool(t, "x")})
	assert.True(t, reg.Has("x"))
	assert.False(t, reg.Has("y"))
}

func TestRegistry_ToolNames(t *testing.T) {
	reg := mustBuildRegistry(t, []Tool{
		mustNamedTool(t, "b"),
		mustNamedTool(t, "a"),
		mustNamedTool(t, "c"),
	})
	assert.Equal(t, []string{"a", "b", "c"}, reg.ToolNames())
}

func TestRegistry_Subset_ValidTools(t *testing.T) {
	reg := mustBuildRegistry(t, []Tool{
		mustNamedTool(t, "a"),
		mustNamedTool(t, "b"),
		mustNamedTool(t, "c"),
	})
	sub, err := reg.Subset("b", "a")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, sub.ToolNames())
	assert.Len(t, reg.ToolNames(), 3)
}

func TestRegistry_Subset_SilentDedup(t *testing.T) {
	reg := mustBuildRegistry(t, []Tool{
		mustNamedTool(t, "a"),
		mustNamedTool(t, "b"),
	})
	sub, err := reg.Subset("a", "b", "a")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, sub.ToolNames())
}

func TestRegistry_Subset_UnknownTool(t *testing.T) {
	reg := mustBuildRegistry(t, []Tool{mustNamedTool(t, "a")})
	_, err := reg.Subset("a", "missing")
	require.Error(t, err)
	requireToolErrorCode(t, err, CodeToolNotFound, ErrToolNotFound)
	assert.Contains(t, err.Error(), "unknown tool in subset")
}

func TestRegistry_NilReceiverMapHelpers(t *testing.T) {
	t.Parallel()

	var reg *Registry

	assert.False(t, reg.Has("any"))
	tool, ok := reg.GetTool("any")
	assert.Nil(t, tool)
	assert.False(t, ok)
	assert.Nil(t, reg.ToolNames())
	assert.Nil(t, reg.GetAllTools())
}

func TestRegistry_Subset_ExecuteDeniedForNonMember(t *testing.T) {
	reg := mustBuildRegistry(t, []Tool{
		mustNamedTool(t, "allowed"),
		mustNamedTool(t, "denied"),
	})
	sub, err := reg.Subset("allowed")
	require.NoError(t, err)
	err = sub.Execute(
		context.Background(),
		ToolCall{ToolName: "denied", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	requireToolErrorCode(t, err, CodeToolNotFound, ErrToolNotFound)
}

func TestRegistry_Subset_ParentShutdownBlocksSubsetExecute(t *testing.T) {
	reg := mustBuildRegistry(t, []Tool{mustNamedTool(t, "x")})
	sub, err := reg.Subset("x")
	require.NoError(t, err)
	require.NoError(t, reg.Shutdown(context.Background()))
	err = sub.Execute(
		context.Background(),
		ToolCall{ToolName: "x", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	requireToolErrorCode(t, err, CodeShutdown, ErrShutdown)
}

func TestRegistry_Subset_ShutdownBlocksParentExecute(t *testing.T) {
	reg := mustBuildRegistry(t, []Tool{mustNamedTool(t, "x")})
	sub, err := reg.Subset("x")
	require.NoError(t, err)
	require.NoError(t, sub.Shutdown(context.Background()))
	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "x", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	requireToolErrorCode(t, err, CodeShutdown, ErrShutdown)
}

func TestRegistry_Subset_InFlightWaitsOnParentShutdown(t *testing.T) {
	toolStarted := make(chan struct{})
	shutdownStarted := make(chan struct{})
	releaseTool := make(chan struct{})

	type A struct{}
	type R struct{}
	tool, err := NewTool("slow", "slow", func(_ context.Context, _ *RunEnv, _ A) (R, error) {
		close(toolStarted)
		<-releaseTool
		return R{}, nil
	})
	require.NoError(t, err)

	reg := mustBuildRegistry(t, []Tool{tool})
	sub, err := reg.Subset("slow")
	require.NoError(t, err)

	execDone := make(chan error, 1)
	go func() {
		execDone <- sub.Execute(
			context.Background(),
			ToolCall{ToolName: "slow", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
			func(Chunk) error { return nil },
		)
	}()

	<-toolStarted
	shutdownDone := make(chan error, 1)
	go func() {
		close(shutdownStarted)
		shutdownDone <- reg.Shutdown(context.Background())
	}()

	<-shutdownStarted
	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned before in-flight tool finished: %v", err)
	case <-releaseTool:
		t.Fatal("release channel must not be closed before tool is released")
	default:
	}
	close(releaseTool)
	require.NoError(t, <-shutdownDone)
	require.NoError(t, <-execDone)
}

func TestRegistry_Subset_InFlightWaitsOnSubsetShutdown(t *testing.T) {
	toolStarted := make(chan struct{})
	shutdownStarted := make(chan struct{})
	releaseTool := make(chan struct{})

	type A struct{}
	type R struct{}
	tool, err := NewTool("slow", "slow", func(_ context.Context, _ *RunEnv, _ A) (R, error) {
		close(toolStarted)
		<-releaseTool
		return R{}, nil
	})
	require.NoError(t, err)

	reg := mustBuildRegistry(t, []Tool{tool})
	sub, err := reg.Subset("slow")
	require.NoError(t, err)

	execDone := make(chan error, 1)
	go func() {
		execDone <- reg.Execute(
			context.Background(),
			ToolCall{ToolName: "slow", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
			func(Chunk) error { return nil },
		)
	}()

	<-toolStarted
	shutdownDone := make(chan error, 1)
	go func() {
		close(shutdownStarted)
		shutdownDone <- sub.Shutdown(context.Background())
	}()

	<-shutdownStarted
	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned before in-flight tool finished: %v", err)
	case <-releaseTool:
		t.Fatal("release channel must not be closed before tool is released")
	default:
	}
	close(releaseTool)
	require.NoError(t, <-shutdownDone)
	require.NoError(t, <-execDone)
}

func TestRegistry_InvalidRuntimeState(t *testing.T) {
	var reg Registry
	reg.tools = map[string]Tool{"x": mustNamedTool(t, "x")}

	_, err := reg.Subset("x")
	requireToolErrorCode(t, err, CodeRegistryNotReady, ErrRegistryState)

	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "x", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	requireToolErrorCode(t, err, CodeRegistryNotReady, ErrRegistryState)

	err = reg.Shutdown(context.Background())
	requireToolErrorCode(t, err, CodeRegistryNotReady, ErrRegistryState)

	err = ValidateContract(&reg, []string{"x"})
	requireToolErrorCode(t, err, CodeRegistryNotReady, ErrRegistryState)
}

func TestRegistry_Execute_ToolNotFound(t *testing.T) {
	reg := mustBuildRegistry(t, nil)
	err := reg.Execute(
		context.Background(),
		ToolCall{ToolName: "missing", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	requireToolErrorCode(t, err, CodeToolNotFound, ErrToolNotFound)
}

func TestRegistry_Execute_PanicRecovery_OnAfterSummary(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct{}
	tool, err := NewTool("panic", "Panics", func(_ context.Context, _ *RunEnv, _ A) (R, error) {
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
		ToolCall{ToolName: "panic", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{"x": 1}`)}},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	var panicTE *ToolError
	require.ErrorAs(t, err, &panicTE)
	require.Equal(t, CodeInternal, panicTE.Code)
	assert.Equal(t, "1", lastSummary.CallID)
	assert.Equal(t, "panic", lastSummary.ToolName)
	require.Error(t, lastSummary.Error)
}

func TestRegistry_Execute_OnAfterSummaryTracksSoftErrorChunk(t *testing.T) {
	tool := newMiddlewareMinTool(
		"soft_summary",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{
				Event:    EventResult,
				Data:     []byte("budget exceeded"),
				MimeType: MimeTypeText,
				IsError:  true,
			})
		},
	)

	var lastSummary ExecutionSummary
	reg := mustBuildRegistry(
		t,
		[]Tool{tool},
		WithOnAfterExecute(func(_ context.Context, _ ToolCall, summary ExecutionSummary, _ time.Duration) {
			lastSummary = summary
		}),
	)

	err := reg.Execute(
		context.Background(),
		ToolCall{ToolName: "soft_summary", Input: ToolInput{CallID: "s1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	assert.NoError(t, lastSummary.Error)
	assert.Equal(t, 1, lastSummary.ErrorChunks)
	assert.Equal(t, "budget exceeded", lastSummary.LastErrorText)
	assert.Equal(t, 0, lastSummary.ChunksDelivered)
	assert.Equal(t, int64(0), lastSummary.TotalBytes)
}

func TestRegistry_ExecuteBatchStream_OnAfterSummaryTracksSoftenedErrorChunk(t *testing.T) {
	tool := newMiddlewareMinTool(
		"batch_soft_summary",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return errors.New("batch tool failed")
		},
	)

	var (
		lastSummary ExecutionSummary
		afterCalls  atomic.Int32
	)
	reg := mustBuildRegistry(
		t,
		[]Tool{tool},
		WithOnAfterExecute(func(_ context.Context, _ ToolCall, summary ExecutionSummary, _ time.Duration) {
			lastSummary = summary
			afterCalls.Add(1)
		}),
	)

	var chunks []Chunk
	err := reg.ExecuteBatchStream(
		context.Background(),
		[]ToolCall{{
			ToolName: "batch_soft_summary",
			Input:    ToolInput{CallID: "bs1", ArgsJSON: []byte(`{}`)},
		}},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsError)
	assert.Equal(t, int32(1), afterCalls.Load())
	require.NoError(t, lastSummary.Error)
	assert.Equal(t, 1, lastSummary.ErrorChunks)
	assert.Equal(t, "batch tool failed", lastSummary.LastErrorText)
	assert.Equal(t, 0, lastSummary.ChunksDelivered)
	assert.Equal(t, int64(0), lastSummary.TotalBytes)
}

func TestRegistry_ExecuteBatchStream_OnAfterSummaryTracksValidatorSoftenedError(t *testing.T) {
	tool := newMiddlewareMinTool(
		"validator_soft_summary",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return nil
		},
	)

	var (
		lastSummary ExecutionSummary
		afterCalls  atomic.Int32
	)
	reg := mustBuildRegistry(
		t,
		[]Tool{tool},
		WithValidator(&testValidator{
			validateFn: func(_ context.Context, _ string, _ string) error {
				return errors.New("blocked by policy")
			},
		}),
		WithOnAfterExecute(func(_ context.Context, _ ToolCall, summary ExecutionSummary, _ time.Duration) {
			lastSummary = summary
			afterCalls.Add(1)
		}),
	)

	var chunks []Chunk
	err := reg.ExecuteBatchStream(
		context.Background(),
		[]ToolCall{{
			ToolName: "validator_soft_summary",
			Input:    ToolInput{CallID: "vs1", ArgsJSON: []byte(`{}`)},
		}},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsError)
	assert.Equal(t, int32(1), afterCalls.Load())
	require.NoError(t, lastSummary.Error)
	assert.Equal(t, 1, lastSummary.ErrorChunks)
	assert.Contains(t, lastSummary.LastErrorText, "blocked by policy")
	assert.Equal(t, 0, lastSummary.ChunksDelivered)
	assert.Equal(t, int64(0), lastSummary.TotalBytes)
}

func TestRegistry_Execute_ContextDeadlineMapsToErrTimeout(t *testing.T) {
	type A struct{}
	type R struct{}
	tool, err := NewTool("slow", "Slow", func(ctx context.Context, _ *RunEnv, _ A) (R, error) {
		<-ctx.Done()
		return R{}, ctx.Err()
	})
	require.NoError(t, err)

	reg := mustBuildRegistry(t, []Tool{tool})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = reg.Execute(
		ctx,
		ToolCall{ToolName: "slow", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	requireToolErrorCode(t, err, CodeTimeout, ErrTimeout)
}

func TestRegistry_ExecuteIter(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	tool, err := NewStreamTool(
		"iter_stream",
		"Stream",
		func(_ context.Context, _ *RunEnv, a A, yield func(Chunk) error) error {
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
		ToolName: "iter_stream",
		Input:    ToolInput{CallID: "iter1", ArgsJSON: []byte(`{"n": 5}`)},
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
	tool, err := NewTool("double", "Double", func(_ context.Context, _ *RunEnv, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)
	reg := mustBuildRegistry(t, []Tool{tool})

	calls := []ToolCall{
		{ToolName: "double", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{"x": 1}`)}},
		{ToolName: "missing", Input: ToolInput{CallID: "2", ArgsJSON: []byte(`{}`)}},
		{ToolName: "double", Input: ToolInput{CallID: "3", ArgsJSON: []byte(`{"x": 3}`)}},
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

func TestRegistry_ExecuteBatchStream_MiddlewareErrorAsChunk(t *testing.T) {
	type A struct{}
	type R struct {
		OK bool `json:"ok"`
	}
	tool, err := NewTool("guarded", "Guarded", func(_ context.Context, _ *RunEnv, _ A) (R, error) {
		return R{OK: true}, nil
	})
	require.NoError(t, err)

	errRateLimit := errors.New("rate limit exceeded")
	middleware := func(next Tool) Tool {
		return newMiddlewareMinTool(
			next.Manifest().Name,
			func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
				return errRateLimit
			},
		)
	}

	reg, err := NewRegistryBuilder().Use(middleware).Add(tool).Build()
	require.NoError(t, err)

	var chunks []Chunk
	err = reg.ExecuteBatchStream(
		context.Background(),
		[]ToolCall{{ToolName: "guarded", Input: ToolInput{CallID: "c1", ArgsJSON: []byte(`{}`)}}},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, "c1", chunks[0].CallID)
	assert.Equal(t, "guarded", chunks[0].ToolName)
	assert.True(t, chunks[0].IsError)
	assert.Equal(t, MimeTypeText, chunks[0].MimeType)
	assert.Contains(t, string(chunks[0].Data), errRateLimit.Error())
}

func TestRegistry_ExecuteBatchStream_SyntheticErrorChunk_NormalizesEmptyErrorText(t *testing.T) {
	tool := newMiddlewareMinTool(
		"empty_err",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return errors.New("")
		},
	)
	reg := mustBuildRegistry(t, []Tool{tool})

	var chunks []Chunk
	err := reg.ExecuteBatchStream(
		context.Background(),
		[]ToolCall{{ToolName: "empty_err", Input: ToolInput{CallID: "c-empty", ArgsJSON: []byte(`{}`)}}},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsError)
	assert.Equal(t, MimeTypeText, chunks[0].MimeType)
	assert.NotEmpty(t, string(chunks[0].Data))
	assert.Equal(t, "Error executing tool.", string(chunks[0].Data))
}

func TestRegistry_ExecuteBatchStream_SyntheticErrorChunk_NormalizesInvalidUTF8(t *testing.T) {
	tool := newMiddlewareMinTool(
		"utf8_err",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return invalidUTF8Error{}
		},
	)
	reg := mustBuildRegistry(t, []Tool{tool})

	var chunks []Chunk
	err := reg.ExecuteBatchStream(
		context.Background(),
		[]ToolCall{{ToolName: "utf8_err", Input: ToolInput{CallID: "c-utf8", ArgsJSON: []byte(`{}`)}}},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsError)
	assert.Equal(t, MimeTypeText, chunks[0].MimeType)
	assert.True(t, utf8.Valid(chunks[0].Data))
	assert.Contains(t, string(chunks[0].Data), "x")
}

func TestRegistry_ExecuteBatchStream_ReturnsErrStreamAbortedOnYieldError(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("double_abort", "Double", func(_ context.Context, _ *RunEnv, a A) (R, error) {
		return R{Y: a.X * 2}, nil
	})
	require.NoError(t, err)
	reg := mustBuildRegistry(t, []Tool{tool})

	var chunks []Chunk
	err = reg.ExecuteBatchStream(
		context.Background(),
		[]ToolCall{{ToolName: "double_abort", Input: ToolInput{CallID: "c1", ArgsJSON: []byte(`{"x": 2}`)}}},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return errors.New("client closed")
		},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStreamAborted)
	require.Len(t, chunks, 1)
}

func TestRegistry_ExecuteBatchStream_MissingToolYieldErrorReturnsStreamAborted(t *testing.T) {
	reg := mustBuildRegistry(t, nil)

	yieldCalls := 0
	err := reg.ExecuteBatchStream(
		context.Background(),
		[]ToolCall{{ToolName: "missing", Input: ToolInput{CallID: "m1", ArgsJSON: []byte(`{}`)}}},
		func(_ Chunk) error {
			yieldCalls++
			return errors.New("client closed")
		},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStreamAborted)
	assert.Equal(t, 1, yieldCalls)
}

func TestRegistry_ExecuteBatchStream_StreamAbortCancelsSiblings(t *testing.T) {
	startedSecond := make(chan struct{})
	var secondCanceled atomic.Bool

	tool := newMiddlewareMinTool(
		"batch_abort",
		func(ctx context.Context, _ *RunEnv, input ToolInput, yield func(Chunk) error) error {
			if input.CallID == "c1" {
				<-startedSecond
				if err := yield(Chunk{Event: EventResult, Data: []byte("first"), MimeType: MimeTypeText}); err != nil {
					return wrapYieldError(err)
				}
				return nil
			}
			close(startedSecond)
			<-ctx.Done()
			secondCanceled.Store(true)
			return ctx.Err()
		},
	)
	reg := mustBuildRegistry(t, []Tool{tool})

	var chunks []Chunk
	err := reg.ExecuteBatchStream(
		context.Background(),
		[]ToolCall{
			{ToolName: "batch_abort", Input: ToolInput{CallID: "c1", ArgsJSON: []byte(`{}`)}},
			{ToolName: "batch_abort", Input: ToolInput{CallID: "c2", ArgsJSON: []byte(`{}`)}},
		},
		func(c Chunk) error {
			chunks = append(chunks, c)
			return errors.New("client closed")
		},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStreamAborted)
	require.Len(t, chunks, 1)
	assert.True(t, secondCanceled.Load())
}

func TestRegistry_ExecuteBatchStream_StreamAbortPreventsExtraCallbackOnValidatorFailure(t *testing.T) {
	firstYieldReturned := make(chan struct{})
	allowFirstReturn := make(chan struct{})
	tool := newMiddlewareMinTool(
		"batch_abort_mix",
		func(_ context.Context, _ *RunEnv, input ToolInput, yield func(Chunk) error) error {
			if input.CallID != "c1" {
				return nil
			}
			if err := yield(Chunk{Event: EventResult, Data: []byte("first"), MimeType: MimeTypeText}); err != nil {
				close(firstYieldReturned)
				<-allowFirstReturn
				return wrapYieldError(err)
			}
			return nil
		},
	)

	reg := mustBuildRegistry(
		t,
		[]Tool{tool},
		WithValidator(&testValidator{
			validateFn: func(_ context.Context, toolName, argsJSON string) error {
				if toolName == "batch_abort_mix" && argsJSON == `{"x":2}` {
					<-firstYieldReturned
					close(allowFirstReturn)
					return errors.New("blocked by policy")
				}
				return nil
			},
		}),
	)

	var yieldCalls atomic.Int32
	err := reg.ExecuteBatchStream(
		context.Background(),
		[]ToolCall{
			{ToolName: "batch_abort_mix", Input: ToolInput{CallID: "c1", ArgsJSON: []byte(`{"x":1}`)}},
			{ToolName: "batch_abort_mix", Input: ToolInput{CallID: "c2", ArgsJSON: []byte(`{"x":2}`)}},
		},
		func(Chunk) error {
			yieldCalls.Add(1)
			return errors.New("client closed")
		},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStreamAborted)
	assert.Equal(t, int32(1), yieldCalls.Load())
}

func TestRegistry_ValidatorFailClosed(t *testing.T) {
	type A struct {
		Query string `json:"query"`
	}
	type R struct {
		OK bool `json:"ok"`
	}
	tool, err := NewTool("sql", "SQL", func(_ context.Context, _ *RunEnv, _ A) (R, error) {
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
			ToolName: "sql",
			Input:    ToolInput{CallID: "v1", ArgsJSON: []byte(`{"query":"drop table users"}`)},
		},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	requireClientCorrectable(t, err)
	require.ErrorIs(t, err, ErrValidation)
}

func TestRegistry_ShutdownRejectsNewCalls(t *testing.T) {
	reg := mustBuildRegistry(t, nil)
	require.NoError(t, reg.Shutdown(context.Background()))
	err := reg.Execute(
		context.Background(),
		ToolCall{ToolName: "noop", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	requireToolErrorCode(t, err, CodeShutdown, ErrShutdown)
}

func TestRegistry_ConcurrentExecute_BothSucceed(t *testing.T) {
	type A struct{}
	type R struct {
		OK bool `json:"ok"`
	}
	tool, err := NewTool("quick", "Quick", func(_ context.Context, _ *RunEnv, _ A) (R, error) {
		return R{OK: true}, nil
	})
	require.NoError(t, err)

	reg := mustBuildRegistry(t, []Tool{tool})

	errCh := make(chan error, 2)
	for i := range 2 {
		go func(id int) {
			errCh <- reg.Execute(
				context.Background(),
				ToolCall{ToolName: "quick", Input: ToolInput{CallID: fmt.Sprintf("c%d", id), ArgsJSON: []byte(`{}`)}},
				func(Chunk) error { return nil },
			)
		}(i)
	}
	for range 2 {
		require.NoError(t, <-errCh)
	}
}

func TestRegistry_OnChunkCountsOnlySuccess(t *testing.T) {
	type A struct{}

	var chunkCount atomic.Int32
	tool, err := NewStreamTool(
		"stream",
		"stream",
		func(_ context.Context, _ *RunEnv, _ A, yield func(Chunk) error) error {
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
		ToolCall{ToolName: "stream", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
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
	tool, err := NewTool("inc", "Inc", func(_ context.Context, _ *RunEnv, a A) (R, error) {
		return R{Y: a.X + 1}, nil
	})
	require.NoError(t, err)
	reg := mustBuildRegistry(t, []Tool{tool})

	const n = 20
	calls := make([]ToolCall, n)
	for i := range n {
		calls[i] = ToolCall{
			ToolName: "inc",
			Input: ToolInput{
				CallID:   time.Now().Add(time.Duration(i) * time.Nanosecond).Format("150405.000000000"),
				ArgsJSON: []byte(`{"x": 0}`),
			},
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
