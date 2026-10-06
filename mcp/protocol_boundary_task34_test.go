package mcp

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolBoundaryRejectsRecursiveDuplicateKeys(t *testing.T) {
	// Arrange.
	peer := &rpcPeer{}
	raw := []byte(`{"jsonrpc":"2.0","id":1,"result":{"nested":{"key":1,"key":2}}}`)

	// Act.
	_, err := peer.decodeRPCObject(raw)

	// Assert.
	require.ErrorContains(t, err, "duplicate")
	require.ErrorIs(t, err, ErrProtocolViolation)
}

func TestCacheInfoPreservesUnboundedIntegerAndRejectsInvalidForms(t *testing.T) {
	// Arrange.
	const huge = "123456789012345678901234567890"
	valid := []byte(`{"resultType":"complete","tools":[],"ttlMs":` + huge + `,"cacheScope":"private"}`)

	// Act.
	var result ToolsListResult
	err := json.Unmarshal(valid, &result)
	encoded, marshalErr := json.Marshal(result)

	// Assert.
	require.NoError(t, err)
	require.NoError(t, marshalErr)
	require.Equal(t, JSONNumber(huge), result.TTLMS)
	require.Contains(t, string(encoded), `"ttlMs":`+huge)

	for _, raw := range []string{
		`{"resultType":"complete","tools":[],"ttlMs":null,"cacheScope":"private"}`,
		`{"resultType":"complete","tools":[],"ttlMs":1.5,"cacheScope":"private"}`,
		`{"resultType":"complete","tools":[],"ttlMs":-1,"cacheScope":"private"}`,
		`{"resultType":"complete","tools":[],"ttlMs":0,"cacheScope":null}`,
		`{"resultType":"complete","tools":[],"ttlMs":0,"cacheScope":"shared"}`,
	} {
		var invalid ToolsListResult
		require.Error(t, json.Unmarshal([]byte(raw), &invalid), raw)
	}
}

func TestMRTRRequestStateIsByteExactOpaqueString(t *testing.T) {
	// Arrange.
	raw := []byte(`{"resultType":"input_required","requestState":"opaque\u0020state"}`)

	// Act.
	var result InputRequiredResult
	err := json.Unmarshal(raw, &result)

	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, `"opaque\u0020state"`, string(result.RequestState))

	for _, state := range []string{"null", "1", "{}", "[]"} {
		var invalid InputRequiredResult
		require.Error(t, json.Unmarshal([]byte(`{"resultType":"input_required","requestState":`+state+`}`), &invalid))
	}
}

func TestMalformedServerInfoIsDisplayMetadataAndTreatedAbsent(t *testing.T) {
	// Arrange.
	raw := []byte(`{"io.modelcontextprotocol/serverInfo":{"name":1,"version":null},"vendor.trace":{"id":7}}`)

	// Act.
	var meta ResultMeta
	err := json.Unmarshal(raw, &meta)

	// Assert.
	require.NoError(t, err)
	require.Nil(t, meta.ServerInfo)
	require.JSONEq(t, `{"id":7}`, string(meta.Extra["vendor.trace"]))
}

func TestContentBlockTaggedUnionIsSymmetric(t *testing.T) {
	// Arrange.
	valid := ContentBlock{Type: "text", Text: ""}
	foreign := ContentBlock{Type: "text", Text: "ok", Data: "Zm9v"}

	// Act.
	encoded, err := json.Marshal(valid)
	_, foreignErr := json.Marshal(foreign)
	var decoded ContentBlock
	decodeErr := json.Unmarshal([]byte(`{"type":"text","text":"ok","data":"Zm9v"}`), &decoded)

	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"text","text":""}`, string(encoded))
	require.ErrorContains(t, foreignErr, "foreign field")
	require.ErrorContains(t, decodeErr, "foreign field")
}

func TestReservedRPCErrorDataViolationsAreClassified(t *testing.T) {
	// Arrange.
	rpcErr := &RPCError{
		Code:    JSONRPCUnsupportedProtocolVersion,
		Message: "unsupported",
		Data:    json.RawMessage(`{"requested":"x","requested":"y","supported":["2026-07-28"]}`),
	}

	// Act.
	err := TypedRPCError(rpcErr)

	// Assert.
	require.ErrorIs(t, err, ErrProtocolViolation)
	var payloadErr *InvalidPayloadError
	require.ErrorAs(t, err, &payloadErr)
}

// The complete capability frame makes lossless round-trip intent explicit.
func TestCurrentInertCapabilitiesAndUnrelatedExtrasRemainLossless(t *testing.T) {
	// Arrange.
	raw := []byte(
		`{"tools":{},"logging":{"level":"debug"},"completions":{},"experimental":{"vendor":{}},"vendor.example":{"x":1}}`,
	)

	// Act.
	var capabilities ServerCapabilities
	err := json.Unmarshal(raw, &capabilities)
	encoded, marshalErr := json.Marshal(capabilities)

	// Assert.
	require.NoError(t, err)
	require.NoError(t, marshalErr)
	require.Contains(t, capabilities.Extra, "vendor.example")
	require.Contains(t, capabilities.Extra, "logging")
	require.Contains(t, capabilities.Extra, "completions")
	require.Contains(t, capabilities.Extra, "experimental")
	require.JSONEq(t, string(raw), string(encoded))
}

type boundaryExtensionCodec struct{}

func (boundaryExtensionCodec) DecodeExtension(raw json.RawMessage) (any, error) {
	return string(raw), nil
}
func (boundaryExtensionCodec) EncodeExtension(value any) (json.RawMessage, error) {
	return json.RawMessage(value.(string)), nil
}

func TestExtensionRegistryRequiresExplicitOwnership(t *testing.T) {
	// Arrange.
	var registry ExtensionRegistry
	const identifier = "vendor.example/feature"

	// Act.
	registerErr := registry.Register(identifier, boundaryExtensionCodec{})
	decoded, decodeErr := registry.Decode(identifier, json.RawMessage(`{"enabled":true}`))
	_, unknownErr := registry.Decode("vendor.example/unknown", json.RawMessage(`{}`))
	_, duplicateErr := registry.Encode(identifier, `{"x":1,"x":2}`)

	// Assert.
	require.NoError(t, registerErr)
	require.NoError(t, decodeErr)
	require.Equal(t, `{"enabled":true}`, decoded)
	var unsupported *UnsupportedFeatureError
	require.ErrorAs(t, unknownErr, &unsupported)
	require.ErrorIs(t, duplicateErr, ErrProtocolViolation)
	require.NoError(t, registry.Register("io.modelcontextprotocol/example", boundaryExtensionCodec{}))
}

// Full adversarial JSON frames are kept inline so variant contamination stays visible.
func TestResultUnionRejectsForeignVariantFieldsAcrossMethods(t *testing.T) {
	// Arrange.
	completeFixtures := []struct {
		name, raw string
		target    any
	}{
		{
			"discover",
			`{"resultType":"complete","supportedVersions":["2026-07-28"],"capabilities":{},"ttlMs":0,"cacheScope":"private","requestState":"x"}`,
			&DiscoverResult{},
		},
		{
			"tools list",
			`{"resultType":"complete","tools":[],"ttlMs":0,"cacheScope":"private","inputRequests":{}}`,
			&ToolsListResult{},
		},
		{
			"resources list",
			`{"resultType":"complete","resources":[],"ttlMs":0,"cacheScope":"private","requestState":"x"}`,
			&ResourcesListResult{},
		},
		{
			"templates list",
			`{"resultType":"complete","resourceTemplates":[],"ttlMs":0,"cacheScope":"private","inputRequests":{}}`,
			&ResourceTemplatesListResult{},
		},
		{
			"prompts list",
			`{"resultType":"complete","prompts":[],"ttlMs":0,"cacheScope":"private","requestState":"x"}`,
			&PromptsListResult{},
		},
		{
			"resource read",
			`{"resultType":"complete","contents":[],"ttlMs":0,"cacheScope":"private","inputRequests":{}}`,
			&ResourcesReadResult{},
		},
		{"prompt get", `{"resultType":"complete","messages":[],"requestState":"x"}`, &PromptsGetResult{}},
		{"tool call", `{"resultType":"complete","content":[],"inputRequests":{}}`, &CallToolResult{}},
		{"empty", `{"resultType":"complete","requestState":"x"}`, &CompleteResult{}},
		{
			"subscription",
			`{"resultType":"complete","_meta":{"io.modelcontextprotocol/subscriptionId":1},"inputRequests":{}}`,
			&SubscriptionsListenResult{},
		},
	}

	for _, fixture := range completeFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Act.
			err := json.Unmarshal([]byte(fixture.raw), fixture.target)

			// Assert.
			require.ErrorContains(t, err, "different result variant")
		})
	}

	for _, foreign := range []string{`"content":[]`, `"contents":[]`, `"messages":[]`, `"tools":[]`, `"ttlMs":0`} {
		var input InputRequiredResult
		err := json.Unmarshal([]byte(`{"resultType":"input_required","requestState":"x",`+foreign+`}`), &input)
		require.ErrorContains(t, err, "different result variant", foreign)
	}
}

func TestTask34AdversarialResultUnionFixtureFailClosed(t *testing.T) {
	// Arrange.
	var fixtures map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(readTask34Fixture(t, "adversarial", "result-unions.json"), &fixtures))

	for name, raw := range fixtures {
		t.Run(name, func(t *testing.T) {
			var target any
			switch name {
			case "emptyInputRequired":
				target = &InputRequiredResult{}
			case "inputRequiredOnList":
				target = &ToolsListResult{}
			case "foreignCompleteFields":
				target = &CompleteResult{}
			default:
				target = &CallToolResult{}
			}

			// Act.
			err := json.Unmarshal(raw, target)

			// Assert.
			require.Error(t, err)
		})
	}
}

func TestOutboundExtraAndMetaRejectRecursiveDuplicateJSON(t *testing.T) {
	// Arrange.
	duplicate := json.RawMessage(`{"nested":{"key":1,"key":2}}`)
	meta := Meta{"vendor.example/value": duplicate}
	result := CompleteResult{ResultType: ResultTypeComplete, Extra: Meta{"vendor.example": duplicate}}

	// Act.
	_, metaErr := json.Marshal(meta)
	_, resultErr := json.Marshal(result)

	// Assert.
	require.ErrorContains(t, metaErr, "duplicate")
	require.ErrorContains(t, resultErr, "duplicate")
}

func TestSubscriptionMetaIgnoresMalformedServerInfo(t *testing.T) {
	// Arrange.
	raw := []byte(`{"io.modelcontextprotocol/subscriptionId":7,"io.modelcontextprotocol/serverInfo":{"name":1}}`)

	// Act.
	var meta SubscriptionResultMeta
	err := json.Unmarshal(raw, &meta)

	// Assert.
	require.NoError(t, err)
	require.Nil(t, meta.ServerInfo)
	require.JSONEq(t, `7`, string(meta.SubscriptionID))
}

// Full schema fixtures stay inline to expose dialect and reference placement.
func TestMCPToolCompilesSchemasAtWireBoundary(t *testing.T) {
	// Arrange.
	valid := []byte(
		`{"name":"valid","inputSchema":{"type":"object","$defs":{"id":{"type":"string"}},"properties":{"id":{"$ref":"#/$defs/id"}}},"outputSchema":{"type":"array","items":{"type":"integer"}}}`,
	)
	remoteRef := []byte(
		`{"name":"invalid","inputSchema":{"type":"object","properties":{"id":{"$ref":"https://example.test/schema"}}}}`,
	)

	// Act.
	var tool MCPTool
	validErr := json.Unmarshal(valid, &tool)
	var invalid MCPTool
	invalidErr := json.Unmarshal(remoteRef, &invalid)

	// Assert.
	require.NoError(t, validErr)
	require.ErrorContains(t, invalidErr, "external schema reference")
}

func TestCapabilityExtensionsPreserveCurrentMCPNamespace(t *testing.T) {
	for _, target := range []any{&ClientCapabilities{}, &ServerCapabilities{}} {
		err := json.Unmarshal([]byte(`{"extensions":{"io.modelcontextprotocol/tasks":{}}}`), target)
		require.NoError(t, err)
	}
}

// The table is the executable DTO-family contract matrix.
func TestStrictDTOFamiliesRejectNullAndPreserveAdditiveFields(t *testing.T) {
	probes := []struct {
		name      string
		valid     string
		nullKnown string
		newTarget func() any
	}{
		{
			"server capabilities",
			`{"tools":{},"vendor":{"x":1}}`,
			`{"tools":null}`,
			func() any { return &ServerCapabilities{} },
		},
		{
			"discover",
			`{"resultType":"complete","supportedVersions":["2026-07-28"],"capabilities":{},"ttlMs":0,"cacheScope":"private","vendor":{"x":1}}`,
			`{"resultType":"complete","supportedVersions":["2026-07-28"],"capabilities":{},"ttlMs":0,"cacheScope":"private","instructions":null}`,
			func() any { return &DiscoverResult{} },
		},
		{
			"tool result",
			`{"resultType":"complete","content":[],"vendor":{"x":1}}`,
			`{"resultType":"complete","content":[],"isError":null}`,
			func() any { return &CallToolResult{} },
		},
		{
			"tool",
			`{"name":"t","inputSchema":{"type":"object"},"vendor":{"x":1}}`,
			`{"name":"t","title":null,"inputSchema":{"type":"object"}}`,
			func() any { return &MCPTool{} },
		},
		{
			"resource",
			`{"uri":"file:///a","name":"a","vendor":{"x":1}}`,
			`{"uri":"file:///a","name":"a","icons":null}`,
			func() any { return &Resource{} },
		},
		{
			"resource template",
			`{"uriTemplate":"file:///{path}","name":"a","vendor":{"x":1}}`,
			`{"uriTemplate":"file:///{path}","name":"a","annotations":null}`,
			func() any { return &ResourceTemplate{} },
		},
		{"prompt", `{"name":"p","vendor":{"x":1}}`, `{"name":"p","arguments":null}`, func() any { return &Prompt{} }},
		{
			"prompt argument",
			`{"name":"a","vendor":{"x":1}}`,
			`{"name":"a","required":null}`,
			func() any { return &PromptArgument{} },
		},
		{
			"prompt message",
			`{"role":"user","content":{"type":"text","text":"x"},"vendor":{"x":1}}`,
			`{"role":null,"content":{"type":"text","text":"x"}}`,
			func() any { return &PromptMessage{} },
		},
		{
			"implementation",
			`{"name":"i","version":"1","vendor":{"x":1}}`,
			`{"name":"i","version":"1","icons":null}`,
			func() any { return &Implementation{} },
		},
		{
			"icon",
			`{"src":"https://example.test/a.png","vendor":{"x":1}}`,
			`{"src":"https://example.test/a.png","sizes":null}`,
			func() any { return &Icon{} },
		},
		{"annotations", `{"priority":0.5,"vendor":{"x":1}}`, `{"priority":null}`, func() any { return &Annotations{} }},
		{
			"tool annotations",
			`{"readOnlyHint":true,"vendor":{"x":1}}`,
			`{"readOnlyHint":null}`,
			func() any { return &ToolAnnotations{} },
		},
	}

	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			// Arrange/Act.
			target := probe.newTarget()
			err := json.Unmarshal([]byte(probe.valid), target)
			roundTrip, marshalErr := json.Marshal(target)
			nullErr := json.Unmarshal([]byte(probe.nullKnown), probe.newTarget())

			// Assert.
			require.NoError(t, err)
			require.NoError(t, marshalErr)
			require.JSONEq(t, probe.valid, string(roundTrip))
			require.Error(t, nullErr)
		})
	}
}

func TestMetaIdentifiersRejectEmptyAndPrefixOnlyKeys(t *testing.T) {
	for _, key := range []string{"", "vendor.example/"} {
		_, err := json.Marshal(Meta{key: json.RawMessage(`true`)})
		require.ErrorContains(t, err, "invalid _meta key")
	}
}

func TestCallerRequestMetaRejectsAllReservedSecondLabelNamespaces(t *testing.T) {
	for _, key := range []string{
		"io.modelcontextprotocol/custom",
		"dev.mcp/custom",
		"org.modelcontextprotocol.foo/custom",
	} {
		meta := RequestMeta{
			ProtocolVersion:    ProtocolVersion,
			ClientCapabilities: ClientCapabilities{},
			ClientInfo:         &Implementation{Name: "test", Version: "1"},
			Extra:              Meta{key: json.RawMessage(`true`)},
		}
		_, err := json.Marshal(meta)
		require.ErrorContains(t, err, "reserved key", key)
	}
}

func TestContentBlockVariantsRejectNullWrongTypeAndForeignOptionals(t *testing.T) {
	invalid := []string{
		`{"type":"text","text":null}`,
		`{"type":"text","text":1}`,
		`{"type":"text","text":"x","annotations":null}`,
		`{"type":"image","data":null,"mimeType":"image/png"}`,
		`{"type":"audio","data":"","mimeType":null}`,
		`{"type":"resource_link","uri":"file:///a","name":null}`,
		`{"type":"resource_link","uri":"file:///a","name":"a","size":null}`,
		`{"type":"resource_link","uri":"file:///a","name":"a","icons":null}`,
		`{"type":"resource","resource":null}`,
	}
	for _, raw := range invalid {
		var block ContentBlock
		require.Error(t, json.Unmarshal([]byte(raw), &block), raw)
	}

	zeroSize := JSONNumber("0")
	valid := []ContentBlock{
		{Type: contentTypeText, Text: ""},
		{Type: contentTypeImage, Data: "", MIMEType: "image/png"},
		{Type: contentTypeResourceLink, URI: "file:///a", Name: "", Size: &zeroSize},
	}
	for _, block := range valid {
		raw, err := json.Marshal(block)
		require.NoError(t, err)
		var roundTrip ContentBlock
		require.NoError(t, json.Unmarshal(raw, &roundTrip))
	}
}

func TestCurrentClientCapabilityShapesAreTypedStrictAndInert(t *testing.T) {
	valid := `{"experimental":{"vendor":{}},"roots":{"future":{}},"sampling":{"context":{},"tools":{},"future":{}},"elicitation":{"form":{},"url":{},"future":{}},"extensions":{"io.modelcontextprotocol/tasks":{}},"vendor.example":{"x":1}}`
	var capabilities ClientCapabilities
	require.NoError(t, json.Unmarshal([]byte(valid), &capabilities))
	require.NotNil(t, capabilities.Roots)
	require.NotNil(t, capabilities.Sampling)
	require.NotNil(t, capabilities.Elicitation)
	require.JSONEq(t, `{}`, string(capabilities.Sampling.Context))
	require.JSONEq(t, `{}`, string(capabilities.Elicitation.URL))
	require.Contains(t, capabilities.Extra, "vendor.example")
	encoded, err := json.Marshal(capabilities)
	require.NoError(t, err)
	require.JSONEq(t, valid, string(encoded))

	invalid := []string{
		`{"experimental":null}`,
		`{"experimental":{"vendor":true}}`,
		`{"roots":null}`,
		`{"sampling":null}`,
		`{"sampling":{"context":null}}`,
		`{"sampling":{"tools":[]}}`,
		`{"elicitation":null}`,
		`{"elicitation":{"form":false}}`,
		`{"elicitation":{"url":null}}`,
	}
	for _, raw := range invalid {
		require.Error(t, json.Unmarshal([]byte(raw), &ClientCapabilities{}), raw)
	}
}

func TestMissingRequiredCapabilityErrorDecodesCurrentTypedShapes(t *testing.T) {
	rpcErr := &RPCError{
		Code:    JSONRPCMissingRequiredClientCapability,
		Message: "sampling required",
		Data:    json.RawMessage(`{"requiredCapabilities":{"sampling":{"tools":{}},"elicitation":{"url":{}}}}`),
	}

	err := TypedRPCError(rpcErr)
	var capabilityErr *MissingRequiredClientCapabilityError
	require.ErrorAs(t, err, &capabilityErr)
	require.NotNil(t, capabilityErr.RequiredCapabilities.Sampling)
	require.JSONEq(t, `{}`, string(capabilityErr.RequiredCapabilities.Sampling.Tools))
	require.NotNil(t, capabilityErr.RequiredCapabilities.Elicitation)
	require.JSONEq(t, `{}`, string(capabilityErr.RequiredCapabilities.Elicitation.URL))
}

func TestResourceSizesRequireLosslessJSONIntegersSymmetrically(t *testing.T) {
	for _, size := range []string{"1.0", "1e3", "9007199254740993"} {
		raw := []byte(`{"uri":"file:///a","name":"a","size":` + size + `}`)
		var resource Resource
		require.NoError(t, json.Unmarshal(raw, &resource), size)
		encoded, err := json.Marshal(resource)
		require.NoError(t, err, size)
		require.JSONEq(t, string(raw), string(encoded), size)

		blockRaw := []byte(`{"type":"resource_link","uri":"file:///a","name":"a","size":` + size + `}`)
		var block ContentBlock
		require.NoError(t, json.Unmarshal(blockRaw, &block), size)
		blockEncoded, err := json.Marshal(block)
		require.NoError(t, err, size)
		require.JSONEq(t, string(blockRaw), string(blockEncoded), size)
	}
	for _, size := range []string{"1.5", "1e-3", "null", `"1"`, "true", "[]", "{}"} {
		require.Error(t, json.Unmarshal([]byte(`{"uri":"file:///a","name":"a","size":`+size+`}`), &Resource{}))
		resourceLink := []byte(`{"type":"resource_link","uri":"file:///a","name":"a","size":` + size + `}`)
		require.Error(t, json.Unmarshal(resourceLink, &ContentBlock{}))
	}
}

func TestKnownOptionalWireFieldsRejectPresentNullAndInvalidEnums(t *testing.T) {
	baseMeta := `"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"c","version":"1"}`
	probes := []struct {
		raw    string
		target any
	}{
		{`{` + baseMeta + `,"io.modelcontextprotocol/logLevel":null}`, &RequestMeta{}},
		{`{` + baseMeta + `,"io.modelcontextprotocol/logLevel":"verbose"}`, &RequestMeta{}},
		{`{"extensions":null}`, &ClientCapabilities{}},
		{`{"resourceSubscriptions":null}`, &SubscriptionFilter{}},
		{`{"level":null,"data":null}`, &LogMessageParams{}},
		{`{"level":"verbose","data":null}`, &LogMessageParams{}},
	}
	for _, probe := range probes {
		require.Error(t, json.Unmarshal([]byte(probe.raw), probe.target), probe.raw)
	}

	for _, level := range []string{"alert", "critical", "debug", "emergency", "error", "info", "notice", "warning"} {
		var params LogMessageParams
		require.NoError(t, json.Unmarshal([]byte(`{"level":"`+level+`","data":null}`), &params))
	}
}

func TestResourceTemplateValidatesRFC6570ExpressionsSymmetrically(t *testing.T) {
	for _, value := range []string{"file:///{path}", "https://example.test{/segments*}{?q,limit:3}", "relative/{name}"} {
		raw := []byte(`{"uriTemplate":` + strconv.Quote(value) + `,"name":"resource"}`)
		var descriptor ResourceTemplate
		require.NoError(t, json.Unmarshal(raw, &descriptor), value)
		_, err := json.Marshal(descriptor)
		require.NoError(t, err, value)
	}
	for _, value := range []string{"file:///{", "file:///}", "file:///{?}", "file:///{bad-name}", "file:///{name:12345}", "file:///{a,{b}}"} {
		raw := []byte(`{"uriTemplate":` + strconv.Quote(value) + `,"name":"resource"}`)
		var descriptor ResourceTemplate
		require.Error(t, json.Unmarshal(raw, &descriptor), value)
	}
	_, err := json.Marshal(ResourceTemplate{URITemplate: "file:///{bad-name}", Name: "resource"})
	require.Error(t, err)
}
