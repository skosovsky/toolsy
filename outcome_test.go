package toolsy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionErrorFromChunk_ToolErrorJSON(t *testing.T) {
	t.Parallel()
	data, err := marshalToolErrorWire(NewValidationError("bad field", "city"), "Error executing tool: bad field")
	require.NoError(t, err)

	te := executionErrorFromChunk(Chunk{
		Event:    EventResult,
		Data:     data,
		MimeType: MimeTypeToolErrorJSON,
		IsError:  true,
	})
	require.NotNil(t, te)
	assert.Equal(t, CodeValidationFailed, te.Code)
	assert.Equal(t, []string{"city"}, te.FixableArgs)
}

func TestExecutionErrorFromChunk_PlainTextJSONBody(t *testing.T) {
	t.Parallel()
	body := []byte(`{"code":"VALIDATION_FAILED","retryable":false,"reason":"ignored"}`)
	te := executionErrorFromChunk(Chunk{
		Event:    EventResult,
		Data:     body,
		MimeType: MimeTypeText,
		IsError:  true,
	})
	require.NotNil(t, te)
	assert.Equal(t, CodeInternal, te.Code)
}

func TestExecutionErrorFromChunk_CorruptToolErrorJSON(t *testing.T) {
	t.Parallel()
	te := executionErrorFromChunk(Chunk{
		Event:    EventResult,
		Data:     []byte(`{not-json`),
		MimeType: MimeTypeToolErrorJSON,
		IsError:  true,
	})
	require.NotNil(t, te)
	assert.Equal(t, CodeInternal, te.Code)
}

func TestUnmarshalToolErrorWire_ToolsContractMissing(t *testing.T) {
	t.Parallel()
	orig := NewToolsContractMissingError([]string{"a", "b"}, []string{"b"})
	data, err := marshalToolErrorWire(orig, "contract missing")
	require.NoError(t, err)

	te, err := unmarshalToolErrorWire(data)
	require.NoError(t, err)
	assert.Equal(t, CodeToolsContractMissing, te.Code)
	assert.Equal(t, []string{"b"}, te.FixableArgs)
	require.Error(t, te.Unwrap())
}

func TestDecodeOutcomeAs_Success(t *testing.T) {
	t.Parallel()
	type result struct {
		N int `json:"n"`
	}
	raw, err := json.Marshal(result{N: 7})
	require.NoError(t, err)

	decoded, err := DecodeOutcomeAs[result](ToolOutcome{
		Result:         raw,
		ResultMimeType: MimeTypeJSON,
	})
	require.NoError(t, err)
	require.Equal(t, 7, decoded.N)
}

func TestDecodeOutcomeAs_ExecutionError(t *testing.T) {
	t.Parallel()
	execErr := NewValidationError("bad")
	_, err := DecodeOutcomeAs[struct{}](ToolOutcome{ExecutionError: execErr})
	require.ErrorIs(t, err, execErr)
}

func TestDecodeOutcomeAs_InvalidResultJSON(t *testing.T) {
	t.Parallel()
	_, err := DecodeOutcomeAs[struct {
		N int `json:"n"`
	}](ToolOutcome{
		Result:         []byte(`not-json`),
		ResultMimeType: MimeTypeJSON,
	})
	require.Error(t, err)
}
