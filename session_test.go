package toolsy

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSessionRegistry(t *testing.T, tools []Tool, opts ...RegistryOption) *Registry {
	t.Helper()
	builder := NewRegistryBuilder(opts...).Add(tools...)
	reg, err := builder.Build()
	require.NoError(t, err)
	return reg
}

func TestSessionTrackExecutionCount(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("noop_count", "Noop", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{}, nil
	})
	require.NoError(t, err)

	reg := newSessionRegistry(t, []Tool{tool})
	session := NewSession(reg, WithMaxSteps(0))

	assert.Equal(t, int64(0), session.Track().ExecutionCount())
	for range 2 {
		err = session.Execute(
			context.Background(),
			ToolCall{ID: "c", ToolName: "noop_count", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
			func(Chunk) error { return nil },
		)
		require.NoError(t, err)
	}
	assert.Equal(t, int64(2), session.Track().ExecutionCount())
	assert.Equal(t, int64(0), session.Track().MaxSteps())
}

func TestSessionValidatorFailureConsumesStep(t *testing.T) {
	type A struct{}
	type R struct{}

	var executed bool
	tool, err := NewTool("noop", "Noop", func(_ context.Context, _ RunContext, _ A) (R, error) {
		executed = true
		return R{}, nil
	})
	require.NoError(t, err)

	reg := newSessionRegistry(t, []Tool{tool}, WithValidator(&testValidator{
		validateFn: func(_ context.Context, _, _ string) error {
			return errors.New("always reject")
		},
	}))
	session := NewSession(reg, WithMaxSteps(10))

	err = session.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "noop", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	require.False(t, executed)
	assert.Equal(t, int64(1), session.Track().ExecutionCount())
}

func TestSessionMaxStepsExceeded(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("noop", "Noop", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{}, nil
	})
	require.NoError(t, err)

	reg := newSessionRegistry(t, []Tool{tool})
	session := NewSession(reg, WithMaxSteps(3))

	for i := 1; i <= 3; i++ {
		err = session.Execute(
			context.Background(),
			ToolCall{
				ID:       string(rune('0' + i)),
				ToolName: "noop",
				Input:    ToolInput{ArgsJSON: []byte(`{}`)},
			},
			func(Chunk) error { return nil },
		)
		require.NoError(t, err)
	}

	err = session.Execute(
		context.Background(),
		ToolCall{ID: "4", ToolName: "noop", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrMaxStepsExceeded)
	assert.Equal(t, int64(4), session.Track().ExecutionCount())
}

func TestSessionExecuteIterTracksSteps(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	tool, err := NewStreamTool(
		"count",
		"Count",
		func(_ context.Context, _ RunContext, a A, yield func(Chunk) error) error {
			for i := range a.N {
				if err := yield(
					Chunk{Event: EventProgress, Data: []byte{byte(i)}, MimeType: MimeTypeText},
				); err != nil {
					return err
				}
			}
			return nil
		},
	)
	require.NoError(t, err)

	reg := newSessionRegistry(t, []Tool{tool})
	session := NewSession(reg, WithMaxSteps(2))

	var seen int
	for chunk, iterErr := range session.ExecuteIter(context.Background(), ToolCall{
		ID:       "1",
		ToolName: "count",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":2}`)},
	}) {
		require.NoError(t, iterErr)
		require.Equal(t, EventProgress, chunk.Event)
		seen++
	}
	assert.Equal(t, 2, seen)
	assert.Equal(t, int64(1), session.Track().ExecutionCount())
}

func TestSessionConcurrentUseCountsSteps(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("noop", "Noop", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{}, nil
	})
	require.NoError(t, err)

	reg := newSessionRegistry(t, []Tool{tool})
	session := NewSession(reg, WithMaxSteps(0))

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	errCh := make(chan error, workers)
	for i := range workers {
		go func(i int) {
			defer wg.Done()
			errCh <- session.Execute(
				context.Background(),
				ToolCall{
					ID:       string(rune('a' + i)),
					ToolName: "noop",
					Input:    ToolInput{ArgsJSON: []byte(`{}`)},
				},
				func(Chunk) error { return nil },
			)
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
	assert.Equal(t, int64(workers), session.Track().ExecutionCount())
}

func TestSessionToolNotFoundConsumesStep(t *testing.T) {
	reg := newSessionRegistry(t, nil)
	session := NewSession(reg, WithMaxSteps(0))

	err := session.Execute(
		context.Background(),
		ToolCall{ID: "missing", ToolName: "missing", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrToolNotFound)
	assert.Equal(t, int64(1), session.Track().ExecutionCount())
}

func TestSessionShutdownConsumesStep(t *testing.T) {
	reg := newSessionRegistry(t, nil)
	require.NoError(t, reg.Shutdown(context.Background()))
	session := NewSession(reg, WithMaxSteps(0))

	err := session.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "noop", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrShutdown)
	assert.Equal(t, int64(1), session.Track().ExecutionCount())
}

func TestSessionSemaphoreTimeoutConsumesStep(t *testing.T) {
	type A struct{}
	type R struct{}

	blocked := make(chan struct{})
	release := make(chan struct{})
	tool, err := NewTool("wait", "Wait", func(_ context.Context, _ RunContext, _ A) (R, error) {
		close(blocked)
		<-release
		return R{}, nil
	})
	require.NoError(t, err)

	reg := newSessionRegistry(t, []Tool{tool}, WithMaxConcurrency(1))
	session := NewSession(reg, WithMaxSteps(0))

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- session.Execute(
			context.Background(),
			ToolCall{ID: "1", ToolName: "wait", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
			func(Chunk) error { return nil },
		)
	}()
	<-blocked

	waitCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = session.Execute(
		waitCtx,
		ToolCall{ID: "2", ToolName: "wait", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrTimeout)
	assert.Equal(t, int64(2), session.Track().ExecutionCount())

	close(release)
	require.NoError(t, <-firstDone)
}

func TestSessionOverBudgetShortCircuitsBeforeRegistryExecution(t *testing.T) {
	type A struct{}
	type R struct{}

	var executed atomic.Bool
	var beforeCount atomic.Int32
	var validateCount atomic.Int32

	tool, err := NewTool("noop", "Noop", func(_ context.Context, _ RunContext, _ A) (R, error) {
		executed.Store(true)
		return R{}, nil
	})
	require.NoError(t, err)

	reg := newSessionRegistry(
		t,
		[]Tool{tool},
		WithValidator(&testValidator{validateFn: func(_ context.Context, _, _ string) error {
			validateCount.Add(1)
			return nil
		}}),
		WithOnBeforeExecute(func(_ context.Context, _ ToolCall) {
			beforeCount.Add(1)
		}),
	)
	session := NewSession(reg, WithMaxSteps(1))

	err = session.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "noop", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	require.True(t, executed.Load())
	require.Equal(t, int32(1), beforeCount.Load())
	require.Equal(t, int32(1), validateCount.Load())

	executed.Store(false)
	err = session.Execute(
		context.Background(),
		ToolCall{ID: "2", ToolName: "noop", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrMaxStepsExceeded)
	require.False(t, executed.Load())
	assert.Equal(t, int32(1), beforeCount.Load())
	assert.Equal(t, int32(1), validateCount.Load())
	assert.Equal(t, int64(2), session.Track().ExecutionCount())
}
