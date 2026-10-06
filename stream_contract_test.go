package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerminalStreamContract(t *testing.T) {
	result := Chunk{Event: EventResult, Data: []byte(`{"n":1}`), MimeType: MimeTypeJSON}
	progress := Chunk{Event: EventProgress, Data: []byte("working"), MimeType: MimeTypeText}
	rawSchema, err := os.ReadFile("testdata/execution/terminal-schema.json")
	require.NoError(t, err)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(rawSchema, &schema))
	tests := []struct {
		name    string
		produce InvocationHandler
		kind    string
		limit   int
		results int
	}{
		{"valid", func(y func(Chunk) error) error {
			if err := y(progress); err != nil {
				return err
			}
			return y(result)
		}, "", 0, 1},
		{"invalid", func(y func(Chunk) error) error {
			return y(Chunk{Event: EventResult, Data: []byte(`{"n":"wrong"}`), MimeType: MimeTypeJSON})
		}, "schema_mismatch", 0, 0},
		{"ignored-invalid", func(y func(Chunk) error) error {
			_ = y(Chunk{Event: EventResult, Data: []byte(`null`), MimeType: MimeTypeJSON})
			return nil
		}, "schema_mismatch", 0, 0},
		{"duplicate", func(y func(Chunk) error) error {
			if err := y(result); err != nil {
				return err
			}
			_ = y(result)
			return nil
		}, "duplicate_terminal", 0, 0},
		{"missing", func(y func(Chunk) error) error { return y(progress) }, "missing_terminal", 0, 0},
		{"error-after-result", func(y func(Chunk) error) error {
			if err := y(result); err != nil {
				return err
			}
			return errors.New("producer failed")
		}, "aborted", 0, 0},
		{"limit", func(y func(Chunk) error) error { return y(result) }, "output_limit", 2, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: each invocation owns its terminal candidate and validator state.
			tool, err := NewStreamTool(
				"terminal",
				"Terminal",
				func(_ context.Context, _ *RunEnv, _ struct{}, y func(Chunk) error) error { return test.produce(y) },
				WithTerminalStream(test.limit),
				WithOutputSchema(schema),
			)
			require.NoError(t, err)
			var results int
			// Act.
			err = tool.Execute(
				context.Background(),
				NewRunEnv(nil),
				ToolInput{ArgsJSON: []byte(`{}`)},
				func(c Chunk) error {
					if c.Event == EventResult {
						results++
					}
					return nil
				},
			)
			// Assert: failed producers never deliver a final successful result.
			assert.Equal(t, test.results, results)
			if test.kind == "" {
				require.NoError(t, err)
			} else {
				var contractErr *StreamContractError
				require.ErrorAs(t, err, &contractErr)
				assert.Equal(t, test.kind, contractErr.Kind)
				var te *ToolError
				require.ErrorAs(t, err, &te)
				assert.False(t, te.Retryable)
			}
		})
	}
}

func TestTerminalStreamControlAndConsumerAbort(t *testing.T) {
	for _, pause := range []bool{false, true} {
		// Arrange: a producer deliberately ignores the callback failure.
		tool, err := NewStreamTool(
			"interrupted",
			"Interrupted",
			func(_ context.Context, _ *RunEnv, _ struct{}, y func(Chunk) error) error {
				if pause {
					_ = YieldControl(y, &PauseSignal{Reason: "approval"})
				} else {
					_ = y(Chunk{Event: EventProgress, Data: []byte("p"), MimeType: MimeTypeText})
				}
				_ = y(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
				return nil
			},
			WithTerminalStream(0),
			WithOutputSchema(map[string]any{"type": "object"}),
		)
		require.NoError(t, err)
		var results int
		// Act.
		err = tool.Execute(
			context.Background(),
			NewRunEnv(nil),
			ToolInput{ArgsJSON: []byte(`{}`)},
			func(c Chunk) error {
				if c.Event == EventResult {
					results++
				}
				if pause {
					return nil
				}
				return errors.New("consumer closed")
			},
		)
		// Assert: neither an ignored pause nor abort becomes completed.
		assert.Zero(t, results)
		if pause {
			require.ErrorIs(t, err, ErrPause)
		} else {
			var contractErr *StreamContractError
			require.ErrorAs(t, err, &contractErr)
			assert.Equal(t, "aborted", contractErr.Kind)
		}
	}
}

func TestStreamSemanticsMustBeExplicit(t *testing.T) {
	// Arrange/Act: no implicit mode, terminal schema or negative limit.
	fn := func(context.Context, *RunEnv, struct{}, func(Chunk) error) error { return nil }
	_, err := NewStreamTool("implicit", "Implicit", fn)
	require.ErrorContains(t, err, "explicit semantics")
	_, err = NewStreamTool("schema", "Schema", fn, WithTerminalStream(0))
	require.ErrorContains(t, err, "output schema")
	_, err = NewStreamTool(
		"limit",
		"Limit",
		fn,
		WithTerminalStream(-1),
		WithOutputSchema(map[string]any{"type": "object"}),
	)
	require.ErrorContains(t, err, "negative")
	// Assert: independent results explicitly retain their own useful semantics.
	tool, err := NewStreamTool(
		"independent",
		"Independent",
		func(_ context.Context, _ *RunEnv, _ struct{}, y func(Chunk) error) error {
			for range 2 {
				if yieldErr := y(
					Chunk{Event: EventResult, Data: []byte("data"), MimeType: MimeTypeText},
				); yieldErr != nil {
					return yieldErr
				}
			}
			return nil
		},
		WithIndependentStream(),
	)
	require.NoError(t, err)
	var results int
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			nil,
			ToolInput{ArgsJSON: []byte(`{}`)},
			func(Chunk) error { results++; return nil },
		),
	)
	assert.Equal(t, 2, results)
}

func TestTerminalStreamConcurrentCalls(t *testing.T) {
	// Arrange: one immutable tool used concurrently.
	tool, err := NewStreamTool(
		"concurrent",
		"Concurrent",
		func(_ context.Context, _ *RunEnv, _ struct{}, y func(Chunk) error) error {
			return y(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
		WithTerminalStream(0),
		WithOutputSchema(map[string]any{"type": "object"}),
	)
	require.NoError(t, err)
	var wg sync.WaitGroup
	errorsCh := make(chan error, 8)
	// Act.
	for range 8 {
		wg.Go(func() {
			var count int
			err := tool.Execute(
				context.Background(),
				nil,
				ToolInput{ArgsJSON: []byte(`{}`)},
				func(Chunk) error { count++; return nil },
			)
			if err == nil && count != 1 {
				err = errors.New("incorrect terminal count")
			}
			errorsCh <- err
		})
	}
	wg.Wait()
	close(errorsCh)
	// Assert.
	for err := range errorsCh {
		require.NoError(t, err)
	}
}
