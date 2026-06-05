package toolsy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMiddlewareMinTool(
	name string,
	execute func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error,
) *minTool {
	return &minTool{
		manifest: ToolManifest{Name: name, Parameters: map[string]any{"type": "object"}},
		execute:  execute,
	}
}

func TestWithLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	inner := newMiddlewareMinTool(
		"log_me",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{"ok":true}`), MimeType: MimeTypeJSON})
		},
	)
	wrapped := WithLogging(logger)(inner)

	var out []byte
	err := wrapped.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(c Chunk) error {
			out = c.Data
			return nil
		},
	)
	require.NoError(t, err)
	assert.Equal(t, []byte(`{"ok":true}`), out)

	logStr := buf.String()
	assert.Contains(t, logStr, "tool start")
	assert.Contains(t, logStr, "tool end")
	assert.Contains(t, logStr, "log_me")
}

func TestWithLogging_SoftErrorChunkLogsToolError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	inner := newMiddlewareMinTool(
		"log_soft_error",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{
				Event:    EventResult,
				Data:     []byte("budget exceeded"),
				MimeType: MimeTypeText,
				IsError:  true,
			})
		},
	)
	wrapped := WithLogging(logger)(inner)

	err := wrapped.Execute(context.Background(), NewRunEnv(nil), ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error {
		return nil
	})
	require.NoError(t, err)

	logStr := buf.String()
	assert.Contains(t, logStr, "tool start")
	assert.Contains(t, logStr, "tool error")
	assert.Contains(t, logStr, "error_chunks=1")
	assert.Contains(t, logStr, "last_error_text=\"budget exceeded\"")
}

func TestWithRecovery(t *testing.T) {
	inner := newMiddlewareMinTool(
		"panic_me",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			panic("test panic")
		},
	)
	wrapped := WithRecovery()(inner)

	err := wrapped.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	var sysErr *ToolError
	require.ErrorAs(t, err, &sysErr)
	assert.Contains(t, sysErr.Err.Error(), "panic")
}

func TestRegistryBuilderUse(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	tool, err := NewTool("wrap_me", "desc", func(_ context.Context, _ *RunEnv, a A) (R, error) {
		return R{Y: a.X + 1}, nil
	})
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Use(WithLogging(logger)).Add(tool).Build()
	require.NoError(t, err)

	args, _ := json.Marshal(A{X: 2})
	var r R
	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "wrap_me", Input: ToolInput{CallID: "1", ArgsJSON: args}},
		func(c Chunk) error { return json.Unmarshal(c.Data, &r) },
	)
	require.NoError(t, err)
	assert.Equal(t, 3, r.Y)
	assert.Equal(t, 1, strings.Count(buf.String(), "tool start"))
}

func TestMiddlewareShortCircuitSkipsInnerTool(t *testing.T) {
	var called atomic.Bool
	inner := newMiddlewareMinTool(
		"blocked",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			called.Store(true)
			return nil
		},
	)

	errRateLimit := errors.New("rate limit exceeded")
	shortCircuit := func(next Tool) Tool {
		return newMiddlewareMinTool(
			next.Manifest().Name,
			func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
				return errRateLimit
			},
		)
	}

	reg, err := NewRegistryBuilder().Use(shortCircuit).Add(inner).Build()
	require.NoError(t, err)

	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "blocked", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, errRateLimit)
	assert.False(t, called.Load())
}

func TestAsAsyncTool_MiddlewareRunsInBackground(t *testing.T) {
	var (
		mwFinished atomic.Bool
		mwDuration time.Duration
		mwMu       sync.Mutex
	)

	trackMW := func(next Tool) Tool {
		return newMiddlewareMinTool(
			next.Manifest().Name,
			func(ctx context.Context, env *RunEnv, input ToolInput, yield func(Chunk) error) error {
				start := time.Now()
				err := next.Execute(ctx, env, input, yield)
				mwMu.Lock()
				mwDuration = time.Since(start)
				mwMu.Unlock()
				mwFinished.Store(true)
				return err
			},
		)
	}

	const sleep = 80 * time.Millisecond
	base := newMiddlewareMinTool(
		"heavy",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			time.Sleep(sleep)
			return nil
		},
	)
	reg, err := NewRegistryBuilder().
		Use(trackMW).
		Add(AsAsyncTool(base)).
		Build()
	require.NoError(t, err)

	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "heavy", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	assert.False(t, mwFinished.Load(), "middleware must not finish in sync accept path")

	require.Eventually(t, mwFinished.Load, time.Second, 10*time.Millisecond)
	mwMu.Lock()
	dur := mwDuration
	mwMu.Unlock()
	assert.GreaterOrEqual(t, dur, sleep, "middleware must measure background work duration")
}

func TestAsAsyncTool_BudgetSoftErrorInOnComplete(t *testing.T) {
	var (
		done     sync.WaitGroup
		gotErr   error
		executed atomic.Bool
	)
	done.Add(1)

	base := newMiddlewareMinTool(
		"budgeted",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			executed.Store(true)
			return nil
		},
	)
	tracker := &testBudgetTracker{
		allowFn: func(context.Context, ToolManifest, ToolInput) (bool, string, error) {
			return false, "quota exhausted", nil
		},
	}
	reg, err := NewRegistryBuilder().
		Use(WithBudget()).
		Add(AsAsyncTool(base, WithOnComplete(func(_ context.Context, _ string, _ []Chunk, err error) {
			gotErr = err
			done.Done()
		}))).
		Build()
	require.NoError(t, err)

	err = reg.Execute(
		context.Background(),
		ToolCall{
			ToolName: "budgeted",
			Input:    ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)},
			Env:      budgetEnv(tracker),
		},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.Error(t, gotErr)
	assert.Contains(t, gotErr.Error(), "quota exhausted")
	assert.False(t, executed.Load())
	te, ok := AsToolError(gotErr)
	require.True(t, ok)
	assert.Equal(t, CodeToolExecutionFailed, te.Code)
}

func TestAsAsyncTool_ErrorFormatterSoftErrorInOnComplete(t *testing.T) {
	var (
		done   sync.WaitGroup
		gotErr error
	)
	done.Add(1)

	const failMsg = "background tool failed"
	base := newMiddlewareMinTool(
		"failer",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			return errors.New(failMsg)
		},
	)
	reg, err := NewRegistryBuilder().
		Use(WithErrorFormatter()).
		Add(AsAsyncTool(base, WithOnComplete(func(_ context.Context, _ string, _ []Chunk, err error) {
			gotErr = err
			done.Done()
		}))).
		Build()
	require.NoError(t, err)

	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "failer", Input: ToolInput{CallID: "1", ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.Error(t, gotErr)
	assert.Contains(t, gotErr.Error(), failMsg)
	te, ok := AsToolError(gotErr)
	require.True(t, ok)
	assert.Equal(t, CodeToolExecutionFailed, te.Code)
}

func TestAsAsyncTool_BackgroundYieldValidationInOnComplete(t *testing.T) {
	var (
		done   sync.WaitGroup
		gotErr error
	)
	done.Add(1)

	base := newMiddlewareMinTool(
		"invalid_chunk",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			_ = yield(Chunk{Event: EventResult, Data: []byte("x"), MimeType: ""})
			return nil
		},
	)
	wrapped := AsAsyncTool(base, WithOnComplete(func(_ context.Context, _ string, _ []Chunk, err error) {
		gotErr = err
		done.Done()
	}))
	err := wrapped.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	require.NoError(t, err)
	done.Wait()
	require.Error(t, gotErr)
	te, ok := AsToolError(gotErr)
	require.True(t, ok)
	assert.Equal(t, CodeInternal, te.Code)
}
