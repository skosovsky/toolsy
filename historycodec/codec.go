package historycodec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/skosovsky/toolsy"
)

const wireVersion = 2

type wireToolCall struct {
	Version  int    `json:"v"`
	Kind     string `json:"kind"`
	ToolName string `json:"tool_name"`
	CallID   string `json:"call_id"`
	ArgsJSON []byte `json:"args_json"`
}

type wireError struct {
	Code        toolsy.ErrorCode `json:"code"`
	Retryable   bool             `json:"retryable"`
	Reason      string           `json:"reason"`
	FixableArgs []string         `json:"fixable_args"`
	SafeMessage string           `json:"safe_message"`
}

type wireEnvelope struct {
	Kind          toolsy.ToolEnvelopeKind  `json:"kind"`
	Error         *wireError               `json:"error"`
	DeliveryClass toolsy.ToolDeliveryClass `json:"delivery_class"`
	Audience      toolsy.ToolAudience      `json:"audience"`
	Raw           []byte                   `json:"raw"`
	MimeType      string                   `json:"mime_type"`
	Metadata      map[string]any           `json:"metadata"`
}

type wireToolResult struct {
	Version  int          `json:"v"`
	Kind     string       `json:"kind"`
	CallID   string       `json:"call_id"`
	ToolName string       `json:"tool_name"`
	Data     []byte       `json:"data"`
	MimeType string       `json:"mime_type"`
	IsError  bool         `json:"is_error"`
	Empty    bool         `json:"empty"`
	Noop     bool         `json:"noop"`
	Envelope wireEnvelope `json:"envelope"`
}

// MarshalToolCall encodes an unbound transcript call; execution context and attachments fail explicitly.
func MarshalToolCall(call toolsy.ToolCall) ([]byte, error) {
	var unbound toolsy.CallContext
	if call.Env != nil || !reflect.DeepEqual(call.CallContext, unbound) || len(call.Input.Attachments) != 0 {
		return nil, errors.New("historycodec: call context, environment and attachments are unsupported")
	}
	return json.Marshal(
		wireToolCall{
			Version:  wireVersion,
			Kind:     "tool_call",
			ToolName: call.ToolName,
			CallID:   call.Input.CallID,
			ArgsJSON: call.Input.ArgsJSON,
		},
	)
}

// UnmarshalToolCall decodes exactly one strict version 2 transcript call.
func UnmarshalToolCall(data []byte) (toolsy.ToolCall, error) {
	var w wireToolCall
	if err := decodeStrict(data, &w, "v", "kind", "tool_name", "call_id", "args_json"); err != nil {
		return toolsy.ToolCall{}, fmt.Errorf("historycodec: tool call: %w", err)
	}
	if w.Version != wireVersion || w.Kind != "tool_call" {
		return toolsy.ToolCall{}, errors.New("historycodec: unsupported tool call version or kind")
	}
	return toolsy.ToolCall{ //nolint:exhaustruct_v5 // transcript deliberately excludes runtime bindings
		ToolName: w.ToolName, Input: toolsy.ToolInput{CallID: w.CallID, ArgsJSON: w.ArgsJSON, Attachments: nil}}, nil
}

// MarshalToolResult encodes a raw delivered result with explicit delivery binding.
// Complete typed outcomes require toolsy.ResultCodec instead.
func MarshalToolResult(chunk toolsy.Chunk) ([]byte, error) {
	if chunk.Event != toolsy.EventResult || chunk.TypedResult != nil || len(chunk.Effects) != 0 ||
		len(chunk.Controls) != 0 ||
		chunk.Control != nil ||
		chunk.Progress != nil {
		return nil, errors.New(
			"historycodec: only raw result transcripts without effects, controls or progress are supported",
		)
	}
	var env toolsy.ToolEnvelope
	if chunk.Envelope == nil {
		env = chunk.ToolEnvelope()
	} else {
		env = *chunk.Envelope
	}
	if env.Result != nil || env.Error != nil && env.Error.Err != nil {
		return nil, errors.New("historycodec: typed envelope results and wrapped errors are unsupported")
	}
	if err := validateMetadata(env.Metadata, make(map[uintptr]bool)); err != nil {
		return nil, err
	}
	w := wireToolResult{
		Version:  wireVersion,
		Kind:     "tool_result",
		CallID:   chunk.CallID,
		ToolName: chunk.ToolName,
		Data:     chunk.Data,
		MimeType: chunk.MimeType,
		IsError:  chunk.IsError,
		Empty:    chunk.EmptyResult,
		Noop:     chunk.Noop,
		Envelope: wireEnvelope{
			Error:         nil,
			Kind:          env.Kind,
			DeliveryClass: env.DeliveryClass,
			Audience:      env.Audience,
			Raw:           env.Raw,
			MimeType:      env.MimeType,
			Metadata:      env.Metadata,
		},
	}
	if env.Error != nil {
		w.Envelope.Error = &wireError{
			Code:        env.Error.Code,
			Retryable:   env.Error.Retryable,
			Reason:      env.Error.Reason,
			FixableArgs: env.Error.FixableArgs,
			SafeMessage: env.Error.SafeMessage,
		}
	}
	if err := validateResult(w); err != nil {
		return nil, err
	}
	return json.Marshal(w)
}

// UnmarshalToolResult restores explicit transcript delivery semantics without runtime execution state.
func UnmarshalToolResult(data []byte) (toolsy.Chunk, error) {
	var w wireToolResult
	if err := decodeStrict(
		data,
		&w,
		"v",
		"kind",
		"call_id",
		"tool_name",
		"data",
		"mime_type",
		"is_error",
		"empty",
		"noop",
		"envelope",
	); err != nil {
		return toolsy.Chunk{}, fmt.Errorf("historycodec: tool result: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return toolsy.Chunk{}, err
	}
	var envFields map[string]json.RawMessage
	if err := json.Unmarshal(fields["envelope"], &envFields); err != nil {
		return toolsy.Chunk{}, err
	}
	if err := requireFields(
		envFields,
		"kind",
		"error",
		"delivery_class",
		"audience",
		"raw",
		"mime_type",
		"metadata",
	); err != nil {
		return toolsy.Chunk{}, err
	}
	if w.Envelope.Error != nil {
		var errorFields map[string]json.RawMessage
		if err := json.Unmarshal(envFields["error"], &errorFields); err != nil {
			return toolsy.Chunk{}, err
		}
		if err := requireFields(
			errorFields,
			"code",
			"retryable",
			"reason",
			"fixable_args",
			"safe_message",
		); err != nil {
			return toolsy.Chunk{}, err
		}
	}
	if err := validateResult(w); err != nil {
		return toolsy.Chunk{}, err
	}
	env := &toolsy.ToolEnvelope{
		Result:        nil,
		Error:         nil,
		Kind:          w.Envelope.Kind,
		DeliveryClass: w.Envelope.DeliveryClass,
		Audience:      w.Envelope.Audience,
		Raw:           w.Envelope.Raw,
		MimeType:      w.Envelope.MimeType,
		Metadata:      w.Envelope.Metadata,
	}
	if w.Envelope.Error != nil {
		err := w.Envelope.Error
		env.Error = &toolsy.ToolError{
			Code:        err.Code,
			Retryable:   err.Retryable,
			Reason:      err.Reason,
			FixableArgs: err.FixableArgs,
			SafeMessage: err.SafeMessage,
			Err:         nil,
		}
	}
	return toolsy.Chunk{
		CallID:      w.CallID,
		ToolName:    w.ToolName,
		Event:       toolsy.EventResult,
		Data:        w.Data,
		MimeType:    w.MimeType,
		IsError:     w.IsError,
		EmptyResult: w.Empty,
		Noop:        w.Noop,
		Envelope:    env,
	}, nil
}

func validateResult(w wireToolResult) error {
	if w.Version != wireVersion || w.Kind != "tool_result" {
		return errors.New("historycodec: unsupported tool result version or kind")
	}
	e := w.Envelope
	if e.Audience != toolsy.AudienceInternal && e.Audience != toolsy.AudienceModel &&
		e.Audience != toolsy.AudienceUser {
		return errors.New("historycodec: explicit supported audience is required")
	}
	if e.DeliveryClass != toolsy.DeliveryClassBinary && e.DeliveryClass != toolsy.DeliveryClassStructured &&
		e.DeliveryClass != toolsy.DeliveryClassText {
		return errors.New("historycodec: explicit supported delivery class is required")
	}
	if w.IsError && (e.Kind != toolsy.ToolEnvelopeKindError || e.Error == nil) ||
		!w.IsError && (e.Kind != toolsy.ToolEnvelopeKindResult || e.Error != nil) {
		return errors.New("historycodec: inconsistent result/error classification")
	}
	if !bytes.Equal(e.Raw, w.Data) || e.MimeType != w.MimeType {
		return errors.New("historycodec: inconsistent raw delivery envelope")
	}
	return nil
}

func validateMetadata(value any, active map[uintptr]bool) error {
	switch v := value.(type) {
	case nil, bool, string, float64, json.Number:
		return nil // json.Marshal rejects nonfinite floats and invalid json.Number values.
	case []any:
		ptr := reflect.ValueOf(v).Pointer()
		if ptr != 0 && active[ptr] {
			return errors.New("historycodec: cyclic metadata")
		}
		active[ptr] = true
		defer delete(active, ptr)
		for _, item := range v {
			if err := validateMetadata(item, active); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		ptr := reflect.ValueOf(v).Pointer()
		if ptr != 0 && active[ptr] {
			return errors.New("historycodec: cyclic metadata")
		}
		active[ptr] = true
		defer delete(active, ptr)
		for _, item := range v {
			if err := validateMetadata(item, active); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("historycodec: unsupported metadata type %T", value)
	}
}

func decodeStrict(data []byte, dst any, required ...string) error {
	probe := json.NewDecoder(bytes.NewReader(data))
	probe.UseNumber()
	if err := scanValue(probe); err != nil {
		return err
	}
	if _, err := probe.Token(); err != io.EOF {
		return errors.New("expected exactly one JSON document")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	return requireFields(fields, required...)
}

func requireFields(fields map[string]json.RawMessage, required ...string) error {
	allowed := make(map[string]bool, len(required))
	for _, key := range required {
		allowed[key] = true
		value, ok := fields[key]
		if !ok {
			return fmt.Errorf("historycodec: missing field %q", key)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			switch key {
			case "args_json", "data", "raw", "error", "metadata", "fixable_args":
			default:
				return fmt.Errorf("historycodec: null field %q", key)
			}
		}
	}
	for key := range fields {
		if !allowed[key] {
			return fmt.Errorf("historycodec: unknown field %q", key)
		}
	}
	return nil
}

func scanValue(decoder *json.Decoder) error {
	token, tokenErr := decoder.Token()
	if tokenErr != nil {
		return tokenErr
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		if err := scanObject(decoder); err != nil {
			return err
		}
	case '[':
		for decoder.More() {
			if err := scanValue(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected closing delimiter")
	}
	_, closingErr := decoder.Token()
	return closingErr
}

func scanObject(decoder *json.Decoder) error {
	seen := make(map[string]bool)
	for decoder.More() {
		keyToken, tokenErr := decoder.Token()
		if tokenErr != nil {
			return tokenErr
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("invalid object key")
		}
		if seen[key] {
			return fmt.Errorf("duplicate field %q", key)
		}
		seen[key] = true
		if err := scanValue(decoder); err != nil {
			return err
		}
	}
	return nil
}
