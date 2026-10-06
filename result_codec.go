package toolsy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// JSONResultCodec preserves the declared result/effect types, the complete
// delivery envelope and built-in control signals. Metadata must be JSON-shaped.
// Hosts with non-JSON values provide their own ResultCodec instead.
// Dynamic JSON numbers decode as [json.Number] without float64 rounding;
// arbitrary concrete Go types inside interfaces are not reconstructed.
type JSONResultCodec[R, E any] struct{}

type cachedControl struct {
	Kind    string `json:"kind"`
	Text    string `json:"text"`
	Payload []byte `json:"payload"`
}

type cachedResult[R, E any] struct {
	Format            string            `json:"format"`
	HasResult         bool              `json:"has_result"`
	HasEnvelopeResult bool              `json:"has_envelope_result"`
	Data              []byte            `json:"data"`
	MIME              string            `json:"mime"`
	Result            *R                `json:"result"`
	EnvelopeResult    *R                `json:"envelope_result"`
	Empty             bool              `json:"empty"`
	Noop              bool              `json:"noop"`
	Effects           []E               `json:"effects"`
	Controls          []cachedControl   `json:"controls"`
	Delivery          ToolDeliveryClass `json:"delivery"`
	Audience          ToolAudience      `json:"audience"`
	Metadata          map[string]any    `json:"metadata"`
}

// EncodeResult encodes one successful terminal result without type erasure.
func (JSONResultCodec[R, E]) EncodeResult(chunk Chunk) ([]byte, error) {
	if chunk.Event != EventResult || chunk.IsError {
		return nil, errors.New("codec requires a successful result")
	}
	env := chunk.ToolEnvelope()
	if env.Kind != ToolEnvelopeKindResult || env.Error != nil || !bytes.Equal(env.Raw, chunk.Data) ||
		env.MimeType != chunk.MimeType {
		return nil, errors.New("unsupported inconsistent delivery envelope")
	}
	stored := cachedResult[R, E]{Data: chunk.Data, MIME: chunk.MimeType, Empty: chunk.EmptyResult, Noop: chunk.Noop,
		Delivery: env.DeliveryClass, Audience: env.Audience, Metadata: env.Metadata,
		Result: nil, EnvelopeResult: nil, Effects: nil, Controls: nil,
		Format: "toolsy.complete_result", HasResult: chunk.TypedResult != nil, HasEnvelopeResult: env.Result != nil}
	if chunk.TypedResult != nil {
		result, ok := chunk.TypedResult.(R)
		if !ok {
			return nil, errors.New("unsupported result type for cache codec")
		}
		stored.Result = &result
	}
	if env.Result != nil {
		result, ok := env.Result.(R)
		if !ok {
			return nil, errors.New("unsupported envelope result type for cache codec")
		}
		stored.EnvelopeResult = &result
	}
	for _, effect := range chunk.Effects {
		typed, ok := effect.(E)
		if !ok {
			return nil, errors.New("unsupported effect type for cache codec")
		}
		stored.Effects = append(stored.Effects, typed)
	}
	for _, control := range chunk.Controls {
		encoded, err := encodeCachedControl(control)
		if err != nil {
			return nil, err
		}
		stored.Controls = append(stored.Controls, encoded)
	}
	return json.Marshal(stored)
}

// DecodeResult restores the declared result/effect types and delivery semantics.
func (JSONResultCodec[R, E]) DecodeResult(raw []byte) (Chunk, error) {
	var stored cachedResult[R, E]
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&stored); err != nil {
		return Chunk{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Chunk{}, errors.New("cached result must contain exactly one JSON document")
	}
	if stored.Format != "toolsy.complete_result" {
		return Chunk{}, errors.New("unsupported cached result format")
	}
	if stored.Audience == "" || stored.Delivery == "" {
		return Chunk{}, errors.New("cached result has no delivery binding")
	}
	chunk := Chunk{
		Event:       EventResult,
		Data:        stored.Data,
		MimeType:    stored.MIME,
		EmptyResult: stored.Empty,
		Noop:        stored.Noop,
	}
	if stored.HasResult {
		var result R
		if stored.Result != nil {
			result = *stored.Result
		}
		chunk.TypedResult = result
	}
	chunk.Effects = effectsToAny(stored.Effects)
	for _, encoded := range stored.Controls {
		control, err := decodeCachedControl(encoded)
		if err != nil {
			return Chunk{}, err
		}
		chunk.Controls = append(chunk.Controls, control)
	}
	chunk.Envelope = NewResultEnvelope(
		chunk.TypedResult,
		chunk.Data,
		chunk.MimeType,
		stored.Delivery,
		stored.Audience,
		stored.Metadata,
	)
	if stored.HasEnvelopeResult {
		var result R
		if stored.EnvelopeResult != nil {
			result = *stored.EnvelopeResult
		}
		chunk.Envelope.Result = result
	} else {
		chunk.Envelope.Result = nil
	}
	return prepareChunk(chunk)
}

func encodeCachedControl(control ControlSignal) (cachedControl, error) {
	switch value := control.(type) {
	case *PauseSignal:
		if value != nil {
			return cachedControl{Kind: "pause", Text: value.Reason, Payload: nil}, nil
		}
	case *YieldSignal:
		if value != nil {
			return cachedControl{Kind: "yield", Text: value.Result, Payload: nil}, nil
		}
	case *HaltSignal:
		if value != nil {
			return cachedControl{Kind: "halt", Text: value.Reason, Payload: nil}, nil
		}
	case *UIActionSignal:
		if value != nil {
			return cachedControl{Kind: "ui", Text: value.Action, Payload: value.PayloadJSON}, nil
		}
	}
	return cachedControl{}, errors.New("unsupported control for cache codec")
}

func decodeCachedControl(control cachedControl) (ControlSignal, error) {
	switch control.Kind {
	case "pause":
		return &PauseSignal{Reason: control.Text}, nil
	case "yield":
		return &YieldSignal{Result: control.Text}, nil
	case "halt":
		return &HaltSignal{Reason: control.Text}, nil
	case "ui":
		return &UIActionSignal{Action: control.Text, PayloadJSON: control.Payload}, nil
	default:
		return nil, errors.New("unsupported cached control")
	}
}
