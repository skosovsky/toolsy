package toolsy

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		Event:    EventResult,
		IsError:  true,
		MimeType: MimeTypeToolErrorJSON,
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

func TestValidateChunk_ErrorChunkRejectsPlainText(t *testing.T) {
	err := validateChunk(Chunk{
		Event:    EventResult,
		IsError:  true,
		Data:     []byte("oops"),
		MimeType: MimeTypeText,
	})
	requireInternalErrorContains(t, err, "error chunks require mime type")
}

func TestNormalizeErrorChunk_TextBecomesToolErrorJSON(t *testing.T) {
	normalized := normalizeErrorChunk(Chunk{
		Event:    EventResult,
		Data:     []byte("budget exceeded"),
		MimeType: MimeTypeText,
		IsError:  true,
	})
	require.True(t, normalized.IsError)
	require.Equal(t, MimeTypeToolErrorJSON, normalized.MimeType) //nolint:testifylint // mime type, not JSON document
	te, err := unmarshalToolErrorWire(normalized.Data)
	require.NoError(t, err)
	require.Equal(t, CodeInternal, te.Code)
	assert.Contains(t, te.Reason, "malformed error chunk")
	assert.Contains(t, te.Reason, "budget exceeded")
}

func TestPrepareChunk_NormalizesLegacyTextError(t *testing.T) {
	prepared, err := prepareChunk(Chunk{
		Event:    EventResult,
		Data:     []byte("soft fail"),
		MimeType: MimeTypeText,
		IsError:  true,
	})
	require.NoError(t, err)
	require.Equal(t, MimeTypeToolErrorJSON, prepared.MimeType) //nolint:testifylint // mime type, not JSON document
}

func TestPrepareChunk_WrongMimeJSONError(t *testing.T) {
	prepared, err := prepareChunk(Chunk{
		Event:    EventResult,
		Data:     []byte(`{"code":"validation_failed"}`),
		MimeType: MimeTypeJSON,
		IsError:  true,
	})
	require.NoError(t, err)
	require.Equal(t, MimeTypeToolErrorJSON, prepared.MimeType) //nolint:testifylint // mime type, not JSON document
}

func TestPrepareChunk_EmptyDataError(t *testing.T) {
	_, err := prepareChunk(Chunk{
		Event:    EventResult,
		IsError:  true,
		MimeType: MimeTypeToolErrorJSON,
	})
	requireInternalErrorContains(t, err, "error chunks must include payload in Data")
}

func TestValidateChunk_DataWithoutMime(t *testing.T) {
	err := validateChunk(Chunk{Event: EventResult, Data: []byte("x")})
	requireInternalErrorContains(t, err, "chunk data requires mime type")
}

func TestValidateChunk_MimeWithoutData(t *testing.T) {
	err := validateChunk(Chunk{Event: EventResult, MimeType: MimeTypeText})
	requireInternalErrorContains(t, err, "chunk mime type without data is invalid")
}
