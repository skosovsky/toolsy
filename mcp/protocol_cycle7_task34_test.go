package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPinnedInputRequestUnionRejectsMalformedVariants(t *testing.T) {
	valid := []string{
		`{"sampling":{"method":"sampling/createMessage","params":{"messages":[{"role":"user","content":{"type":"text","text":"hello"}}],"maxTokens":1}}}`,
		`{"roots":{"method":"roots/list","future":{"x":1}}}`,
		`{"elicit":{"method":"elicitation/create","params":{"message":"Continue?","requestedSchema":{"type":"object","properties":{"confirmed":{"type":"boolean"}}}}}}`,
	}
	for _, raw := range valid {
		require.NoError(t, validateInputRequests(json.RawMessage(raw)), raw)
	}

	invalid := []string{
		`{"x":null}`,
		`{"x":{}}`,
		`{"x":{"method":"unknown","params":{}}}`,
		`{"x":{"method":"sampling/createMessage","params":{"messages":[]}}}`,
		`{"x":{"method":"roots/list","params":null}}`,
		`{"x":{"method":"elicitation/create","params":null}}`,
		`{"x":{"method":"elicitation/create","params":{"mode":"url","message":"Sign in"}}}`,
		`{"x":{"method":"elicitation/create","params":{"message":"Continue?","requestedSchema":{"type":"boolean"}}}}`,
	}
	for _, raw := range invalid {
		require.Error(t, validateInputRequests(json.RawMessage(raw)), raw)
	}

	var result InputRequiredResult
	err := json.Unmarshal(
		[]byte(`{"resultType":"input_required","inputRequests":{"x":{"method":"unknown","params":{}}}}`),
		&result,
	)
	require.Error(t, err)
}

func TestPinnedInputResponseUnionRunsBeforeAllOutboundMethods(t *testing.T) {
	valid := `{"sample":{"role":"assistant","content":{"type":"text","text":"done"},"model":"m"},"roots":{"roots":[{"uri":"file:///tmp"}]},"elicit":{"action":"accept","content":{"confirmed":true,"labels":["a"],"count":1},"future":{}}}`
	meta := `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"c","version":"1"}}`
	validFrames := []struct {
		raw    string
		target any
	}{
		{`{"name":"tool","inputResponses":` + valid + "," + meta + "}", &ToolsCallParams{}},
		{`{"uri":"file:///a","inputResponses":` + valid + "," + meta + "}", &ResourcesReadParams{}},
		{`{"name":"prompt","inputResponses":` + valid + "," + meta + "}", &PromptsGetParams{}},
	}
	for _, frame := range validFrames {
		require.NoError(t, json.Unmarshal([]byte(frame.raw), frame.target), frame.raw)
	}

	invalid := []string{
		`{"x":null}`,
		`{"x":{}}`,
		`{"x":{"action":"approve"}}`,
		`{"x":{"action":"accept","content":{"fractional":1.5}}}`,
		`{"x":{"role":"assistant","content":{"type":"text","text":"done"}}}`,
		`{"x":{"roots":null}}`,
	}
	targets := []struct {
		prefix string
		new    func() any
	}{
		{`{"name":"tool","inputResponses":`, func() any { return &ToolsCallParams{} }},
		{`{"uri":"file:///a","inputResponses":`, func() any { return &ResourcesReadParams{} }},
		{`{"name":"prompt","inputResponses":`, func() any { return &PromptsGetParams{} }},
	}
	for _, responses := range invalid {
		for _, target := range targets {
			frame := target.prefix + responses + "," + meta + "}"
			require.Error(t, json.Unmarshal([]byte(frame), target.new()), frame)
		}
	}
}

func TestInvalidInputResponsesFailBeforeCapabilityChecks(t *testing.T) {
	client := &Client{}
	invalid := json.RawMessage(`{"x":null}`)

	_, _, toolErr := client.CallTool(t.Context(), ToolsCallParams{Name: "tool", InputResponses: invalid})
	_, _, resourceErr := client.ReadResourceRound(
		t.Context(),
		ResourcesReadParams{URI: "file:///a", InputResponses: invalid},
	)
	_, _, promptErr := client.GetPromptRound(
		t.Context(),
		PromptsGetParams{Name: "prompt", InputResponses: invalid},
	)

	for _, err := range []error{toolErr, resourceErr, promptErr} {
		var invalidPayload *InvalidPayloadError
		require.ErrorAs(t, err, &invalidPayload)
		var missingCapability *CapabilityError
		require.NotErrorAs(t, err, &missingCapability)
	}
}

func TestCanonicalBase64RejectsAlternateSpellingsAndProjectionBypass(t *testing.T) {
	for _, value := range []string{"", "Zg==", "Zm9v"} {
		resourceRaw := []byte(`{"uri":"file:///a","blob":"` + value + `"}`)
		require.NoError(t, json.Unmarshal(resourceRaw, &ResourceContents{}), value)
		contentRaw := []byte(`{"type":"image","data":"` + value + `","mimeType":"image/png"}`)
		require.NoError(t, json.Unmarshal(contentRaw, &ContentBlock{}), value)
	}

	for _, value := range []string{"Zh==", "Zg", "Zg==\n", "__8="} {
		resourceRaw := []byte(`{"uri":"file:///a","blob":` + string(mustJSONString(t, value)) + "}")
		require.Error(t, json.Unmarshal(resourceRaw, &ResourceContents{}), value)
		contentRaw := []byte(`{"type":"audio","data":` + string(mustJSONString(t, value)) + `,"mimeType":"audio/wav"}`)
		require.Error(t, json.Unmarshal(contentRaw, &ContentBlock{}), value)

		blob := value
		_, err := FormatResourceContents([]ResourceContents{{URI: "file:///a", Blob: &blob}})
		require.Error(t, err, value)
	}
}

func mustJSONString(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}
