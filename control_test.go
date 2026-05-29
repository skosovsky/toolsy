package toolsy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestYieldControl_PauseReturnsErrPause(t *testing.T) {
	var got Chunk
	err := YieldControl(func(c Chunk) error {
		got = c
		return nil
	}, &PauseSignal{Reason: "wait"})
	require.ErrorIs(t, err, ErrPause)
	assert.Equal(t, EventControl, got.Event)
	pause, ok := got.Control.(*PauseSignal)
	require.True(t, ok)
	assert.Equal(t, "wait", pause.Reason)
}

func TestYieldControl_YieldReturnsErrYield(t *testing.T) {
	err := YieldControl(func(Chunk) error { return nil }, &YieldSignal{Result: "done"})
	require.ErrorIs(t, err, ErrYield)
}

func TestYieldControl_HaltReturnsErrHalt(t *testing.T) {
	err := YieldControl(func(Chunk) error { return nil }, &HaltSignal{Reason: "stop"})
	require.ErrorIs(t, err, ErrHalt)
}

func TestYieldControl_UIActionReturnsErrUIAction(t *testing.T) {
	var got Chunk
	err := YieldControl(func(c Chunk) error {
		got = c
		return nil
	}, &UIActionSignal{Action: "open_panel", PayloadJSON: []byte(`{"id":"x"}`)})
	require.ErrorIs(t, err, ErrUIAction)
	assert.Equal(t, EventControl, got.Event)
	ui, ok := got.Control.(*UIActionSignal)
	require.True(t, ok)
	assert.Equal(t, "open_panel", ui.Action)
	assert.JSONEq(t, `{"id":"x"}`, string(ui.PayloadJSON))
}

func TestYieldControl_NilSignalIsSystemError(t *testing.T) {
	err := YieldControl(func(Chunk) error { return nil }, nil)
	require.True(t, IsSystemError(err))
}

func TestIsControlError(t *testing.T) {
	assert.True(t, IsControlError(ErrPause))
	assert.True(t, IsControlError(ErrYield))
	assert.True(t, IsControlError(ErrHalt))
	assert.True(t, IsControlError(ErrUIAction))
	assert.False(t, IsControlError(errors.New("other")))
}

func TestWithErrorFormatter_BypassesControlErrors(t *testing.T) {
	inner := newMiddlewareMinTool(
		"ctrl",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			return ErrPause
		},
	)
	wrapped := WithErrorFormatter()(inner)
	err := wrapped.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	require.ErrorIs(t, err, ErrPause)
}

func TestValidateChunk_ControlRequiresSignal(t *testing.T) {
	err := validateChunk(Chunk{Event: EventControl})
	require.True(t, IsSystemError(err))
}

func TestWithCompletionPolicy(t *testing.T) {
	type A struct{}
	type R struct{}
	tool, err := NewTool("t", "d", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{}, nil
	}, WithCompletionPolicy(CompletionSilentYield))
	require.NoError(t, err)
	assert.Equal(t, CompletionSilentYield, tool.Manifest().CompletionPolicy)
}
