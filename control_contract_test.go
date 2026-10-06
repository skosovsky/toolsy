package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestControlContractInvalidSignalsNeverDelivered(t *testing.T) {
	tests := []struct {
		name   string
		signal ControlSignal
	}{
		{
			"nil",
			nil,
		}, {"typed-nil-pause", (*PauseSignal)(nil)}, {"typed-nil-yield", (*YieldSignal)(nil)}, {"typed-nil-halt", (*HaltSignal)(nil)}, {"typed-nil-host", (*HostEventSignal)(nil)},
		{"text-over", &PauseSignal{Reason: strings.Repeat("x", MaxControlBytes+1)}},
		{"invalid-text", &YieldSignal{Result: string([]byte{0xff})}},
		{
			"name-empty",
			&HostEventSignal{},
		}, {"name-over", &HostEventSignal{Name: strings.Repeat("a", MaxHostEventNameBytes+1)}},
		{"name-route-injection", &HostEventSignal{Name: "ui refresh"}},
		{"json-invalid", &HostEventSignal{Name: "event", PayloadJSON: []byte(`{`)}},
		{"json-invalid-utf8", &HostEventSignal{Name: "event", PayloadJSON: []byte{'"', 0xff, '"'}}},
		{"json-duplicate", &HostEventSignal{Name: "event", PayloadJSON: []byte(`{"a":1,"a":2}`)}},
		{
			"json-depth",
			&HostEventSignal{
				Name:        "event",
				PayloadJSON: []byte(strings.Repeat("[", 129) + "0" + strings.Repeat("]", 129)),
			},
		},
		{
			"combined-over",
			&HostEventSignal{Name: "event", PayloadJSON: []byte(`"` + strings.Repeat("a", MaxControlBytes-2) + `"`)},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange.
			calls := 0
			// Act.
			err := YieldControl(func(Chunk) error { calls++; return nil }, test.signal)
			// Assert.
			require.Equal(t, 0, calls)
			requireControlContractFailure(t, err)
		})
	}
}

func requireControlContractFailure(t *testing.T, err error) {
	t.Helper()
	var contract *ResultContractError
	require.ErrorAs(t, err, &contract)
	require.Equal(t, "control_contract", contract.Kind)
	var te *ToolError
	require.ErrorAs(t, err, &te)
	require.Equal(t, CodeInternal, te.Code)
	require.False(t, te.Retryable)
	require.False(t, IsControlError(err))
}

func TestControlContractInclusiveBoundsAndSnapshot(t *testing.T) {
	// Arrange.
	signal := &HostEventSignal{
		Name:        strings.Repeat("a", MaxHostEventNameBytes),
		PayloadJSON: []byte(`"` + strings.Repeat("a", MaxControlBytes-MaxHostEventNameBytes-2) + `"`),
	}
	var delivered *HostEventSignal
	// Act: mutating producer data during delivery must not alter its delivered snapshot.
	err := YieldControl(func(c Chunk) error {
		delivered = c.Control.(*HostEventSignal)
		signal.Name = ""
		signal.PayloadJSON[1] = 'z'
		return nil
	}, signal)
	// Assert.
	require.ErrorIs(t, err, ErrHostEvent)
	require.Len(t, delivered.Name, MaxHostEventNameBytes)
	require.Equal(t, byte('a'), delivered.PayloadJSON[1])
	for _, sig := range []ControlSignal{&PauseSignal{Reason: strings.Repeat("x", MaxControlBytes)}, &YieldSignal{Result: strings.Repeat("x", MaxControlBytes)}, &HaltSignal{Reason: strings.Repeat("x", MaxControlBytes)}} {
		require.True(t, IsControlError(YieldControl(func(Chunk) error { return nil }, sig)))
	}
}

func TestControlContractConsumerErrorAndNilCallback(t *testing.T) {
	// Arrange.
	cause := errors.New("consumer stopped")
	// Act.
	err := YieldControl(func(Chunk) error { return cause }, &PauseSignal{Reason: "wait"})
	// Assert.
	require.ErrorIs(t, err, cause)
	require.False(t, IsControlError(err))
	requireControlContractFailure(t, YieldControl(nil, &PauseSignal{}))
}

func TestControlContractTypedResultsAndCodec(t *testing.T) {
	tests := []struct {
		name     string
		controls []ControlSignal
		valid    bool
	}{
		{"single", []ControlSignal{&HostEventSignal{Name: "index.updated", PayloadJSON: []byte(`{"id":"x"}`)}}, true},
		{"count-boundary", makeControls(MaxControlSignals, ""), true},
		{"count-over", makeControls(MaxControlSignals+1, ""), false},
		{"aggregate-boundary", makeControls(2, strings.Repeat("x", MaxControlBytes/2)), true},
		{"aggregate-over", makeControls(2, strings.Repeat("x", MaxControlBytes/2+1)), false},
		{"nil", []ControlSignal{(*HostEventSignal)(nil)}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange.
			tool, err := NewTypedTool(
				TypedToolSpec[NoSubject, NoScope, struct{}, string, string]{
					Name:        "control_result",
					Description: "control result",
					Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
						result := NewToolResult[string, string]("done")
						result.Controls = test.controls
						return result, nil
					},
				},
			)
			require.NoError(t, err)
			var delivered []Chunk
			// Act.
			err = tool.Execute(
				context.Background(),
				NewRunEnv(nil),
				ToolInput{ArgsJSON: []byte(`{}`)},
				func(c Chunk) error { delivered = append(delivered, c); return nil },
			)
			// Assert.
			codec := JSONResultCodec[string, string]{}
			if !test.valid {
				require.Empty(t, delivered)
				requireControlContractFailure(t, err)
				_, codecErr := codec.EncodeResult(Chunk{Event: EventResult, Controls: test.controls})
				requireControlContractFailure(t, codecErr)
				return
			}
			require.NoError(t, err) // Terminal declarations do not emit a control sentinel.
			require.Len(t, delivered, 1)
			data, err := codec.EncodeResult(delivered[0])
			require.NoError(t, err)
			restored, err := codec.DecodeResult(data)
			require.NoError(t, err)
			require.Equal(t, test.controls, restored.Controls)
		})
	}
}

func makeControls(count int, text string) []ControlSignal {
	controls := make([]ControlSignal, count)
	for i := range controls {
		controls[i] = &PauseSignal{Reason: text}
	}
	return controls
}

func TestControlContractCodecRejectsLegacyUIAndOversize(t *testing.T) {
	// Arrange.
	codec := JSONResultCodec[string, string]{}
	data, err := codec.EncodeResult(
		Chunk{Event: EventResult, Controls: []ControlSignal{&HostEventSignal{Name: "event"}}},
	)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(data, &wire))
	controls := wire["controls"].([]any)
	item := controls[0].(map[string]any)
	// Act/Assert: old UI kind cannot acquire neutral-event semantics silently.
	item["kind"] = "ui"
	legacy, err := json.Marshal(wire)
	require.NoError(t, err)
	_, err = codec.DecodeResult(legacy)
	require.Error(t, err)
	item["kind"] = "pause"
	item["text"] = strings.Repeat("x", MaxControlBytes+1)
	oversized, err := json.Marshal(wire)
	require.NoError(t, err)
	_, err = codec.DecodeResult(oversized)
	requireControlContractFailure(t, err)
}

func TestControlRequestsDoNotStopRegistryOrUnrelatedCalls(t *testing.T) {
	for _, sig := range []ControlSignal{&PauseSignal{Reason: "wait"}, &YieldSignal{Result: "done"}, &HaltSignal{Reason: "stop"}, &HostEventSignal{Name: "host.event"}} {
		t.Run(ControlErrorFromSignal(sig).Error(), func(t *testing.T) {
			// Arrange: a host still owns scheduling after receiving a control.
			tool, err := NewStreamTool(
				"control",
				"control",
				func(_ context.Context, _ *RunEnv, _ struct{}, y func(Chunk) error) error { return YieldControl(y, sig) },
				WithIndependentStream(),
			)
			require.NoError(t, err)
			next, err := NewTool(
				"next",
				"next",
				func(context.Context, *RunEnv, struct{}) (string, error) { return "next result", nil },
				WithCompletionPolicy(CompletionHalt),
			)
			require.NoError(t, err)
			registry, err := NewRegistryBuilder().Use(WithErrorFormatter()).Add(tool, next).Build()
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			controls := 0
			// Act.
			err = registry.Execute(
				ctx,
				ToolCall{ToolName: "control", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
				func(c Chunk) error { require.Equal(t, EventControl, c.Event); controls++; return nil },
			)
			// Assert.
			require.ErrorIs(t, err, ControlErrorFromSignal(sig))
			require.Equal(t, 1, controls)
			require.NoError(t, ctx.Err())
			results := 0
			require.NoError(
				t,
				registry.Execute(
					ctx,
					ToolCall{ToolName: "next", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
					func(c Chunk) error { results++; require.Equal(t, EventResult, c.Event); return nil },
				),
			)
			require.Equal(t, 1, results)
		})
	}
}

func TestControlContractMalformedChunksFailBeforeNormalization(t *testing.T) {
	tests := []Chunk{
		{Event: EventControl, Control: &PauseSignal{}, IsError: true, Data: []byte("failed"), MimeType: MimeTypeText},
		{Event: EventControl, Control: &PauseSignal{}, Data: []byte("data"), MimeType: MimeTypeText},
		{Event: EventResult, Control: &PauseSignal{}},
		{Event: EventProgress, Controls: []ControlSignal{&PauseSignal{}}},
	}
	for _, chunk := range tests {
		// Arrange/Act: error normalization must not conceal a malformed control declaration.
		_, err := prepareChunk(chunk)
		// Assert.
		requireControlContractFailure(t, err)
	}
}
