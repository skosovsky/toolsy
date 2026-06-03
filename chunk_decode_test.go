package toolsy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeChunkAs_JSON(t *testing.T) {
	type out struct {
		Sum int `json:"sum"`
	}
	data, err := json.Marshal(out{Sum: 3})
	require.NoError(t, err)
	c := Chunk{Event: EventResult, MimeType: MimeTypeJSON, Data: data}
	got, err := DecodeChunkAs[out](c)
	require.NoError(t, err)
	require.Equal(t, 3, got.Sum)
}

func TestDecodeChunkAs_WrongMime(t *testing.T) {
	type out struct {
		X int `json:"x"`
	}
	_, err := DecodeChunkAs[out](Chunk{Event: EventResult, MimeType: MimeTypeText, Data: []byte("hi")})
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeSchemaInvalid, te.Code)
}

func TestDecodeChunkAs_ErrorChunk(t *testing.T) {
	type out struct {
		X int `json:"x"`
	}
	_, err := DecodeChunkAs[out](Chunk{
		Event:    EventResult,
		MimeType: MimeTypeJSON,
		Data:     []byte(`{}`),
		IsError:  true,
	})
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeSchemaInvalid, te.Code)
}

func TestDecodeChunkAs_InvalidJSON(t *testing.T) {
	type out struct {
		X int `json:"x"`
	}
	_, err := DecodeChunkAs[out](Chunk{Event: EventResult, MimeType: MimeTypeJSON, Data: []byte("{")})
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeSchemaInvalid, te.Code)
	require.Equal(t, "invalid JSON", te.Reason)
}

func TestDecodeChunkAsText(t *testing.T) {
	s, err := DecodeChunkAsText(Chunk{Event: EventResult, MimeType: MimeTypeText, Data: []byte("ok")})
	require.NoError(t, err)
	require.Equal(t, "ok", s)
}
