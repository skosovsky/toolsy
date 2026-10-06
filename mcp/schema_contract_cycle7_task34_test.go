package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
	"github.com/skosovsky/toolsy/textprocessor"
)

func TestTask34PinnedSchemaRequiresIntegerResourceSizes(t *testing.T) {
	// Arrange.
	schema := task34PinnedSchema(t)
	for _, definitionName := range []string{"Resource", "ResourceLink"} {
		definition := task34SchemaDefinition(t, schema, definitionName)
		properties := task34SchemaObject(t, definition, "properties")
		size := task34SchemaObject(t, properties, "size")

		// Assert.
		require.Equal(t, "integer", size["type"], definitionName)
	}

	validResources := []struct {
		name string
		raw  string
		into any
	}{
		{"resource", `{"uri":"file:///tmp/a","name":"a","size":0}`, &Resource{}},
		{"resource link", `{"type":"resource_link","uri":"file:///tmp/a","name":"a","size":0}`, &ContentBlock{}},
	}
	for _, fixture := range validResources {
		t.Run("valid "+fixture.name, func(t *testing.T) {
			// Act and assert.
			require.NoError(t, json.Unmarshal([]byte(fixture.raw), fixture.into))
		})
	}

	for _, size := range []string{"1.5", `"1"`, "null"} {
		t.Run("invalid resource size "+size, func(t *testing.T) {
			// Act.
			var resource Resource
			resourceErr := json.Unmarshal(
				[]byte(`{"uri":"file:///tmp/a","name":"a","size":`+size+`}`),
				&resource,
			)
			var link ContentBlock
			linkErr := json.Unmarshal(
				[]byte(`{"type":"resource_link","uri":"file:///tmp/a","name":"a","size":`+size+`}`),
				&link,
			)

			// Assert.
			require.Error(t, resourceErr)
			require.Error(t, linkErr)
		})
	}
}

func TestTask34PinnedSchemaMRTRInputUnionsAreExact(t *testing.T) {
	// Arrange.
	schema := task34PinnedSchema(t)
	expectedRequests := []string{
		"#/$defs/CreateMessageRequest",
		"#/$defs/ListRootsRequest",
		"#/$defs/ElicitRequest",
	}
	expectedResponses := []string{
		"#/$defs/CreateMessageResult",
		"#/$defs/ListRootsResult",
		"#/$defs/ElicitResult",
	}

	// Act and assert.
	require.ElementsMatch(t, expectedRequests, task34SchemaAnyOfRefs(t, schema, "InputRequest"))
	require.ElementsMatch(t, expectedResponses, task34SchemaAnyOfRefs(t, schema, "InputResponse"))
	require.Equal(
		t,
		"#/$defs/InputRequest",
		task34SchemaAdditionalPropertyRef(t, schema, "InputRequests"),
	)
	require.Equal(
		t,
		"#/$defs/InputResponse",
		task34SchemaAdditionalPropertyRef(t, schema, "InputResponses"),
	)

	requestFixtures := []string{
		`{"sample":{"method":"sampling/createMessage","params":{"maxTokens":1,"messages":[]}}}`,
		`{"roots":{"method":"roots/list"}}`,
		`{"elicit":{"method":"elicitation/create","params":{"message":"confirm","requestedSchema":{"type":"object","properties":{}}}}}`,
	}
	responseFixtures := []string{
		`{"sample":{"content":{"type":"text","text":"ok"},"model":"test","role":"assistant"}}`,
		`{"roots":{"roots":[]}}`,
		`{"elicit":{"action":"cancel"}}`,
	}
	for _, raw := range requestFixtures {
		require.NoError(t, task34ValidateSchemaDefinition(schema, "InputRequests", raw), raw)
	}
	for _, raw := range responseFixtures {
		require.NoError(t, task34ValidateSchemaDefinition(schema, "InputResponses", raw), raw)
	}
	require.Error(
		t,
		task34ValidateSchemaDefinition(schema, "InputRequests", `{"future":{"method":"future/request"}}`),
	)
	require.Error(
		t,
		task34ValidateSchemaDefinition(schema, "InputResponses", `{"future":{"value":true}}`),
	)
}

func TestTask34CanonicalBase64RejectsNonCanonicalSpellings(t *testing.T) {
	// Arrange.
	validContent := []string{
		`{"type":"image","data":"YQ==","mimeType":"image/png"}`,
		`{"type":"audio","data":"YQ==","mimeType":"audio/wav"}`,
	}
	for _, raw := range validContent {
		var block ContentBlock
		require.NoError(t, json.Unmarshal([]byte(raw), &block), raw)
	}
	var validResource ResourceContents
	require.NoError(t, json.Unmarshal(
		[]byte(`{"uri":"file:///tmp/a","blob":"YQ=="}`),
		&validResource,
	))

	invalid := []string{"YQ", "YR==", "YQ===", "YQ==\n", "_w=="}
	for _, encoded := range invalid {
		t.Run(fmt.Sprintf("%q", encoded), func(t *testing.T) {
			imageRaw, err := json.Marshal(map[string]any{
				"type": "image", "data": encoded, "mimeType": "image/png",
			})
			require.NoError(t, err)
			resourceRaw, err := json.Marshal(map[string]any{
				"uri": "file:///tmp/a", "blob": encoded,
			})
			require.NoError(t, err)

			// Act.
			var block ContentBlock
			contentErr := json.Unmarshal(imageRaw, &block)
			var contents ResourceContents
			resourceErr := json.Unmarshal(resourceRaw, &contents)

			// Assert.
			require.Error(t, contentErr)
			require.Error(t, resourceErr)
		})
	}
}

func TestTask34ReservedHTTPErrorStatusCoherence(t *testing.T) {
	// Arrange.
	schema := task34PinnedSchema(t)
	cases := []struct {
		name       string
		definition string
		code       JSONNumber
		data       string
		assertType func(*testing.T, error)
	}{
		{
			name:       "header mismatch",
			definition: "HeaderMismatchError",
			code:       JSONRPCHeaderMismatch,
			assertType: func(t *testing.T, err error) {
				var target *HeaderMismatchError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:       "missing capability",
			definition: "MissingRequiredClientCapabilityError",
			code:       JSONRPCMissingRequiredClientCapability,
			data:       `,"data":{"requiredCapabilities":{"roots":{}}}`,
			assertType: func(t *testing.T, err error) {
				var target *MissingRequiredClientCapabilityError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:       "unsupported version",
			definition: "UnsupportedProtocolVersionError",
			code:       JSONRPCUnsupportedProtocolVersion,
			data:       `,"data":{"requested":"draft","supported":["2026-07-28"]}`,
			assertType: func(t *testing.T, err error) {
				var target *UnsupportedProtocolVersionError
				require.ErrorAs(t, err, &target)
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			definition := task34SchemaDefinition(t, schema, testCase.definition)
			require.Contains(t, definition["description"], "400 Bad Request")
			errorSchema := task34SchemaObject(t, task34SchemaObject(t, definition, "properties"), "error")
			allOf, ok := errorSchema["allOf"].([]any)
			require.True(t, ok)
			require.Len(t, allOf, 2)
			override, ok := allOf[1].(map[string]any)
			require.True(t, ok)
			codeSchema := task34SchemaObject(t, task34SchemaObject(t, override, "properties"), "code")
			require.Equal(t, json.Number(fmt.Sprint(testCase.code)), codeSchema["const"])

			for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized} {
				t.Run(http.StatusText(status), func(t *testing.T) {
					awaitErr := task34ReservedHTTPError(t, status, testCase.code, testCase.data)
					if status == http.StatusBadRequest {
						testCase.assertType(t, awaitErr)
						return
					}
					var httpErr *HTTPError
					require.ErrorAs(t, awaitErr, &httpErr)
					require.Equal(t, status, httpErr.StatusCode)
				})
			}
		})
	}
}

func TestTask34StdioRejectsOversizedOutgoingFrameBeforeWrite(t *testing.T) {
	// Arrange. PrepareRequest performs the bound check before delivery, so no
	// process or writer is needed to prove the oversized frame cannot reach I/O.
	const frameLimit = 256
	transport := NewStdioTransport(
		"",
		nil,
		WithStdioLimits(TransportLimits{MaxFrameBytes: frameLimit, MaxQueueBytes: frameLimit}),
	)
	transport.started = true
	transport.peer = newRPCPeer(t.Context(), slog.Default(), transport.send)
	params := ToolsCallParams{
		Name:      "oversized",
		Arguments: json.RawMessage(`{"payload":"` + strings.Repeat("x", frameLimit) + `"}`),
		Meta:      requestMetaForContract(),
	}

	// Act.
	pending, err := transport.PrepareRequest(t.Context(), MethodToolsCall, params)

	// Assert.
	require.Nil(t, pending)
	var payloadErr *InvalidPayloadError
	require.ErrorAs(t, err, &payloadErr)
	require.Equal(t, "stdio outgoing frame", payloadErr.Subject)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func task34PinnedSchema(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(
		"testdata", "task34", "schema", "mcp-2026-07-28.schema.json",
	))
	require.NoError(t, err)
	decoded, err := jsonschemax.Decode(raw)
	require.NoError(t, err)
	schema, ok := decoded.(map[string]any)
	require.True(t, ok)
	return schema
}

func task34SchemaDefinition(t *testing.T, schema map[string]any, name string) map[string]any {
	t.Helper()
	return task34SchemaObject(t, task34SchemaObject(t, schema, "$defs"), name)
}

func task34SchemaObject(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := parent[key].(map[string]any)
	require.True(t, ok, key)
	return value
}

func task34SchemaAnyOfRefs(t *testing.T, schema map[string]any, name string) []string {
	t.Helper()
	definition := task34SchemaDefinition(t, schema, name)
	items, ok := definition["anyOf"].([]any)
	require.True(t, ok)
	refs := make([]string, 0, len(items))
	for _, item := range items {
		entry, entryOK := item.(map[string]any)
		require.True(t, entryOK)
		ref, refOK := entry["$ref"].(string)
		require.True(t, refOK)
		refs = append(refs, ref)
	}
	return refs
}

func task34SchemaAdditionalPropertyRef(t *testing.T, schema map[string]any, name string) string {
	t.Helper()
	additional := task34SchemaObject(t, task34SchemaDefinition(t, schema, name), "additionalProperties")
	ref, ok := additional["$ref"].(string)
	require.True(t, ok)
	return ref
}

func task34ValidateSchemaDefinition(schema map[string]any, name, raw string) error {
	schema["$ref"] = "#/$defs/" + name
	compiled, err := jsonschemax.Compile(schema)
	if err != nil {
		return err
	}
	instance, err := jsonschemax.Decode([]byte(raw))
	if err != nil {
		return err
	}
	return compiled.Validate(instance)
}

func task34ReservedHTTPError(t *testing.T, status int, code JSONNumber, data string) error {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		var envelope Request
		if err = json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode request body: %v", err)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		_, err = fmt.Fprintf(
			writer,
			`{"jsonrpc":"2.0","id":%s,"error":{"code":%s,"message":"rejected"%s}}`,
			envelope.ID,
			code,
			data,
		)
		if err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(t.Context()))
	defer transport.Close()
	pending, err := transport.PrepareRequest(t.Context(), MethodServerDiscover, DiscoverParams{
		Meta: requestMetaForContract(),
	})
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	_, err = pending.Await(t.Context())
	return err
}
