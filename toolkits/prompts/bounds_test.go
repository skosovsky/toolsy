package prompts

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type documentProvider struct{ document Document }

func (p *documentProvider) Get(context.Context, string, map[string]any) (Document, error) {
	return p.document, nil
}

func TestProviderRequired(t *testing.T) {
	// Arrange.
	var typedNil *documentProvider
	for _, provider := range []Provider{nil, typedNil} {
		// Act.
		_, err := AsTool(provider)
		// Assert.
		require.Error(t, err)
	}
}

func TestPromptLimitConfiguration(t *testing.T) {
	// Arrange.
	for _, opt := range []Option{WithMaxBytes(-1), WithMaxSourceBytes(-1), WithMaxProvenanceBytes(-1), WithMaxOutputBytes(-1)} {
		// Act.
		_, err := AsTool(&documentProvider{}, opt)
		// Assert.
		require.Error(t, err)
	}
	_, err := AsTool(
		&documentProvider{},
		WithMaxBytes(0),
		WithMaxSourceBytes(0),
		WithMaxProvenanceBytes(0),
		WithMaxOutputBytes(0),
	)
	require.NoError(t, err)
}

func TestPromptDocumentBounds(t *testing.T) {
	for _, fixture := range []struct {
		name     string
		document Document
		opts     []Option
	}{
		{"escapedWire", Document{Instructions: strings.Repeat("<", 20)}, []Option{WithMaxOutputBytes(80)}},
		{"multibyteItem", Document{Instructions: "界界"}, []Option{WithMaxBytes(5)}},
		{"sourceTotal", Document{Instructions: "abc", Source: "def", Version: "ghi"}, []Option{WithMaxSourceBytes(8)}},
		{"provenance", Document{Instructions: "abc", Source: "long-source"}, []Option{WithMaxProvenanceBytes(3)}},
		{"invalidUTF8", Document{Instructions: string([]byte{255})}, nil},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			tool, err := AsTool(&documentProvider{document: fixture.document}, fixture.opts...)
			require.NoError(t, err)
			yielded := false
			// Act.
			err = tool.Execute(
				context.Background(),
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(`{"role_id":"test"}`)},
				func(toolsy.Chunk) error { yielded = true; return nil },
			)
			// Assert.
			require.Error(t, err)
			require.False(t, yielded)
		})
	}
}

func TestPromptDocumentProvenanceAndExactWireBoundary(t *testing.T) {
	// Arrange.
	document := Document{Instructions: "<界", Source: "host://roles/test", Version: "42"}
	raw, err := json.Marshal(document)
	require.NoError(t, err)
	tool, err := AsTool(&documentProvider{document: document}, WithMaxOutputBytes(len(raw)))
	require.NoError(t, err)
	var output []byte
	// Act.
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"role_id":"test"}`)},
		func(c toolsy.Chunk) error { output = append([]byte(nil), c.Data...); return nil },
	)
	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(output))
	require.Len(t, output, len(raw))
}
