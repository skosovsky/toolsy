package toolsy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func requireSystemErrorContains(t *testing.T, err error, substring string) {
	t.Helper()
	var systemErr *SystemError
	require.ErrorAs(t, err, &systemErr)
	require.ErrorContains(t, systemErr.Err, substring)
}

func TestValidateChunk_Success(t *testing.T) {
	err := validateChunk(Chunk{
		Event:    EventResult,
		Data:     []byte(`{"ok":true}`),
		MimeType: MimeTypeJSON,
	})
	require.NoError(t, err)
}

func TestValidateChunk_RejectsMissingEvent(t *testing.T) {
	err := validateChunk(Chunk{
		Data:     []byte(`{"ok":true}`),
		MimeType: MimeTypeJSON,
	})
	require.Error(t, err)
	requireSystemErrorContains(t, err, "chunk event is required")
}

func TestValidateChunk_RejectsUnsupportedEvent(t *testing.T) {
	err := validateChunk(Chunk{
		Event:    EventType("unknown"),
		Data:     []byte("payload"),
		MimeType: MimeTypeText,
	})
	require.Error(t, err)
	requireSystemErrorContains(t, err, "unsupported chunk event")
}

func TestValidateChunk_ErrorChunkSuccess(t *testing.T) {
	err := validateChunk(Chunk{
		Event:    EventResult,
		IsError:  true,
		Data:     []byte("boom"),
		MimeType: MimeTypeText,
	})
	require.NoError(t, err)
}

func TestValidateChunk_ErrorChunkRejectsMissingData(t *testing.T) {
	err := validateChunk(Chunk{
		Event:   EventResult,
		IsError: true,
	})
	require.Error(t, err)
	requireSystemErrorContains(t, err, "error chunks must include UTF-8 text in Data")
}

func TestValidateChunk_ErrorChunkRejectsNonTextMimeType(t *testing.T) {
	err := validateChunk(Chunk{
		Event:    EventResult,
		IsError:  true,
		Data:     []byte(`{"error":"boom"}`),
		MimeType: MimeTypeJSON,
	})
	require.Error(t, err)
	requireSystemErrorContains(t, err, "error chunks require mime type")
}

func TestValidateChunk_ErrorChunkRejectsInvalidUTF8(t *testing.T) {
	err := validateChunk(Chunk{
		Event:    EventResult,
		IsError:  true,
		Data:     []byte{0xff, 0xfe, 0xfd},
		MimeType: MimeTypeText,
	})
	require.Error(t, err)
	requireSystemErrorContains(t, err, "error chunks must contain valid UTF-8 text")
}

func TestValidateChunk_DataWithoutMimeTypeIsRejected(t *testing.T) {
	err := validateChunk(Chunk{
		Event: EventResult,
		Data:  []byte("payload"),
	})
	require.Error(t, err)
	requireSystemErrorContains(t, err, "chunk data requires mime type")
}

func TestValidateChunk_MimeTypeWithoutDataIsRejected(t *testing.T) {
	err := validateChunk(Chunk{
		Event:    EventResult,
		MimeType: MimeTypeText,
	})
	require.Error(t, err)
	requireSystemErrorContains(t, err, "chunk mime type without data is invalid")
}
