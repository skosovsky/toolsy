package toolsy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func requireInternalErrorContains(t *testing.T, err error, substring string) {
	t.Helper()
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeInternal, te.Code)
	require.ErrorContains(t, te.Unwrap(), substring)
}

func TestValidateChunk_Success(t *testing.T) {
	err := validateChunk(Chunk{
		Event:    EventResult,
		Data:     []byte(`{"ok":true}`),
		MimeType: MimeTypeJSON,
	})
	require.NoError(t, err)
}

func TestValidateChunk_MissingEvent(t *testing.T) {
	err := validateChunk(Chunk{})
	requireInternalErrorContains(t, err, "chunk event is required")
}

func TestValidateChunk_UnsupportedEvent(t *testing.T) {
	err := validateChunk(Chunk{Event: "unknown"})
	requireInternalErrorContains(t, err, "unsupported chunk event")
}

func TestValidateChunk_ErrorChunkMissingData(t *testing.T) {
	err := validateChunk(Chunk{
		Event:   EventResult,
		IsError: true,
	})
	requireInternalErrorContains(t, err, "error chunks must include payload in Data")
}

func TestValidateChunk_ErrorChunkToolErrorJSON(t *testing.T) {
	data, err := marshalToolErrorWire(NewValidationError("bad"), "Error executing tool: bad")
	require.NoError(t, err)
	err = validateChunk(Chunk{
		Event:    EventResult,
		IsError:  true,
		Data:     data,
		MimeType: MimeTypeToolErrorJSON,
	})
	require.NoError(t, err)
}

func TestValidateChunk_ErrorChunkWrongMime(t *testing.T) {
	err := validateChunk(Chunk{
		Event:    EventResult,
		IsError:  true,
		Data:     []byte("oops"),
		MimeType: MimeTypeJSON,
	})
	requireInternalErrorContains(t, err, "error chunks require mime type")
}

func TestValidateChunk_ErrorChunkInvalidUTF8(t *testing.T) {
	err := validateChunk(Chunk{
		Event:    EventResult,
		IsError:  true,
		Data:     []byte{0xff, 0xfe},
		MimeType: MimeTypeText,
	})
	requireInternalErrorContains(t, err, "error chunks must contain valid UTF-8 text")
}

func TestValidateChunk_DataWithoutMime(t *testing.T) {
	err := validateChunk(Chunk{Event: EventResult, Data: []byte("x")})
	requireInternalErrorContains(t, err, "chunk data requires mime type")
}

func TestValidateChunk_MimeWithoutData(t *testing.T) {
	err := validateChunk(Chunk{Event: EventResult, MimeType: MimeTypeText})
	requireInternalErrorContains(t, err, "chunk mime type without data is invalid")
}
