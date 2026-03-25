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

func TestNormalizeChunk_RawDataBecomesJSON(t *testing.T) {
	chunk, err := normalizeChunk(Chunk{RawData: map[string]any{"ok": true}})
	require.NoError(t, err)
	require.Equal(t, EventResult, chunk.Event)
	if chunk.MimeType != MimeTypeJSON {
		t.Fatalf("unexpected mime type: %s", chunk.MimeType)
	}
	require.JSONEq(t, `{"ok":true}`, string(chunk.Data))
}

func TestNormalizeChunk_ErrorChunkDefaultsToTextMimeType(t *testing.T) {
	chunk, err := normalizeChunk(Chunk{IsError: true, Data: []byte("boom")})
	require.NoError(t, err)
	require.Equal(t, EventResult, chunk.Event)
	require.Equal(t, MimeTypeText, chunk.MimeType)
	require.Equal(t, []byte("boom"), chunk.Data)
}

func TestNormalizeChunk_ErrorChunkRejectsRawDataOnly(t *testing.T) {
	_, err := normalizeChunk(Chunk{IsError: true, RawData: map[string]any{"message": "boom"}})
	require.Error(t, err)
	requireSystemErrorContains(t, err, "error chunks must set Data as UTF-8 text")
}

func TestNormalizeChunk_DataWithoutMimeTypeIsRejected(t *testing.T) {
	_, err := normalizeChunk(Chunk{Data: []byte("payload")})
	require.Error(t, err)
	requireSystemErrorContains(t, err, "chunk data requires mime type")
}

func TestNormalizeChunk_ErrorChunkRejectsNonTextMimeType(t *testing.T) {
	_, err := normalizeChunk(Chunk{
		IsError:  true,
		Data:     []byte(`{"error":"boom"}`),
		MimeType: MimeTypeJSON,
	})
	require.Error(t, err)
	requireSystemErrorContains(t, err, "error chunks require mime type")
}

func TestNormalizeChunk_ErrorChunkRejectsInvalidUTF8(t *testing.T) {
	_, err := normalizeChunk(Chunk{
		IsError: true,
		Data:    []byte{0xff, 0xfe, 0xfd},
	})
	require.Error(t, err)
	requireSystemErrorContains(t, err, "error chunks must contain valid UTF-8 text")
}
