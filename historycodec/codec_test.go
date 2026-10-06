package historycodec_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/historycodec"
)

func TestToolCallRoundtrip(t *testing.T) {
	// Arrange.
	call := toolsy.ToolCall{
		ToolName: "weather",
		Input:    toolsy.ToolInput{CallID: "call-1", ArgsJSON: []byte(`{"city":"Paris"}`)},
	}
	// Act.
	data, err := historycodec.MarshalToolCall(call)
	require.NoError(t, err)
	back, err := historycodec.UnmarshalToolCall(data)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, call, back)
}

func TestResultPreservesDeliveryAndReplayProvenance(t *testing.T) {
	for _, audience := range []toolsy.ToolAudience{toolsy.AudienceInternal, toolsy.AudienceUser, toolsy.AudienceModel} {
		t.Run(string(audience), func(t *testing.T) {
			// Arrange.
			data := []byte{0, 255, 42}
			chunk := toolsy.Chunk{
				CallID:      "call-1",
				ToolName:    "binary",
				Event:       toolsy.EventResult,
				Data:        data,
				MimeType:    "application/octet-stream",
				EmptyResult: true,
				Noop:        true,
				Envelope: toolsy.NewResultEnvelope(
					nil,
					data,
					"application/octet-stream",
					toolsy.DeliveryClassBinary,
					audience,
					map[string]any{
						toolsy.CacheReplayMetadata: true,
						"source": map[string]any{
							"sequence": json.Number("9007199254740993"),
							"tags":     []any{"safe", nil},
						},
					},
				),
			}
			// Act.
			wire, err := historycodec.MarshalToolResult(chunk)
			require.NoError(t, err)
			back, err := historycodec.UnmarshalToolResult(wire)
			// Assert.
			require.NoError(t, err)
			require.Equal(t, chunk, back)
			require.Equal(t, audience, back.ToolEnvelope().Audience)
			require.Equal(t, true, back.ToolEnvelope().Metadata[toolsy.CacheReplayMetadata])
		})
	}
}

func TestRawResultMaterializesDeliveryBinding(t *testing.T) {
	// Arrange.
	chunk := toolsy.Chunk{Event: toolsy.EventResult, Data: []byte("hi"), MimeType: "text/plain"}
	// Act.
	wire, err := historycodec.MarshalToolResult(chunk)
	require.NoError(t, err)
	back, err := historycodec.UnmarshalToolResult(wire)
	// Assert.
	require.NoError(t, err)
	require.NotNil(t, back.Envelope)
	require.Equal(t, chunk.ToolEnvelope(), back.ToolEnvelope())
}

func TestSoftErrorClassificationRoundtrip(t *testing.T) {
	// Arrange.
	data := []byte(`{"error":"denied"}`)
	errInfo := &toolsy.ToolError{
		Code:        toolsy.CodeValidationFailed,
		Reason:      "denied",
		SafeMessage: "fix input",
		FixableArgs: []string{"query"},
	}
	chunk := toolsy.Chunk{
		Event:    toolsy.EventResult,
		Data:     data,
		MimeType: toolsy.MimeTypeJSON,
		IsError:  true,
		Envelope: toolsy.NewErrorEnvelope(
			errInfo,
			data,
			toolsy.MimeTypeJSON,
			toolsy.DeliveryClassStructured,
			toolsy.AudienceInternal,
			nil,
		),
	}
	// Act.
	wire, err := historycodec.MarshalToolResult(chunk)
	require.NoError(t, err)
	back, err := historycodec.UnmarshalToolResult(wire)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, chunk, back)
}

func TestUnsupportedExecutionStateFailsEncoding(t *testing.T) {
	var typedNil *string
	for name, mutate := range map[string]func(*toolsy.Chunk){
		"control event":   func(c *toolsy.Chunk) { c.Event = toolsy.EventControl },
		"progress event":  func(c *toolsy.Chunk) { c.Event = toolsy.EventProgress },
		"typed result":    func(c *toolsy.Chunk) { c.TypedResult = "typed" },
		"typed nil":       func(c *toolsy.Chunk) { c.TypedResult = typedNil },
		"effects":         func(c *toolsy.Chunk) { c.Effects = []any{"effect"} },
		"controls":        func(c *toolsy.Chunk) { c.Controls = []toolsy.ControlSignal{&toolsy.PauseSignal{Reason: "wait"}} },
		"control":         func(c *toolsy.Chunk) { c.Control = &toolsy.PauseSignal{Reason: "wait"} },
		"progress":        func(c *toolsy.Chunk) { c.Progress = &toolsy.ProgressInfo{} },
		"envelope result": func(c *toolsy.Chunk) { c.Envelope = toolsy.NewResultEnvelope("typed", nil, "", "", "", nil) },
		"wrapped error": func(c *toolsy.Chunk) {
			c.IsError = true
			c.Envelope = toolsy.NewErrorEnvelope(&toolsy.ToolError{Code: toolsy.CodeValidationFailed, Err: errors.New("runtime")}, nil, "", "", "", nil)
		},
		"custom metadata": func(c *toolsy.Chunk) {
			c.Envelope = toolsy.NewResultEnvelope(nil, nil, "", "", "", map[string]any{"custom": struct{ Secret string }{"x"}})
		},
		"cycle": func(c *toolsy.Chunk) {
			metadata := map[string]any{}
			metadata["cycle"] = metadata
			c.Envelope = &toolsy.ToolEnvelope{Kind: toolsy.ToolEnvelopeKindResult, Audience: toolsy.AudienceModel, DeliveryClass: toolsy.DeliveryClassStructured, Metadata: metadata}
		},
		"inconsistent envelope": func(c *toolsy.Chunk) { c.Envelope = toolsy.NewResultEnvelope(nil, []byte("other"), "", "", "", nil) },
	} {
		t.Run(name, func(t *testing.T) {
			// Arrange.
			chunk := toolsy.Chunk{Event: toolsy.EventResult}
			mutate(&chunk)
			// Act.
			wire, err := historycodec.MarshalToolResult(chunk)
			// Assert.
			require.Error(t, err)
			require.Nil(t, wire)
		})
	}
}

func TestCallsRejectContextAndAttachments(t *testing.T) {
	for name, call := range map[string]toolsy.ToolCall{
		"subject":     {CallContext: toolsy.CallContext{Subject: "secret"}},
		"scope":       {CallContext: toolsy.CallContext{Scope: "tenant"}},
		"values":      {CallContext: toolsy.CallContext{Values: map[string]any{"token": "secret"}}},
		"metadata":    {CallContext: toolsy.CallContext{Metadata: toolsy.CallMetadata{ViewID: "view"}}},
		"environment": {Env: toolsy.NewRunEnv(nil)},
		"attachments": {Input: toolsy.ToolInput{Attachments: []toolsy.Attachment{{Data: []byte("file")}}}},
	} {
		t.Run(name, func(t *testing.T) {
			// Arrange: call contains unsupported state.
			// Act.
			wire, err := historycodec.MarshalToolCall(call)
			// Assert.
			require.Error(t, err)
			require.Nil(t, wire)
		})
	}
}

func TestDecodingRejectsLossyAndAmbiguousRecords(t *testing.T) {
	// Arrange.
	wire, err := historycodec.MarshalToolResult(
		toolsy.Chunk{
			Event: toolsy.EventResult,
			Envelope: toolsy.NewResultEnvelope(
				nil,
				nil,
				"",
				"",
				toolsy.AudienceInternal,
				map[string]any{toolsy.CacheReplayMetadata: true},
			),
		},
	)
	require.NoError(t, err)
	valid := string(wire)
	cases := map[string]string{
		"v1":               strings.Replace(valid, `"v":2`, `"v":1`, 1),
		"missing version":  strings.Replace(valid, `"v":2,`, "", 1),
		"missing audience": strings.Replace(valid, `"audience":"internal",`, "", 1),
		"null audience":    strings.Replace(valid, `"audience":"internal"`, `"audience":null`, 1),
		"unknown audience": strings.Replace(valid, `"audience":"internal"`, `"audience":"everyone"`, 1),
		"duplicate audience": strings.Replace(
			valid,
			`"audience":"internal"`,
			`"audience":"internal","audience":"model"`,
			1,
		),
		"case alias audience": strings.Replace(
			valid,
			`"audience":"internal"`,
			`"audience":"internal","Audience":"model"`,
			1,
		),
		"unknown field":     strings.Replace(valid, `"v":2`, `"v":2,"typed_result":42`, 1),
		"trailing document": valid + ` {}`,
		"trailing garbage":  valid + ` !`,
		"missing boolean":   strings.Replace(valid, `"is_error":false,`, "", 1),
		"null boolean":      strings.Replace(valid, `"is_error":false`, `"is_error":null`, 1),
		"null envelope": strings.Replace(
			valid,
			valid[strings.Index(valid, `"envelope":`):len(valid)-1],
			`"envelope":null`,
			1,
		),
		"duplicate metadata": strings.Replace(
			valid,
			`"toolsy.cache_replay":true`,
			`"toolsy.cache_replay":true,"toolsy.cache_replay":false`,
			1,
		),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			// Act.
			back, err := historycodec.UnmarshalToolResult([]byte(data))
			// Assert.
			require.Error(t, err)
			require.Equal(t, toolsy.Chunk{}, back)
		})
	}
}

func TestToolCallStrictDecoding(t *testing.T) {
	// Arrange.
	wire, err := historycodec.MarshalToolCall(toolsy.ToolCall{ToolName: "x"})
	require.NoError(t, err)
	valid := string(wire)
	for _, data := range []string{`{"v":1,"tool_name":"x","args_json":null}`, valid + `{}`, strings.Replace(valid, `"tool_name":"x"`, `"tool_name":"x","ToolName":"evil"`, 1), strings.Replace(valid, `"kind":"tool_call"`, `"kind":"tool_result"`, 1)} {
		// Act.
		_, err := historycodec.UnmarshalToolCall([]byte(data))
		// Assert.
		require.Error(t, err)
	}
}
