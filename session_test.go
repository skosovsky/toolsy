package toolsy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionTrackExecutionCount(t *testing.T) {
	type A struct{}
	type R struct{}
	tool, err := NewTool("noop_count", "Noop", func(_ context.Context, _ A) (R, error) {
		return R{}, nil
	})
	require.NoError(t, err)

	reg := NewRegistry()
	reg.Register(tool)
	session := NewSession(reg, WithMaxSteps(0))

	assert.Equal(t, int64(0), session.Track().ExecutionCount())
	for range 2 {
		err = session.Execute(
			context.Background(),
			ToolCall{ID: "c", ToolName: "noop_count", Args: raw(`{}`)},
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
	tool, err := NewTool("noop", "Noop", func(_ context.Context, _ A) (R, error) {
		executed = true
		return R{}, nil
	})
	require.NoError(t, err)

	reg := NewRegistry(WithValidator(&testValidator{validateFn: func(_ context.Context, _, _ string) error {
		return errors.New("always reject")
	}}))
	reg.Register(tool)
	session := NewSession(reg, WithMaxSteps(10))

	err = session.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "noop", Args: raw(`{}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	require.False(t, executed)
	assert.Equal(t, int64(1), session.Track().ExecutionCount())
}

func TestSessionMaxStepsExceeded(t *testing.T) {
	type A struct{}
	type R struct{}
	tool, err := NewTool("noop", "Noop", func(_ context.Context, _ A) (R, error) {
		return R{}, nil
	})
	require.NoError(t, err)

	reg := NewRegistry()
	reg.Register(tool)
	session := NewSession(reg, WithMaxSteps(3))

	for i := 1; i <= 3; i++ {
		err = session.Execute(
			context.Background(),
			ToolCall{ID: string(rune('0' + i)), ToolName: "noop", Args: raw(`{}`)},
			func(Chunk) error { return nil },
		)
		require.NoError(t, err)
	}
	err = session.Execute(
		context.Background(),
		ToolCall{ID: "4", ToolName: "noop", Args: raw(`{}`)},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrMaxStepsExceeded)
	assert.Equal(t, int64(4), session.Track().ExecutionCount())
}

func TestSessionExecuteIterTracksSteps(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	tool, err := NewStreamTool("count", "Count", func(_ context.Context, a A, yield func(Chunk) error) error {
		for i := range a.N {
			if err := yield(Chunk{Event: EventProgress, Data: []byte{byte(i)}, MimeType: MimeTypeText}); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)

	reg := NewRegistry()
	reg.Register(tool)
	session := NewSession(reg, WithMaxSteps(2))

	var seen int
	for chunk, iterErr := range session.ExecuteIter(context.Background(), ToolCall{
		ID: "1", ToolName: "count", Args: raw(`{"n":2}`),
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
	tool, err := NewTool("noop", "Noop", func(_ context.Context, _ A) (R, error) {
		return R{}, nil
	})
	require.NoError(t, err)

	reg := NewRegistry()
	reg.Register(tool)
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
				ToolCall{ID: string(rune('a' + i)), ToolName: "noop", Args: raw(`{}`)},
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
	reg := NewRegistry()
	session := NewSession(reg, WithMaxSteps(0))

	err := session.Execute(
		context.Background(),
		ToolCall{ID: "missing", ToolName: "missing", Args: raw(`{}`)},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrToolNotFound)
	assert.Equal(t, int64(1), session.Track().ExecutionCount())
}

func TestSessionShutdownConsumesStep(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Shutdown(context.Background()))
	session := NewSession(reg, WithMaxSteps(0))

	err := session.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "noop", Args: raw(`{}`)},
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
	tool, err := NewTool("wait", "Wait", func(_ context.Context, _ A) (R, error) {
		close(blocked)
		<-release
		return R{}, nil
	})
	require.NoError(t, err)

	reg := NewRegistry(
		WithMaxConcurrency(1),
	)
	reg.Register(tool)
	session := NewSession(reg, WithMaxSteps(0))

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- session.Execute(
			context.Background(),
			ToolCall{ID: "1", ToolName: "wait", Args: raw(`{}`)},
			func(Chunk) error { return nil },
		)
	}()
	<-blocked

	waitCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = session.Execute(
		waitCtx,
		ToolCall{ID: "2", ToolName: "wait", Args: raw(`{}`)},
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

	var executed bool
	var beforeCount int
	var validateCount int
	tool, err := NewTool("noop", "Noop", func(_ context.Context, _ A) (R, error) {
		executed = true
		return R{}, nil
	})
	require.NoError(t, err)

	reg := NewRegistry(
		WithValidator(&testValidator{validateFn: func(_ context.Context, _, _ string) error {
			validateCount++
			return nil
		}}),
		WithOnBeforeExecute(func(_ context.Context, _ ToolCall) {
			beforeCount++
		}),
	)
	reg.Register(tool)
	session := NewSession(reg, WithMaxSteps(1))

	err = session.Execute(
		context.Background(),
		ToolCall{ID: "1", ToolName: "noop", Args: raw(`{}`)},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	require.True(t, executed)
	require.Equal(t, 1, beforeCount)
	require.Equal(t, 1, validateCount)

	executed = false
	err = session.Execute(
		context.Background(),
		ToolCall{ID: "2", ToolName: "noop", Args: raw(`{}`)},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrMaxStepsExceeded)
	require.False(t, executed)
	assert.Equal(t, 1, beforeCount)
	assert.Equal(t, 1, validateCount)
	assert.Equal(t, int64(2), session.Track().ExecutionCount())
}
