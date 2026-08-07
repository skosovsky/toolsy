package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func readTask34Fixture(t *testing.T, parts ...string) json.RawMessage {
	t.Helper()
	path := filepath.Join(append([]string{"testdata", "task34"}, parts...)...)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}

func fixtureResult(t *testing.T, raw json.RawMessage) json.RawMessage {
	t.Helper()
	fields, err := decodeObjectFields(raw)
	require.NoError(t, err)
	require.Contains(t, fields, "result")
	return fields["result"]
}

func TestTask34OfficialResultFixturesDecodeStrictly(t *testing.T) {
	// Arrange.
	discoveryRaw := fixtureResult(t, readTask34Fixture(t, "official", "discover-response.json"))
	toolsRaw := fixtureResult(t, readTask34Fixture(t, "official", "tools-list-response.json"))
	inputRaw := fixtureResult(t, readTask34Fixture(t, "official", "input-required-response.json"))
	contentRaw := readTask34Fixture(t, "official", "content-result.json")

	// Act.
	var discovery DiscoverResult
	discoveryErr := json.Unmarshal(discoveryRaw, &discovery)
	var tools ToolsListResult
	toolsErr := json.Unmarshal(toolsRaw, &tools)
	var input InputRequiredResult
	inputErr := json.Unmarshal(inputRaw, &input)
	var content CallToolResult
	contentErr := json.Unmarshal(contentRaw, &content)

	// Assert.
	require.NoError(t, discoveryErr)
	require.Equal(t, []string{ProtocolVersion}, discovery.SupportedVersions)
	require.Equal(t, JSONNumber("3600000"), discovery.TTLMS)
	require.NoError(t, toolsErr)
	require.Equal(t, JSONNumber("30000"), tools.TTLMS)
	require.NoError(t, inputErr)
	require.JSONEq(t, `"opaque-round-1"`, string(input.RequestState))
	require.NoError(t, contentErr)
	require.Len(t, content.Content, 6)
}

func TestTask34AdversarialCacheAndDuplicateFixturesFailClosed(t *testing.T) {
	// Arrange.
	var cacheCases map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(
		readTask34Fixture(t, "adversarial", "cache-invalid.json"),
		&cacheCases,
	))
	duplicate := readTask34Fixture(t, "adversarial", "duplicate-result-key.json")

	// Act/Assert.
	for name, raw := range cacheCases {
		t.Run(name, func(t *testing.T) {
			var result ToolsListResult
			require.Error(t, json.Unmarshal(raw, &result))
		})
	}
	require.Error(t, validateJSONValue(duplicate))
}

func TestTask34OfficialRequestAndHTTPFixtures(t *testing.T) {
	// Arrange.
	discoverRaw := readTask34Fixture(t, "official", "discover-request.json")
	retryRaw := readTask34Fixture(t, "official", "input-retry-request.json")
	var httpFixture struct {
		Headers map[string]string `json:"headers"`
		Body    Request           `json:"body"`
	}
	require.NoError(t, json.Unmarshal(
		readTask34Fixture(t, "official", "http-tools-call.json"),
		&httpFixture,
	))

	// Act.
	var discover Request
	discoverErr := json.Unmarshal(discoverRaw, &discover)
	var discoverParams DiscoverParams
	discoverParamsErr := json.Unmarshal(discover.Params, &discoverParams)
	var retry Request
	retryErr := json.Unmarshal(retryRaw, &retry)
	var retryParams ToolsCallParams
	retryParamsErr := json.Unmarshal(retry.Params, &retryParams)
	var httpParams ToolsCallParams
	httpParamsErr := json.Unmarshal(httpFixture.Body.Params, &httpParams)

	// Assert.
	require.NoError(t, discoverErr)
	require.NoError(t, discoverParamsErr)
	require.Equal(t, MethodServerDiscover, discover.Method)
	require.Equal(t, ProtocolVersion, discoverParams.Meta.ProtocolVersion)
	require.NoError(t, retryErr)
	require.NoError(t, retryParamsErr)
	require.Equal(t, MethodToolsCall, retry.Method)
	require.JSONEq(
		t,
		`{"confirm":{"action":"accept","content":{"confirmed":true}}}`,
		string(retryParams.InputResponses),
	)
	require.JSONEq(t, `"opaque-round-1"`, string(retryParams.RequestState))
	require.NoError(t, httpParamsErr)
	require.Equal(t, MethodToolsCall, httpFixture.Headers["Mcp-Method"])
	require.Equal(t, httpParams.Name, httpFixture.Headers["Mcp-Name"])
	require.Equal(t, "=?base64?SGVsbG8sIOS4lueVjA==?=", httpFixture.Headers["Mcp-Param-Label"])
}

func TestTask34OfficialSubscriptionSequence(t *testing.T) {
	// Arrange.
	var fixture struct {
		Request Request           `json:"request"`
		Stream  []json.RawMessage `json:"stream"`
	}
	require.NoError(t, json.Unmarshal(
		readTask34Fixture(t, "official", "subscription-sequence.json"),
		&fixture,
	))

	// Act.
	var params SubscriptionsListenParams
	paramsErr := json.Unmarshal(fixture.Request.Params, &params)
	provenance := &sseRequestProvenance{
		requestID: bytes.Clone(fixture.Request.ID),
		method:    MethodSubscriptionsListen,
	}
	methods := make([]string, 0, len(fixture.Stream)-1)
	for _, frame := range fixture.Stream[:len(fixture.Stream)-1] {
		fields, err := decodeObjectFields(frame)
		require.NoError(t, err)
		var method string
		require.NoError(t, json.Unmarshal(fields["method"], &method))
		methods = append(methods, method)
		paramsFields, err := decodeObjectFields(fields["params"])
		require.NoError(t, err)
		meta, err := decodeObjectFields(paramsFields["_meta"])
		require.NoError(t, err)
		require.NoError(t, matchRPCID(meta[metaSubscriptionID], fixture.Request.ID))
		terminal, acknowledged, provenanceErr := validateSSEProvenance(frame, provenance)
		require.NoError(t, provenanceErr)
		require.False(t, terminal)
		provenance.acknowledged = provenance.acknowledged || acknowledged
	}
	terminalFrame, _, provenanceErr := validateSSEProvenance(
		fixture.Stream[len(fixture.Stream)-1],
		provenance,
	)
	terminal := fixtureResult(t, fixture.Stream[len(fixture.Stream)-1])
	var closed SubscriptionsListenResult
	terminalErr := json.Unmarshal(terminal, &closed)

	// Assert.
	require.NoError(t, paramsErr)
	require.Equal(t, MethodSubscriptionsListen, fixture.Request.Method)
	require.Equal(t, MethodSubscriptionsAcknowledged, methods[0])
	require.Equal(t, []string{
		MethodSubscriptionsAcknowledged,
		MethodToolsListChanged,
		MethodResourceUpdated,
	}, methods)
	require.NoError(t, terminalErr)
	require.NoError(t, provenanceErr)
	require.True(t, terminalFrame)
	require.NoError(t, matchRPCID(closed.Meta.SubscriptionID, fixture.Request.ID))
}

func TestTask34AdversarialDiscoveryAndResultUnionFixtures(t *testing.T) {
	// Arrange.
	legacyRaw := readTask34Fixture(t, "adversarial", "discover-legacy-only.json")
	wrongFieldRaw := readTask34Fixture(t, "adversarial", "discover-wrong-version-field.json")
	var unionCases map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(
		readTask34Fixture(t, "adversarial", "result-unions.json"),
		&unionCases,
	))

	// Act.
	var legacy DiscoverResult
	legacyErr := json.Unmarshal(legacyRaw, &legacy)
	var wrongField DiscoverResult
	wrongFieldErr := json.Unmarshal(wrongFieldRaw, &wrongField)
	containsCurrent := false
	for _, version := range legacy.SupportedVersions {
		containsCurrent = containsCurrent || version == ProtocolVersion
	}
	decodeErrors := make(map[string]error, len(unionCases))
	for name, raw := range unionCases {
		switch name {
		case "emptyInputRequired":
			decodeErrors[name] = json.Unmarshal(raw, &InputRequiredResult{})
		case "inputRequiredOnList":
			decodeErrors[name] = json.Unmarshal(raw, &ToolsListResult{})
		default:
			decodeErrors[name] = json.Unmarshal(raw, &CallToolResult{})
		}
	}

	// Assert.
	require.NoError(t, legacyErr)
	require.False(t, containsCurrent)
	require.Error(t, wrongFieldErr)
	for name, err := range decodeErrors {
		require.Error(t, err, name)
	}
}

func TestTask34AdversarialHeaderAndSchemaFixtures(t *testing.T) {
	// Arrange.
	var headerMismatch struct {
		Headers       map[string]string `json:"headers"`
		Body          Request           `json:"body"`
		ExpectedError struct {
			HTTPStatus  int        `json:"httpStatus"`
			JSONRPCCode JSONNumber `json:"jsonrpcCode"`
		} `json:"expectedError"`
	}
	require.NoError(t, json.Unmarshal(
		readTask34Fixture(t, "adversarial", "http-header-mismatch.json"),
		&headerMismatch,
	))
	var tools ToolsListResult
	require.NoError(t, json.Unmarshal(
		readTask34Fixture(t, "adversarial", "x-mcp-header-invalid-tools.json"),
		&tools,
	))
	var invalid map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(
		readTask34Fixture(t, "adversarial", "schema-and-content-invalid.json"),
		&invalid,
	))

	// Act.
	headerBodyFields, headerBodyErr := decodeObjectFields(headerMismatch.Body.Params)
	var bodyName string
	_ = json.Unmarshal(headerBodyFields["name"], &bodyName)
	toolErrors := make(map[string]error, len(tools.Tools))
	for _, tool := range tools.Tools {
		_, toolErrors[tool.Name] = compileHTTPToolHeaderBindings(tool.InputSchema)
	}
	_, remoteRefErr := decodeSchemaObject(invalid["remoteRefSchema"], false)
	_, unknownDialectErr := decodeSchemaObject(invalid["unknownDialectSchema"], false)
	contentErrors := make(map[string]error)
	for _, name := range []string{"invalidImageBase64", "unknownContentType"} {
		contentErrors[name] = json.Unmarshal(invalid[name], &ContentBlock{})
	}
	contentErrors["mixedResourceContents"] = json.Unmarshal(
		invalid["mixedResourceContents"],
		&ResourceContents{},
	)
	mappedHeaderError := TypedRPCError(&RPCError{
		Code:    headerMismatch.ExpectedError.JSONRPCCode,
		Message: "fixture header/body mismatch",
	})

	// Assert.
	require.NoError(t, headerBodyErr)
	require.NotEqual(t, headerMismatch.Headers["Mcp-Method"], headerMismatch.Body.Method)
	require.NotEqual(t, headerMismatch.Headers["Mcp-Name"], bodyName)
	require.Equal(t, JSONRPCHeaderMismatch, headerMismatch.ExpectedError.JSONRPCCode)
	var typedHeaderError *HeaderMismatchError
	require.ErrorAs(t, mappedHeaderError, &typedHeaderError)
	require.NoError(t, toolErrors["valid"])
	for name, err := range toolErrors {
		if name != "valid" {
			require.Error(t, err, name)
		}
	}
	require.Error(t, remoteRefErr)
	require.Error(t, unknownDialectErr)
	for name, err := range contentErrors {
		require.Error(t, err, name)
	}
}

func TestTask34AdversarialSubscriptionPeerAndLegacyMatrices(t *testing.T) {
	// Arrange.
	var subscriptionCases map[string][]json.RawMessage
	require.NoError(t, json.Unmarshal(
		readTask34Fixture(t, "adversarial", "subscription-invalid-sequences.json"),
		&subscriptionCases,
	))
	var peerCases map[string][]json.RawMessage
	require.NoError(t, json.Unmarshal(
		readTask34Fixture(t, "adversarial", "peer-sequences.json"),
		&peerCases,
	))
	var legacy struct {
		ProtocolVersions []string `json:"protocolVersions"`
		Methods          []string `json:"methods"`
		Headers          []string `json:"headers"`
		HTTPMethods      []string `json:"httpMethods"`
	}
	require.NoError(t, json.Unmarshal(
		readTask34Fixture(t, "adversarial", "legacy-forbidden.json"),
		&legacy,
	))

	// Act.
	requestID := json.RawMessage(`"listen-1"`)
	beforeAck := &sseRequestProvenance{requestID: requestID, method: MethodSubscriptionsListen}
	_, _, beforeAckErr := validateSSEProvenance(
		subscriptionCases["notificationBeforeAck"][0],
		beforeAck,
	)
	wrongID := &sseRequestProvenance{requestID: requestID, method: MethodSubscriptionsListen}
	_, acknowledged, wrongIDAckErr := validateSSEProvenance(
		subscriptionCases["wrongSubscription"][0],
		wrongID,
	)
	wrongID.acknowledged = acknowledged
	_, _, wrongIDErr := validateSSEProvenance(subscriptionCases["wrongSubscription"][1], wrongID)

	outsideAck := subscriptionCases["outsideEffectiveFilter"][0]
	outsideFields, outsideFieldsErr := decodeObjectFields(outsideAck)
	outsideParams, outsideParamsErr := decodeObjectFields(outsideFields["params"])
	var acknowledgedParams SubscriptionsAcknowledgedParams
	outsideAckErr := json.Unmarshal(outsideFields["params"], &acknowledgedParams)
	outsideMethod := fixtureMethod(t, subscriptionCases["outsideEffectiveFilter"][1])
	outsideAllowed := subscriptionAllows(acknowledgedParams.Notifications, InvalidationPrompts)

	peer := newRPCPeer(t.Context(), nil, func(context.Context, []byte) error { return nil })
	t.Cleanup(func() { peer.close(ErrTransportClosed) })
	_, _, beginErr := peer.beginRequest(MethodServerDiscover, DiscoverParams{Meta: &RequestMeta{
		ProtocolVersion:    ProtocolVersion,
		ClientCapabilities: ClientCapabilities{},
		ClientInfo:         &Implementation{Name: "fixture-host", Version: "1.0.0"},
	}})
	fractionalErr := peer.dispatch(peerCases["fractionalID"][0])
	serverRequestErr := peer.dispatch(peerCases["serverRequest"][0])
	firstTerminalErr := peer.dispatch(peerCases["duplicateTerminal"][0])
	duplicateTerminalErr := peer.dispatch(peerCases["duplicateTerminal"][1])
	unknownTerminalErr := peer.dispatch(peerCases["unknownTerminal"][0])
	legacyMethodErrors := make(map[string]error, len(legacy.Methods))
	for _, method := range legacy.Methods {
		legacyMethodErrors[method] = validateOutgoingRequest(method, RequestParams{})
	}

	// Assert.
	require.Error(t, beforeAckErr)
	require.NoError(t, wrongIDAckErr)
	require.True(t, acknowledged)
	require.Error(t, wrongIDErr)
	require.NoError(t, outsideFieldsErr)
	require.NoError(t, outsideParamsErr)
	require.Contains(t, outsideParams, "notifications")
	require.NoError(t, outsideAckErr)
	require.Equal(t, MethodPromptsListChanged, outsideMethod)
	require.False(t, outsideAllowed)
	require.NoError(t, beginErr)
	require.Error(t, fractionalErr)
	require.Error(t, serverRequestErr)
	require.NoError(t, firstTerminalErr)
	require.Error(t, duplicateTerminalErr)
	require.Error(t, unknownTerminalErr)
	for method, err := range legacyMethodErrors {
		require.Error(t, err, method)
	}
	require.Contains(t, legacy.ProtocolVersions, "2025-11-25")
	require.Contains(t, legacy.Methods, "initialize")
	require.Contains(t, legacy.Headers, "Mcp-Session-Id")
	require.ElementsMatch(t, []string{"GET", "DELETE"}, legacy.HTTPMethods)
}

func TestTask34FixtureInventoryIsComplete(t *testing.T) {
	// Arrange.
	expected := []string{
		"adversarial/cache-invalid.json",
		"adversarial/discover-legacy-only.json",
		"adversarial/discover-wrong-version-field.json",
		"adversarial/duplicate-result-key.json",
		"adversarial/http-header-mismatch.json",
		"adversarial/legacy-forbidden.json",
		"adversarial/peer-sequences.json",
		"adversarial/result-unions.json",
		"adversarial/schema-and-content-invalid.json",
		"adversarial/subscription-invalid-sequences.json",
		"adversarial/x-mcp-header-invalid-tools.json",
		"official/content-result.json",
		"official/discover-request.json",
		"official/discover-response.json",
		"official/http-tools-call.json",
		"official/input-required-response.json",
		"official/input-retry-request.json",
		"official/subscription-sequence.json",
		"official/tools-list-response.json",
		"schema/PROVENANCE.json",
		"schema/mcp-2026-07-28.schema.json",
	}

	// Act.
	var actual []string
	fixtureRoot := filepath.Join("testdata", "task34")
	require.NoError(t, filepath.WalkDir(
		fixtureRoot,
		func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || filepath.Ext(path) != ".json" {
				return err
			}
			relative, relErr := filepath.Rel(fixtureRoot, path)
			if relErr != nil {
				return relErr
			}
			actual = append(actual, filepath.ToSlash(relative))
			return nil
		},
	))
	sort.Strings(expected)
	sort.Strings(actual)

	// Assert.
	require.Equal(t, expected, actual)
}

func fixtureMethod(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	fields, err := decodeObjectFields(raw)
	require.NoError(t, err)
	var method string
	require.NoError(t, json.Unmarshal(fields["method"], &method))
	return method
}
