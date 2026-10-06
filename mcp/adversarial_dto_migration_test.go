package mcp

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocol_StrictRequiredFieldsAndTaggedUnions(t *testing.T) {
	// Arrange: every negative contains unrelated current required fields.
	fixtures := []struct {
		name, raw, good, field string
		into                   any
	}{
		{
			"missing tool content",
			`{"resultType":"complete"}`,
			`{"resultType":"complete","content":[]}`,
			"content",
			&CallToolResult{},
		},
		{
			"null resource contents",
			`{"resultType":"complete","ttlMs":0,"cacheScope":"private","contents":null}`,
			`{"resultType":"complete","ttlMs":0,"cacheScope":"private","contents":[]}`,
			"contents",
			&ResourcesReadResult{},
		},
		{
			"missing text",
			`{"resultType":"complete","content":[{"type":"text"}]}`,
			`{"resultType":"complete","content":[{"type":"text","text":"x"}]}`,
			"text",
			&CallToolResult{},
		},
		{
			"cross variant field",
			`{"resultType":"complete","content":[{"type":"text","text":"x","data":"bad"}]}`,
			`{"resultType":"complete","content":[{"type":"text","text":"x"}]}`,
			"data",
			&CallToolResult{},
		},
		{
			"invalid prompt role",
			`{"resultType":"complete","messages":[{"role":"system","content":{"type":"text","text":"x"}}]}`,
			`{"resultType":"complete","messages":[{"role":"user","content":{"type":"text","text":"x"}}]}`,
			"role",
			&PromptsGetResult{},
		},
		{
			"resource without uri",
			`{"resultType":"complete","ttlMs":0,"cacheScope":"private","contents":[{"text":"x"}]}`,
			`{"resultType":"complete","ttlMs":0,"cacheScope":"private","contents":[{"uri":"file:///x","text":"x"}]}`,
			"uri",
			&ResourcesReadResult{},
		},
		{
			"invalid audience",
			`{"resultType":"complete","content":[{"type":"text","text":"x","annotations":{"audience":["system"]}}]}`,
			`{"resultType":"complete","content":[{"type":"text","text":"x","annotations":{"audience":["user"]}}]}`,
			"audience",
			&CallToolResult{},
		},
		{
			"null text",
			`{"resultType":"complete","content":[{"type":"text","text":null}]}`,
			`{"resultType":"complete","content":[{"type":"text","text":"x"}]}`,
			"text",
			&CallToolResult{},
		},
		{
			"null image data",
			`{"resultType":"complete","content":[{"type":"image","data":null,"mimeType":"image/png"}]}`,
			`{"resultType":"complete","content":[{"type":"image","data":"AAE=","mimeType":"image/png"}]}`,
			"data",
			&CallToolResult{},
		},
		{
			"null resource text",
			`{"resultType":"complete","ttlMs":0,"cacheScope":"private","contents":[{"uri":"file:///x","text":null}]}`,
			`{"resultType":"complete","ttlMs":0,"cacheScope":"private","contents":[{"uri":"file:///x","text":"x"}]}`,
			"text",
			&ResourcesReadResult{},
		},
		{
			"null progress",
			`{"progressToken":"x","progress":null}`,
			`{"progressToken":"x","progress":1}`,
			"progress",
			&ProgressParams{},
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			positive := reflect.New(reflect.TypeOf(fixture.into).Elem()).Interface()
			require.NoError(t, json.Unmarshal([]byte(fixture.good), positive))
			// Act.
			err := json.Unmarshal([]byte(fixture.raw), fixture.into)
			// Assert: missing envelope fields cannot mask the targeted failure.
			require.ErrorContains(t, err, fixture.field)
		})
	}
}
