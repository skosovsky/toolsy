package mcp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustObjectField(t *testing.T, encoded []byte, name string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	return fields[name]
}

func TestResourceContents_HasNoNonStandardAnnotationsSurface(t *testing.T) {
	// Arrange.
	resourceType := reflect.TypeFor[ResourceContents]()

	// Act.
	_, exists := resourceType.FieldByName("Annotations")

	// Assert.
	require.False(t, exists)
}

func TestResourceContents_RejectsUnknownAndLegacyAnnotationFields(t *testing.T) {
	fixtures := []string{
		`{"uri":"file:///a","text":"x","annotations":{}}`,
		`{"uri":"file:///a","text":"x","future":true}`,
	}
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			// Arrange.
			var resource ResourceContents

			// Act.
			err := json.Unmarshal([]byte(fixture), &resource)

			// Assert.
			require.Error(t, err)
		})
	}
}

func TestProgressToken_StringAndNumberWireContract(t *testing.T) {
	// Arrange.
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "string", raw: `"token"`, want: "token"},
		{name: "integer", raw: `42`, want: "42"},
		{name: "fractional", raw: `1.5`, want: "1.5"},
		{name: "integral exponent", raw: `1e0`, want: "1e0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var token ProgressToken

			// Act.
			err := json.Unmarshal([]byte(tc.raw), &token)
			encoded, marshalErr := json.Marshal(token)

			// Assert.
			require.NoError(t, err)
			require.NoError(t, marshalErr)
			require.Equal(t, tc.want, token.String())
			require.JSONEq(t, tc.raw, string(encoded))
		})
	}
}

func TestEmptyResult_RequiresObjectAndPreservesExtensions(t *testing.T) {
	for _, invalid := range []string{"null", `[]`, `42`, `"scalar"`} {
		var result EmptyResult
		require.Error(t, json.Unmarshal([]byte(invalid), &result))
	}

	// Arrange.
	fixture := `{"_meta":{"trace":"x"},"extension":{"ok":true}}`
	var result EmptyResult

	// Act.
	err := json.Unmarshal([]byte(fixture), &result)
	roundTrip, marshalErr := json.Marshal(result)

	// Assert.
	require.NoError(t, err)
	require.NoError(t, marshalErr)
	require.JSONEq(t, fixture, string(roundTrip))
}

func TestMeta_KeyFormatIsValidatedOnBothDirections(t *testing.T) {
	// Arrange.
	valid := []string{"", "trace", "trace.id", "com.example/trace-id", "com.example/"}
	invalid := []string{"bad prefix/value", "vendor?/trace", ".vendor/trace", "vendor./trace", "-trace"}

	for _, key := range valid {
		// Arrange.
		raw := fmt.Sprintf(`{"%s":true}`, key)
		var meta Meta

		// Act.
		decodeErr := json.Unmarshal([]byte(raw), &meta)
		_, encodeErr := json.Marshal(meta)

		// Assert.
		require.NoError(t, decodeErr, key)
		require.NoError(t, encodeErr, key)
	}
	for _, key := range invalid {
		// Arrange.
		raw := fmt.Sprintf(`{"%s":true}`, key)
		var meta Meta

		// Act.
		decodeErr := json.Unmarshal([]byte(raw), &meta)
		_, encodeErr := json.Marshal(Meta{key: json.RawMessage(`true`)})

		// Assert.
		require.Error(t, decodeErr, key)
		require.Error(t, encodeErr, key)
	}
}

func TestRequestMeta_ExtensionKeyFormatIsValidated(t *testing.T) {
	// Arrange.
	var requestMeta RequestMeta

	// Act.
	unmarshalErr := json.Unmarshal([]byte(`{"bad prefix/value":true}`), &requestMeta)
	_, marshalErr := json.Marshal(RequestMeta{Extra: Meta{
		"bad prefix/value": json.RawMessage(`true`),
	}})

	// Assert.
	require.Error(t, unmarshalErr)
	require.Error(t, marshalErr)
}

func TestProgressToken_RejectsNullAndCompositeValues(t *testing.T) {
	// Arrange.
	invalid := []string{`null`, `{}`, `[]`, `true`}

	for _, raw := range invalid {
		// Act.
		var token ProgressToken
		err := json.Unmarshal([]byte(raw), &token)

		// Assert.
		require.Error(t, err, raw)
	}
}

func TestCallToolResult_AllContentVariantsRemainLossless(t *testing.T) {
	// Arrange.
	fixture := `{
  "content": [
    {"type":"text","text":"hello","annotations":{"audience":["assistant"],"priority":0.7},"_meta":{"x":1}},
    {"type":"image","data":"aW1n","mimeType":"image/png"},
    {"type":"audio","data":"YXVkaW8=","mimeType":"audio/wav"},
    {"type":"resource_link","uri":"file:///a.go","name":"a.go","title":"Source","description":"code","mimeType":"text/x-go","size":12,"icons":[{"src":"https://example/icon.png","mimeType":"image/png"}]},
    {"type":"resource","resource":{"uri":"file:///b.go","mimeType":"text/x-go","text":"package b","_meta":{"trace":"ok"}}}
  ],
  "structuredContent":{"ok":true},
  "_meta":{"server":"value"}
}`
	var result CallToolResult

	// Act.
	err := json.Unmarshal([]byte(fixture), &result)
	roundTrip, marshalErr := json.Marshal(result)

	// Assert.
	require.NoError(t, err)
	require.NoError(t, marshalErr)
	require.Len(t, result.Content, 5)
	require.Equal(t, "aW1n", result.Content[1].Data)
	require.Equal(t, "image/png", result.Content[1].MIMEType)
	require.Nil(t, result.Content[1].Meta)
	require.NotNil(t, result.Content[4].Resource)
	require.JSONEq(t, fixture, string(roundTrip))
	require.NotContains(t, string(roundTrip), `"base64"`)
	require.NotContains(t, string(roundTrip), `"mediaType"`)
}

func TestContentBlock_EmptyRequiredPayloadRoundTrips(t *testing.T) {
	// Arrange.
	fixture := `{"content":[{"type":"text","text":""},{"type":"image","data":"","mimeType":"image/png"}]}`
	var result CallToolResult

	// Act.
	err := json.Unmarshal([]byte(fixture), &result)
	roundTrip, marshalErr := json.Marshal(result)

	// Assert.
	require.NoError(t, err)
	require.NoError(t, marshalErr)
	require.JSONEq(t, fixture, string(roundTrip))
}

func TestResourceLinkSize_PreservesIntegralJSONNumbers(t *testing.T) {
	fixtures := []string{"9007199254740993", "1.0", "1e3"}
	for _, size := range fixtures {
		t.Run(size, func(t *testing.T) {
			// Arrange.
			fixture := fmt.Sprintf(
				`{"content":[{"type":"resource_link","uri":"file:///x","name":"x","size":%s}]}`,
				size,
			)
			var result CallToolResult

			// Act.
			err := json.Unmarshal([]byte(fixture), &result)
			roundTrip, marshalErr := json.Marshal(result)

			// Assert.
			require.NoError(t, err)
			require.NoError(t, marshalErr)
			require.Equal(t, size, result.Content[0].Size.String())
			require.JSONEq(t, fixture, string(roundTrip))
		})
	}
}

func TestStrictDTOsRejectPresentNullAndFractionalInteger(t *testing.T) {
	// Arrange.
	fixtures := []struct {
		name string
		raw  string
		into any
	}{
		{name: "isError null", raw: `{"content":[],"isError":null}`, into: &CallToolResult{}},
		{
			name: "resource size null",
			raw:  `{"content":[{"type":"resource_link","uri":"file:///x","name":"x","size":null}]}`,
			into: &CallToolResult{},
		},
		{
			name: "resource size non-number",
			raw:  `{"content":[{"type":"resource_link","uri":"file:///x","name":"x","size":"1"}]}`,
			into: &CallToolResult{},
		},
		{
			name: "resource size fractional number",
			raw:  `{"content":[{"type":"resource_link","uri":"file:///x","name":"x","size":1.5}]}`,
			into: &CallToolResult{},
		},
		{
			name: "resource link invalid icon theme",
			raw:  `{"content":[{"type":"resource_link","uri":"file:///x","name":"x","icons":[{"src":"https://example/icon.png","theme":"auto"}]}]}`,
			into: &CallToolResult{},
		},
		{
			name: "mime type null",
			raw:  `{"contents":[{"uri":"file:///x","mimeType":null,"text":"x"}]}`,
			into: &ResourcesReadResult{},
		},
		{
			name: "prompt required null",
			raw:  `{"prompts":[{"name":"p","arguments":[{"name":"a","required":null}]}]}`,
			into: &PromptsListResult{},
		},
		{
			name: "capability flag null",
			raw:  `{"protocolVersion":"2025-11-25","capabilities":{"tools":{"listChanged":null}},"serverInfo":{"name":"s","version":"1"}}`,
			into: &InitializeResult{},
		},
		{
			name: "logging logger null",
			raw:  `{"level":"info","logger":null,"data":{}}`,
			into: &LogMessageParams{},
		},
		{
			name: "logging meta null",
			raw:  `{"level":"info","data":{},"_meta":null}`,
			into: &LogMessageParams{},
		},
		{
			name: "resource update meta null",
			raw:  `{"uri":"file:///x","_meta":null}`,
			into: &ResourceUpdatedParams{},
		},
		{name: "base request meta null", raw: `{"_meta":null}`, into: &RequestParams{}},
		{
			name: "initialize request meta null",
			raw:  `{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"c","version":"1"},"_meta":null}`,
			into: &InitializeParams{},
		},
		{name: "cursor request meta null", raw: `{"_meta":null}`, into: &CursorParams{}},
		{name: "tool call meta null", raw: `{"name":"x","_meta":null}`, into: &ToolsCallParams{}},
		{name: "resource read meta null", raw: `{"uri":"file:///x","_meta":null}`, into: &ResourcesReadParams{}},
		{name: "prompt get meta null", raw: `{"name":"x","_meta":null}`, into: &PromptsGetParams{}},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Act.
			err := json.Unmarshal([]byte(fixture.raw), fixture.into)

			// Assert.
			require.Error(t, err)
		})
	}
}

func TestResourceLinkSize_RejectsFractionalNumberOnMarshal(t *testing.T) {
	// Arrange.
	size := JSONNumber("1.5")
	result := CallToolResult{Content: []ContentBlock{{
		Type: contentTypeResourceLink,
		URI:  "file:///x",
		Name: "x",
		Size: &size,
	}}}

	// Act.
	_, err := json.Marshal(result)

	// Assert.
	require.ErrorContains(t, err, "field \"size\" must be a JSON integer")
}

func TestStrictWireDTOsRejectInvalidProgrammaticValuesOnMarshal(t *testing.T) {
	// Arrange.
	text := ""
	blob := ""
	invalidAudience := &Annotations{Audience: []string{"system"}}
	fixtures := []struct {
		name  string
		value any
	}{
		{
			name:  "content cross variant field",
			value: ContentBlock{Type: contentTypeText, Text: "ok", Data: "forbidden"},
		},
		{
			name: "content invalid nested annotations",
			value: ContentBlock{
				Type: contentTypeText, Text: "ok", Annotations: invalidAudience,
			},
		},
		{
			name: "resource link invalid nested icon",
			value: ContentBlock{
				Type: contentTypeResourceLink, URI: "file:///x", Name: "x",
				Icons: []Icon{{Src: "https://example/icon.png", Theme: "auto"}},
			},
		},
		{name: "resource missing content variant", value: ResourceContents{URI: "file:///x"}},
		{
			name:  "resource has both content variants",
			value: ResourceContents{URI: "file:///x", Text: &text, Blob: &blob},
		},
		{name: "call result null content", value: CallToolResult{}},
		{name: "resources result null contents", value: ResourcesReadResult{}},
		{name: "tools result null tools", value: ToolsListResult{}},
		{name: "prompts result null prompts", value: PromptsListResult{}},
		{name: "prompt get result null messages", value: PromptsGetResult{}},
		{name: "roots result null roots", value: RootsListResult{}},
		{name: "progress missing token", value: ProgressParams{}},
		{name: "tool call missing name", value: ToolsCallParams{}},
		{name: "resource read missing uri", value: ResourcesReadParams{}},
		{name: "tool missing input schema", value: MCPTool{Name: "tool"}},
		{
			name:  "prompt message invalid role",
			value: PromptMessage{Role: "system", Content: ContentBlock{Type: contentTypeText, Text: "ok"}},
		},
		{name: "root missing uri", value: Root{}},
		{name: "cancelled missing request id", value: CancelledParams{}},
		{name: "request missing envelope fields", value: Request{}},
		{name: "notification missing envelope fields", value: Notification{}},
		{name: "response missing result and error", value: Response{JSONRPC: JSONRPCVersion, ID: json.RawMessage(`1`)}},
		{name: "json rpc error missing integer code", value: JSONRPCError{Message: "bad"}},
		{
			name:  "server capability must be object",
			value: ServerCapabilities{Logging: json.RawMessage(`[]`)},
		},
		{name: "tool execution unknown task support", value: ToolExecution{TaskSupport: "future"}},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Act.
			_, err := json.Marshal(fixture.value)

			// Assert.
			require.Error(t, err)
		})
	}
}

func TestStrictWireDTOsMarshalSchemaValidEmptyAndZeroValues(t *testing.T) {
	// Arrange.
	empty := ""
	fixtures := []any{
		ContentBlock{Type: contentTypeText, Text: ""},
		ContentBlock{Type: contentTypeImage, Data: "", MIMEType: "image/png"},
		ResourceContents{URI: "file:///empty", Text: &empty},
		CallToolResult{Content: []ContentBlock{}},
		ResourcesReadResult{Contents: []ResourceContents{}},
		ToolsListResult{Tools: []MCPTool{}},
		PromptsListResult{Prompts: []Prompt{}},
		PromptsGetResult{Messages: []PromptMessage{}},
		RootsListResult{Roots: []Root{}},
		ProgressParams{ProgressToken: NewStringProgressToken("token"), Progress: 0},
		ProgressParams{ProgressToken: NewStringProgressToken("negative"), Progress: -2, Total: new(-1.0)},
		InitializeParams{ProtocolVersion: "", Capabilities: ClientCapabilities{}, ClientInfo: Implementation{}},
		PromptsGetParams{Name: ""},
		Prompt{Name: ""},
		PromptArgument{Name: ""},
		Implementation{Name: "", Version: ""},
		ContentBlock{Type: contentTypeResourceLink, URI: "file:///empty-name", Name: ""},
		ContentBlock{
			Type: contentTypeText, Text: "x", Annotations: &Annotations{LastModified: "arbitrary"},
		},
		ToolExecution{},
		Request{
			JSONRPC: JSONRPCVersion, ID: json.RawMessage(`1`), Method: MethodPing,
			Params: json.RawMessage(`{}`),
		},
		Notification{JSONRPC: JSONRPCVersion, Method: MethodInitialized, Params: json.RawMessage(`{}`)},
		Response{JSONRPC: JSONRPCVersion, ID: json.RawMessage(`1`), Result: json.RawMessage(`{}`)},
		Response{
			JSONRPC: JSONRPCVersion,
			ID:      json.RawMessage(jsonNull),
			Error:   &JSONRPCError{Code: JSONRPCInvalidRequest, Message: "Invalid Request"},
		},
	}

	for _, fixture := range fixtures {
		// Act.
		encoded, err := json.Marshal(fixture)

		// Assert.
		require.NoError(t, err)
		require.True(t, json.Valid(encoded))
	}
}

func TestJSONRPCEnvelopeDTOsRejectCrossVariantFields(t *testing.T) {
	// Arrange.
	fixtures := []struct {
		name string
		raw  string
		into any
	}{
		{
			name: "request with result",
			raw:  `{"jsonrpc":"2.0","id":1,"method":"x","result":{}}`,
			into: &Request{},
		},
		{
			name: "notification with id",
			raw:  `{"jsonrpc":"2.0","id":1,"method":"x"}`,
			into: &Notification{},
		},
		{
			name: "response with method",
			raw:  `{"jsonrpc":"2.0","id":1,"method":"x","result":{}}`,
			into: &Response{},
		},
		{
			name: "response with params",
			raw:  `{"jsonrpc":"2.0","id":1,"result":{},"params":{}}`,
			into: &Response{},
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Act.
			err := json.Unmarshal([]byte(fixture.raw), fixture.into)

			// Assert.
			require.Error(t, err)
		})
	}
}

func TestServerCapabilitiesExtraRejectsReservedCollisions(t *testing.T) {
	reserved := []string{"tools", "resources", "prompts", "logging", "completions", "tasks", "experimental"}
	for _, name := range reserved {
		t.Run(name, func(t *testing.T) {
			// Arrange.
			capabilities := ServerCapabilities{Extra: map[string]json.RawMessage{
				name: json.RawMessage(`{}`),
			}}

			// Act.
			_, err := json.Marshal(capabilities)

			// Assert.
			require.ErrorContains(t, err, "collides with a reserved field")
		})
	}

	// Arrange.
	capabilities := ServerCapabilities{Extra: map[string]json.RawMessage{
		"vendor.example/feature": json.RawMessage(`{"enabled":true}`),
	}}

	// Act.
	encoded, err := json.Marshal(capabilities)

	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, `{"vendor.example/feature":{"enabled":true}}`, string(encoded))
}

func TestWireDTOScalarAndBinaryConstraintsApplyOnDecodeAndEncode(t *testing.T) {
	// Arrange.
	invalid := []struct {
		name  string
		raw   string
		into  any
		value any
	}{
		{
			name: "logging level enum", raw: `{"level":"trace","data":null}`,
			into: &LogMessageParams{}, value: LogMessageParams{Level: "trace", Data: json.RawMessage(jsonNull)},
		},
		{
			name: "root file scheme", raw: `{"uri":"https://example.test/root"}`,
			into: &Root{}, value: Root{URI: "https://example.test/root"},
		},
		{
			name: "tool name grammar", raw: `{"name":"has space","inputSchema":{"type":"object"}}`,
			into: &MCPTool{}, value: MCPTool{Name: "has space", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		{
			name: "tool call name grammar", raw: `{"name":"has space"}`,
			into: &ToolsCallParams{}, value: ToolsCallParams{Name: "has space"},
		},
		{
			name: "input schema root type", raw: `{"name":"tool","inputSchema":{}}`,
			into: &MCPTool{}, value: MCPTool{Name: "tool", InputSchema: json.RawMessage(`{}`)},
		},
		{
			name: "input schema required type",
			raw:  `{"name":"tool","inputSchema":{"type":"object","required":null}}`,
			into: &MCPTool{},
			value: MCPTool{Name: "tool", InputSchema: json.RawMessage(
				`{"type":"object","required":null}`,
			)},
		},
		{
			name: "input schema properties type",
			raw:  `{"name":"tool","inputSchema":{"type":"object","properties":[]}}`,
			into: &MCPTool{},
			value: MCPTool{Name: "tool", InputSchema: json.RawMessage(
				`{"type":"object","properties":[]}`,
			)},
		},
		{
			name: "input schema dialect type",
			raw:  `{"name":"tool","inputSchema":{"$schema":42,"type":"object"}}`,
			into: &MCPTool{},
			value: MCPTool{Name: "tool", InputSchema: json.RawMessage(
				`{"$schema":42,"type":"object"}`,
			)},
		},
		{
			name: "output schema root type",
			raw:  `{"name":"tool","inputSchema":{"type":"object"},"outputSchema":{"type":"string"}}`,
			into: &MCPTool{},
			value: MCPTool{Name: "tool", InputSchema: json.RawMessage(`{"type":"object"}`),
				OutputSchema: json.RawMessage(`{"type":"string"}`)},
		},
		{
			name: "image base64", raw: `{"type":"image","data":"%%%","mimeType":"image/png"}`,
			into: &ContentBlock{}, value: ContentBlock{Type: contentTypeImage, Data: "%%%", MIMEType: "image/png"},
		},
		{
			name: "audio base64", raw: `{"type":"audio","data":"%%%","mimeType":"audio/mpeg"}`,
			into: &ContentBlock{}, value: ContentBlock{Type: contentTypeAudio, Data: "%%%", MIMEType: "audio/mpeg"},
		},
		{
			name: "resource blob base64", raw: `{"uri":"file:///x","blob":"%%%"}`,
			into: &ResourceContents{}, value: ResourceContents{URI: "file:///x", Blob: new("%%%-invalid")},
		},
		{
			name: "icon absolute uri", raw: `{"src":"relative/icon.png"}`,
			into: &Icon{}, value: Icon{Src: "relative/icon.png"},
		},
		{
			name: "implementation website uri",
			raw:  `{"name":"","version":"","websiteUrl":"relative"}`,
			into: &Implementation{}, value: Implementation{WebsiteURL: "relative"},
		},
		{
			name: "resource link uri", raw: `{"type":"resource_link","uri":"relative","name":""}`,
			into: &ContentBlock{}, value: ContentBlock{Type: contentTypeResourceLink, URI: "relative"},
		},
		{
			name: "resource contents uri", raw: `{"uri":"relative","text":""}`,
			into: &ResourceContents{}, value: ResourceContents{URI: "relative", Text: new("")},
		},
	}

	for _, fixture := range invalid {
		t.Run(fixture.name, func(t *testing.T) {
			// Act.
			decodeErr := json.Unmarshal([]byte(fixture.raw), fixture.into)
			_, encodeErr := json.Marshal(fixture.value)

			// Assert.
			require.Error(t, decodeErr)
			require.Error(t, encodeErr)
		})
	}

	// Arrange.
	validEmptyStrings := []string{
		`{"type":"image","data":"","mimeType":""}`,
		`{"type":"resource_link","uri":"file:///x","name":""}`,
		`{"jsonrpc":"2.0","id":1,"method":"","params":{}}`,
		`{"jsonrpc":"2.0","method":"","params":{}}`,
	}

	for _, raw := range validEmptyStrings {
		var target any
		switch {
		case strings.Contains(raw, `"type"`):
			target = &ContentBlock{}
		case strings.Contains(raw, `"id"`):
			target = &Request{}
		default:
			target = &Notification{}
		}

		// Act.
		err := json.Unmarshal([]byte(raw), target)

		// Assert.
		require.NoError(t, err)
	}

	for _, raw := range []string{`{"src":"https://example.test/i.png","theme":""}`, `{"taskSupport":""}`} {
		var target any
		if strings.Contains(raw, "theme") {
			target = &Icon{}
		} else {
			target = &ToolExecution{}
		}
		require.Error(t, json.Unmarshal([]byte(raw), target))
	}
}

func TestNestedWireDTOValidationCannotBeBypassed(t *testing.T) {
	// Arrange.
	fixtures := []any{
		ToolsListResult{Tools: []MCPTool{{
			Name: "has space", InputSchema: json.RawMessage(`{"type":"object"}`),
		}}},
		ToolsListResult{Tools: []MCPTool{{
			Name: "valid", InputSchema: json.RawMessage(`{"type":"object","required":null}`),
		}}},
		CallToolResult{Content: []ContentBlock{{
			Type: contentTypeImage, Data: "%%%", MIMEType: "image/png",
		}}},
		RootsListResult{Roots: []Root{{URI: "https://example.test/not-a-root"}}},
		PromptsGetResult{Messages: []PromptMessage{{
			Role: audienceAssistant,
			Content: ContentBlock{
				Type: contentTypeAudio, Data: "%%%", MIMEType: "audio/mpeg",
			},
		}}},
	}

	for _, fixture := range fixtures {
		// Act.
		_, err := json.Marshal(fixture)

		// Assert.
		require.Error(t, err, fixture)
	}
}

func TestIconURISecurityPolicyAppliesDirectAndNested(t *testing.T) {
	// Arrange.
	unsafe := []string{
		"javascript:alert(1)",
		"file:///etc/passwd",
		"ftp://example.test/icon.png",
		"ws://example.test/icon",
		"http://example.test/icon.png",
		"https://:443/icon.png",
		"https://example.test:/icon.png",
		"https://example.test:65536/icon.png",
		"https://example.test/icon image.png",
		"https://example.test/a[b]",
		"https://example.test/i#x#y",
		"https://1:2:3:4:5:6:7:8/icon.png",
		"https://[v1.%66]/icon.png",
		"https://[fe80::1%25]/icon.png",
		"https://[fe80::1%25eth!0]/icon.png",
		"data://evil",
		"data:image/png;base64",
		"data:text/plain,not-an-icon",
		"data:image/png;base64,%%%",
		"data:image/png;base64,AA==?junk",
		"data:image/svg+xml,<svg x>",
		"data:image/png,[x]",
		"data:image/png%3Bfoo,AA",
		"data:image/png;na%3Bme=x,AA",
		"data:image/png;name=a%00b,AA",
		"data:image/png;name=%C2%85,AA",
		"data:image/png;name=%FF,AA",
	}

	for _, src := range unsafe {
		t.Run(src, func(t *testing.T) {
			// Act.
			var decoded Icon
			decodeErr := json.Unmarshal(fmt.Appendf(nil, `{"src":%q}`, src), &decoded)
			_, encodeErr := json.Marshal(Icon{Src: src})
			var nestedDecoded Implementation
			nestedDecodeErr := json.Unmarshal(fmt.Appendf(nil,
				`{"name":"server","version":"1","icons":[{"src":%q}]}`, src), &nestedDecoded)
			_, nestedEncodeErr := json.Marshal(Implementation{
				Name: "server", Version: "1", Icons: []Icon{{Src: src}},
			})

			// Assert.
			require.Error(t, decodeErr)
			require.Error(t, encodeErr)
			require.Error(t, nestedDecodeErr)
			require.Error(t, nestedEncodeErr)
		})
	}

	for _, fixture := range []any{
		Implementation{Name: "", Version: "", Icons: []Icon{{Src: "file:///etc/passwd"}}},
		MCPTool{
			Name: "tool", InputSchema: json.RawMessage(`{"type":"object"}`),
			Icons: []Icon{{Src: "javascript:alert(1)"}},
		},
		Prompt{Name: "", Icons: []Icon{{Src: "http://example.test/icon.png"}}},
		ContentBlock{
			Type: contentTypeResourceLink, URI: "file:///resource", Name: "",
			Icons: []Icon{{Src: "ftp://example.test/icon.png"}},
		},
	} {
		_, err := json.Marshal(fixture)
		require.Error(t, err)
	}

	for _, src := range []string{
		"https://example.test/icon.png",
		"https://user:password@example.test:65535/icon.png",
		"https://[2001:db8::1]:443/icon.png",
		"https://[v1.fe]/icon.png",
		"https://[fe80::1%25eth0]/icon.png",
		"https://example.test/a%5Bb%5D",
		"data:image/png;base64,AA==",
		"data:image/svg+xml,%3Csvg%2F%3E",
		"data:image/png,%5Bx%5D",
		"data:image/png;name=a%20b,AA",
		"data:image/png;name=a%3Bb,AA",
	} {
		encoded, err := json.Marshal(Icon{Src: src})
		require.NoError(t, err)
		require.True(t, json.Valid(encoded))
		nested, nestedErr := json.Marshal(Implementation{
			Name: "server", Version: "1", Icons: []Icon{{Src: src}},
		})
		require.NoError(t, nestedErr)
		var decoded Implementation
		require.NoError(t, json.Unmarshal(nested, &decoded))
	}
}

func TestIconSizesRequireStringItemsDirectAndNested(t *testing.T) {
	// Arrange.
	invalid := []struct {
		raw  string
		into any
	}{
		{raw: `{"src":"https://example.test/icon.png","sizes":[null]}`, into: &Icon{}},
		{
			raw:  `{"name":"server","version":"1","icons":[{"src":"https://example.test/icon.png","sizes":[null]}]}`,
			into: &Implementation{},
		},
	}
	decodeErrors := make([]error, 0, len(invalid))

	// Act.
	for _, fixture := range invalid {
		decodeErrors = append(decodeErrors, json.Unmarshal([]byte(fixture.raw), fixture.into))
	}
	encoded, encodeErr := json.Marshal(Icon{
		Src: "https://example.test/icon.png", Sizes: []string{"", "16x16", "any"},
	})

	// Assert.
	for _, decodeErr := range decodeErrors {
		require.Error(t, decodeErr)
	}
	require.NoError(t, encodeErr)
	require.JSONEq(t, `{"src":"https://example.test/icon.png","sizes":["","16x16","any"]}`, string(encoded))
}

func TestIconHTTPSAuthorityValidationDoesNotDependOnURLStrictColons(t *testing.T) {
	// Arrange.
	t.Setenv("GODEBUG", "urlstrictcolons=0")
	src := "HTTPS://1:2:3:4:5:6:7:8/icon.png"

	// Act.
	var direct Icon
	directDecodeErr := json.Unmarshal(fmt.Appendf(nil, `{"src":%q}`, src), &direct)
	_, directEncodeErr := json.Marshal(Icon{Src: src})
	var nested Implementation
	nestedDecodeErr := json.Unmarshal(fmt.Appendf(nil,
		`{"name":"server","version":"1","icons":[{"src":%q}]}`, src), &nested)
	_, nestedEncodeErr := json.Marshal(Implementation{
		Name: "server", Version: "1", Icons: []Icon{{Src: src}},
	})

	// Assert.
	require.Error(t, directDecodeErr)
	require.Error(t, directEncodeErr)
	require.Error(t, nestedDecodeErr)
	require.Error(t, nestedEncodeErr)
}

func TestServerCapabilitiesRejectsNullDirectAndNested(t *testing.T) {
	// Arrange.
	fixtures := []struct {
		raw  string
		into any
	}{
		{raw: jsonNull, into: &ServerCapabilities{}},
		{
			raw: `{"protocolVersion":"2025-11-25","capabilities":null,` +
				`"serverInfo":{"name":"","version":""}}`,
			into: &InitializeResult{},
		},
	}

	for _, fixture := range fixtures {
		// Act.
		err := json.Unmarshal([]byte(fixture.raw), fixture.into)

		// Assert.
		require.Error(t, err)
	}

	encoded, err := json.Marshal(ServerCapabilities{})
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(encoded))
}

func TestRequestDTOsRoundTripTypedProgressMetaAndExtra(t *testing.T) {
	// Arrange.
	token := NewIntegerProgressToken(42)
	meta := &RequestMeta{
		ProgressToken: token,
		Extra:         Meta{"trace": json.RawMessage(`"abc"`)},
	}
	fixtures := []any{
		InitializeParams{
			ProtocolVersion: ProtocolVersion,
			Capabilities:    ClientCapabilities{},
			ClientInfo:      Implementation{Name: "client", Version: "1"},
			Meta:            meta,
		},
		CursorParams{Meta: meta},
		ToolsCallParams{Name: "tool", Meta: meta},
		ResourcesReadParams{URI: "file:///x", Meta: meta},
		PromptsGetParams{Name: "prompt", Meta: meta},
	}

	for _, fixture := range fixtures {
		// Act.
		encoded, err := json.Marshal(fixture)

		// Assert.
		require.NoError(t, err)
		require.JSONEq(t, `{"progressToken":42,"trace":"abc"}`, string(mustObjectField(t, encoded, metaField)))
	}
}

func TestRequestMetaRejectsNullAndResetsOnReuse(t *testing.T) {
	// Arrange.
	meta := RequestMeta{
		ProgressToken: NewStringProgressToken("old"),
		Extra:         Meta{"old": json.RawMessage(`true`)},
	}

	// Act.
	nullErr := json.Unmarshal([]byte(`null`), &meta)
	retainedToken := meta.ProgressToken.String()
	retainedExtra := meta.Extra["old"]
	emptyErr := json.Unmarshal([]byte(`{}`), &meta)

	// Assert.
	require.Error(t, nullErr)
	require.Equal(t, "old", retainedToken)
	require.JSONEq(t, `true`, string(retainedExtra))
	require.NoError(t, emptyErr)
	require.True(t, meta.ProgressToken.IsZero())
	require.Nil(t, meta.Extra)
}

func TestRequestMetaRejectsReservedExtraCollision(t *testing.T) {
	// Arrange.
	fixtures := []RequestMeta{
		{Extra: Meta{"progressToken": json.RawMessage(`"forged"`)}},
		{
			ProgressToken: NewStringProgressToken("typed"),
			Extra:         Meta{"progressToken": json.RawMessage(`null`)},
		},
	}

	for _, fixture := range fixtures {
		// Act.
		_, err := json.Marshal(fixture)

		// Assert.
		require.Error(t, err)
	}
}

func TestResourceContents_TextAndBlobAreDistinct(t *testing.T) {
	// Arrange.
	fixture := `{"contents":[{"uri":"file:///a","mimeType":"text/plain","text":"hello"},{"uri":"file:///b","mimeType":"application/octet-stream","blob":"AAE="}]}`
	var result ResourcesReadResult

	// Act.
	err := json.Unmarshal([]byte(fixture), &result)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, result.Contents[0].Text)
	require.Nil(t, result.Contents[0].Blob)
	require.Nil(t, result.Contents[1].Text)
	require.NotNil(t, result.Contents[1].Blob)
}

func TestServerCapabilities_PreservesUnknownAdditiveCapabilities(t *testing.T) {
	fixtures := []string{
		`{"tools":{},"future":{"enabled":true}}`,
		`{"future":true}`,
		`{"future":null}`,
		`{"future":[1,"two"]}`,
	}
	for _, fixture := range fixtures {
		// Arrange.
		var capabilities ServerCapabilities

		// Act.
		err := json.Unmarshal([]byte(fixture), &capabilities)
		roundTrip, marshalErr := json.Marshal(capabilities)

		// Assert.
		require.NoError(t, err)
		require.NoError(t, marshalErr)
		require.JSONEq(t, fixture, string(roundTrip))
	}
}

func TestResultDTOsPreserveUnknownTopLevelExtensions(t *testing.T) {
	// Arrange.
	fixtures := []struct {
		name   string
		raw    string
		target any
	}{
		{
			name: "initialize",
			raw: `{"protocolVersion":"2025-11-25","capabilities":{},` +
				`"serverInfo":{"name":"server","version":"1"},"vendor":{"trace":1}}`,
			target: new(InitializeResult),
		},
		{name: "tools list", raw: `{"tools":[],"vendor":{"trace":1}}`, target: new(ToolsListResult)},
		{name: "tool call", raw: `{"content":[],"vendor":{"trace":1}}`, target: new(CallToolResult)},
		{name: "resource read", raw: `{"contents":[],"vendor":{"trace":1}}`, target: new(ResourcesReadResult)},
		{name: "prompts list", raw: `{"prompts":[],"vendor":{"trace":1}}`, target: new(PromptsListResult)},
		{name: "prompt get", raw: `{"messages":[],"vendor":{"trace":1}}`, target: new(PromptsGetResult)},
		{name: "roots list", raw: `{"roots":[],"vendor":{"trace":1}}`, target: new(RootsListResult)},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			raw := []byte(fixture.raw)
			target := fixture.target

			// Act.
			decodeErr := json.Unmarshal(raw, target)
			roundTrip, marshalErr := json.Marshal(target)
			var fields map[string]json.RawMessage
			fieldsErr := json.Unmarshal(roundTrip, &fields)

			// Assert.
			require.NoError(t, decodeErr)
			require.NoError(t, marshalErr)
			require.NoError(t, fieldsErr)
			require.JSONEq(t, `{"trace":1}`, string(fields["vendor"]))
		})
	}
}

func TestResultDTORejectsExtensionCollisionWithReservedField(t *testing.T) {
	// Arrange.
	result := CallToolResult{
		Content: []ContentBlock{},
		Extra:   Meta{"content": json.RawMessage(`"shadow"`)},
	}

	// Act.
	_, err := json.Marshal(result)

	// Assert.
	require.Error(t, err)
}

func TestServerCapabilities_KnownCapabilitiesMustBeObjects(t *testing.T) {
	// Arrange.
	invalid := []string{
		`{"tools":null}`,
		`{"logging":true}`,
		`{"resources":[]}`,
		`{"experimental":{"vendor":null}}`,
		`{"experimental":{"vendor":true}}`,
		`{"tasks":{"list":null}}`,
		`{"tasks":{"cancel":false}}`,
		`{"tasks":{"requests":{"tools":{"call":null}}}}`,
	}

	for _, fixture := range invalid {
		// Act.
		var capabilities ServerCapabilities
		err := json.Unmarshal([]byte(fixture), &capabilities)

		// Assert.
		require.Error(t, err)
		var encodedValue ServerCapabilities
		require.NoError(t, json.Unmarshal([]byte(`{}`), &encodedValue))
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(fixture), &fields))
		for name, raw := range fields {
			switch name {
			case capabilityLoggingField:
				encodedValue.Logging = raw
			case capabilityTasksField:
				encodedValue.Tasks = raw
			case capabilityExperimentField:
				encodedValue.Experimental = raw
			}
		}
		if encodedValue.Logging != nil || encodedValue.Tasks != nil || encodedValue.Experimental != nil {
			_, marshalErr := json.Marshal(encodedValue)
			require.Error(t, marshalErr, fixture)
		}
	}
}

func TestServerCapabilities_AcceptsSchemaShapedKnownCapabilities(t *testing.T) {
	// Arrange.
	fixture := `{"logging":{},"completions":{"vendor":true},` +
		`"experimental":{"vendor":{"enabled":true}},` +
		`"tasks":{"list":{},"cancel":{"vendor":1},` +
		`"requests":{"tools":{"call":{"vendor":true}}}}}`
	var capabilities ServerCapabilities

	// Act.
	err := json.Unmarshal([]byte(fixture), &capabilities)
	roundTrip, marshalErr := json.Marshal(capabilities)

	// Assert.
	require.NoError(t, err)
	require.NoError(t, marshalErr)
	require.JSONEq(t, fixture, string(roundTrip))
}

func TestPrimitiveMetadata_RoundTripsOnCurrentWireDTOs(t *testing.T) {
	// Arrange.
	fixture := `{
  "icon":{"src":"https://example/icon.svg","theme":"dark"},
  "root":{"uri":"file:///workspace","_meta":{"root":true}},
  "progress":{"progressToken":7,"progress":0.5,"_meta":{"trace":"p"}},
  "cancelled":{"requestId":"x","_meta":{"trace":"c"}},
  "updated":{"uri":"file:///workspace/a","_meta":{"trace":"u"}}
}`
	var decoded struct {
		Icon      Icon                  `json:"icon"`
		Root      Root                  `json:"root"`
		Progress  ProgressParams        `json:"progress"`
		Cancelled CancelledParams       `json:"cancelled"`
		Updated   ResourceUpdatedParams `json:"updated"`
	}

	// Act.
	err := json.Unmarshal([]byte(fixture), &decoded)
	roundTrip, marshalErr := json.Marshal(decoded)

	// Assert.
	require.NoError(t, err)
	require.NoError(t, marshalErr)
	require.Equal(t, "dark", decoded.Icon.Theme)
	require.JSONEq(t, fixture, string(roundTrip))
}
