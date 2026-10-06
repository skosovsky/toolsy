package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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

func TestResourceContents_UnknownAnnotationFieldsRemainInert(t *testing.T) {
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
			require.NoError(t, err)
			roundTrip, marshalErr := json.Marshal(resource)
			require.NoError(t, marshalErr)
			require.JSONEq(t, fixture, string(roundTrip))
			require.NotEmpty(t, resource.Extra)
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
	fixture := `{"resultType":"complete","_meta":{"trace":"x"},"extension":{"ok":true}}`
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
	valid := []string{"trace", "trace.id", "com.example/trace-id"}
	for _, key := range valid {
		raw := fmt.Sprintf(`{"%s":true}`, key)
		var meta Meta
		require.NoError(t, json.Unmarshal([]byte(raw), &meta), key)
		_, err := json.Marshal(meta)
		require.NoError(t, err, key)
	}
	invalid := []string{
		"",
		"com.example/",
		"bad prefix/value",
		"vendor?/trace",
		".vendor/trace",
		"vendor./trace",
		"-trace",
	}
	for _, key := range invalid {
		raw := fmt.Sprintf(`{"%s":true}`, key)
		var meta Meta
		require.Error(t, json.Unmarshal([]byte(raw), &meta), key)
		_, err := json.Marshal(Meta{key: json.RawMessage(`true`)})
		require.Error(t, err, key)
	}
}

func TestRequestMeta_ExtensionKeyFormatIsValidated(t *testing.T) {
	// Arrange.
	var requestMeta RequestMeta

	// Act.
	unmarshalErr := json.Unmarshal(
		[]byte(
			`{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"client","version":"test"},"bad prefix/value":true}`,
		),
		&requestMeta,
	)
	_, marshalErr := json.Marshal(RequestMeta{
		ProtocolVersion: ProtocolVersion,
		ClientInfo:      &Implementation{Name: "client", Version: "test"},
		Extra:           Meta{"bad prefix/value": json.RawMessage(`true`)},
	})

	// Assert.
	require.Error(t, unmarshalErr)
	require.Error(t, marshalErr)
	require.ErrorContains(t, unmarshalErr, `invalid _meta key "bad prefix/value"`)
	require.ErrorContains(t, marshalErr, `invalid _meta key "bad prefix/value"`)
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
  "resultType":"complete",
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
	fixture := `{"resultType":"complete","content":[{"type":"text","text":""},{"type":"image","data":"","mimeType":"image/png"}]}`
	var result CallToolResult

	// Act.
	err := json.Unmarshal([]byte(fixture), &result)
	roundTrip, marshalErr := json.Marshal(result)

	// Assert.
	require.NoError(t, err)
	require.NoError(t, marshalErr)
	require.JSONEq(t, fixture, string(roundTrip))
}

func TestResourceLinkSize_PreservesIntegersAndRejectsFraction(t *testing.T) {
	fixtures := []string{"9007199254740993", "1.5", "1e3"}
	for _, size := range fixtures {
		t.Run(size, func(t *testing.T) {
			// Arrange.
			fixture := fmt.Sprintf(
				`{"resultType":"complete","content":[{"type":"resource_link","uri":"file:///x","name":"x","size":%s}]}`,
				size,
			)
			var result CallToolResult

			// Act.
			err := json.Unmarshal([]byte(fixture), &result)
			if size == "1.5" {
				require.Error(t, err)
				_, marshalErr := json.Marshal(
					CallToolResult{
						ResultType: ResultTypeComplete,
						Content: []ContentBlock{
							{Type: "resource_link", URI: "file:///x", Name: "x", Size: new(JSONNumber(size))},
						},
					},
				)
				require.Error(t, marshalErr)
				return
			}
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
		{name: "isError null", raw: `{"resultType":"complete","content":[],"isError":null}`, into: &CallToolResult{}},
		{
			name: "resource size null",
			raw:  `{"resultType":"complete","content":[{"type":"resource_link","uri":"file:///x","name":"x","size":null}]}`,
			into: &CallToolResult{},
		},
		{
			name: "resource size non-number",
			raw:  `{"resultType":"complete","content":[{"type":"resource_link","uri":"file:///x","name":"x","size":"1"}]}`,
			into: &CallToolResult{},
		},
		{
			name: "resource link invalid icon theme",
			raw:  `{"resultType":"complete","content":[{"type":"resource_link","uri":"file:///x","name":"x","icons":[{"src":"https://example/icon.png","theme":"auto"}]}]}`,
			into: &CallToolResult{},
		},
		{
			name: "mime type null",
			raw:  `{"resultType":"complete","ttlMs":0,"cacheScope":"private","contents":[{"uri":"file:///x","mimeType":null,"text":"x"}]}`,
			into: &ResourcesReadResult{},
		},
		{
			name: "prompt required null",
			raw:  `{"resultType":"complete","ttlMs":0,"cacheScope":"private","prompts":[{"name":"p","arguments":[{"name":"a","required":null}]}]}`,
			into: &PromptsListResult{},
		},
		{
			name: "capability flag null",
			raw:  `{"resultType":"complete","supportedVersions":["2026-07-28"],"ttlMs":0,"cacheScope":"private","capabilities":{"tools":{"listChanged":null}},"_meta":{"io.modelcontextprotocol/serverInfo":{"name":"s","version":"1"}}}`,
			into: &DiscoverResult{},
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
		{name: "discover request meta null", raw: `{"_meta":null}`, into: &DiscoverParams{}},
		{
			name: "initialize request meta null",
			raw:  `{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"c","version":"1"},"_meta":null}`,
			into: &DiscoverParams{},
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

func TestRequestDTOsRoundTripTypedProgressMetaAndExtra(t *testing.T) {
	// Arrange.
	token := NewIntegerProgressToken(42)
	meta := &RequestMeta{
		ProtocolVersion: ProtocolVersion,
		ClientInfo:      &Implementation{Name: "client", Version: "test"},
		ProgressToken:   token,
		Extra:           Meta{"trace": json.RawMessage(`"abc"`)},
	}
	fixtures := []any{
		DiscoverParams{Meta: meta},
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
		require.JSONEq(
			t,
			`{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"client","version":"test"},"progressToken":42,"trace":"abc"}`,
			string(mustObjectField(t, encoded, metaObjectField)),
		)
	}
}

func TestRequestMetaRejectsNullAndResetsOnReuse(t *testing.T) {
	// Arrange.
	meta := RequestMeta{
		ProtocolVersion: ProtocolVersion,
		ClientInfo:      &Implementation{Name: "client", Version: "test"},
		ProgressToken:   NewStringProgressToken("old"),
		Extra:           Meta{"old": json.RawMessage(`true`)},
	}

	// Act.
	nullErr := json.Unmarshal([]byte(`null`), &meta)
	retainedToken := meta.ProgressToken.String()
	retainedExtra := meta.Extra["old"]
	emptyObjectErr := json.Unmarshal([]byte(`{}`), &meta)
	require.Error(t, emptyObjectErr)
	emptyErr := json.Unmarshal(
		[]byte(
			`{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"client","version":"test"}}`,
		),
		&meta,
	)

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
	base := RequestMeta{ProtocolVersion: ProtocolVersion, ClientInfo: &Implementation{Name: "client", Version: "test"}}
	_, baseErr := json.Marshal(base)
	require.NoError(t, baseErr)
	fixtures := []RequestMeta{
		{
			ProtocolVersion: ProtocolVersion,
			ClientInfo:      base.ClientInfo,
			Extra:           Meta{"progressToken": json.RawMessage(`"forged"`)},
		},
		{
			ProtocolVersion: ProtocolVersion,
			ClientInfo:      base.ClientInfo,
			ProgressToken:   NewStringProgressToken("typed"),
			Extra:           Meta{"progressToken": json.RawMessage(`null`)},
		},
	}

	for _, fixture := range fixtures {
		// Act.
		_, err := json.Marshal(fixture)

		// Assert.
		require.Error(t, err)
		require.ErrorContains(t, err, "collides with reserved key")
	}
}

func TestResourceContents_TextAndBlobAreDistinct(t *testing.T) {
	// Arrange.
	fixture := `{"resultType":"complete","ttlMs":0,"cacheScope":"private","contents":[{"uri":"file:///a","mimeType":"text/plain","text":"hello"},{"uri":"file:///b","mimeType":"application/octet-stream","blob":"AAE="}]}`
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
	// Arrange.
	fixture := `{"tools":{},"future":{"enabled":true}}`
	var capabilities ServerCapabilities

	// Act.
	err := json.Unmarshal([]byte(fixture), &capabilities)
	roundTrip, marshalErr := json.Marshal(capabilities)

	// Assert.
	require.NoError(t, err)
	require.NoError(t, marshalErr)
	require.JSONEq(t, `{"enabled":true}`, string(capabilities.Extra["future"]))
	require.JSONEq(t, fixture, string(roundTrip))
}

func TestResultDTOsPreserveUnknownTopLevelExtensions(t *testing.T) {
	fixtures := []struct {
		name   string
		raw    string
		target any
	}{
		{
			name: "discovery",
			raw: `{"resultType":"complete","supportedVersions":["2026-07-28"],"ttlMs":0,"cacheScope":"private","capabilities":{},` +
				`"_meta":{"io.modelcontextprotocol/serverInfo":{"name":"server","version":"1"}},"vendor":{"trace":1}}`,
			target: new(DiscoverResult),
		},
		{
			name:   "tools list",
			raw:    `{"resultType":"complete","ttlMs":0,"cacheScope":"private","tools":[],"vendor":{"trace":1}}`,
			target: new(ToolsListResult),
		},
		{
			name:   "tool call",
			raw:    `{"resultType":"complete","content":[],"vendor":{"trace":1}}`,
			target: new(CallToolResult),
		},
		{
			name:   "resource read",
			raw:    `{"resultType":"complete","ttlMs":0,"cacheScope":"private","contents":[],"vendor":{"trace":1}}`,
			target: new(ResourcesReadResult),
		},
		{
			name:   "prompts list",
			raw:    `{"resultType":"complete","ttlMs":0,"cacheScope":"private","prompts":[],"vendor":{"trace":1}}`,
			target: new(PromptsListResult),
		},
		{
			name:   "prompt get",
			raw:    `{"resultType":"complete","messages":[],"vendor":{"trace":1}}`,
			target: new(PromptsGetResult),
		},
		{name: "roots list", raw: `{"roots":[],"vendor":{"trace":1}}`, target: nil},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange and Act.
			if fixture.target == nil {
				transport := newFakeTransport()
				require.NoError(t, transport.Start(context.Background()))
				t.Cleanup(func() { require.NoError(t, transport.Close()) })
				pending, err := transport.PrepareRequest(
					context.Background(),
					"roots/list",
					json.RawMessage(fixture.raw),
				)
				var invalid *InvalidPayloadError
				require.ErrorAs(t, err, &invalid)
				require.ErrorContains(t, err, "legacy method")
				require.Nil(t, pending)
				require.Empty(t, transport.preparations)
				require.Empty(t, transport.requests)
				return
			}
			err := json.Unmarshal([]byte(fixture.raw), fixture.target)
			roundTrip, marshalErr := json.Marshal(fixture.target)

			// Assert.
			require.NoError(t, err)
			require.NoError(t, marshalErr)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(roundTrip, &fields))
			require.JSONEq(t, `{"trace":1}`, string(fields["vendor"]))
		})
	}
}

func TestResultDTORejectsExtensionCollisionWithReservedField(t *testing.T) {
	// Arrange.
	result := CallToolResult{
		ResultType: ResultTypeComplete,
		Content:    []ContentBlock{},
		Extra:      Meta{"content": json.RawMessage(`"shadow"`)},
	}

	// Act.
	_, err := json.Marshal(result)

	// Assert.
	require.Error(t, err)
}

func TestServerCapabilities_KnownCapabilitiesMustBeObjects(t *testing.T) {
	// Arrange.
	invalid := []string{`{"tools":null}`, `{"logging":true}`, `{"resources":[]}`}

	for _, fixture := range invalid {
		// Act.
		var capabilities ServerCapabilities
		err := json.Unmarshal([]byte(fixture), &capabilities)

		// Assert.
		require.Error(t, err)
	}
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
		Root      json.RawMessage       `json:"root"`
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
