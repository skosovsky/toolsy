package rag

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

type mockRetriever struct {
	docs []Document
	err  error
}

func (m *mockRetriever) Retrieve(_ context.Context, _ string) ([]Document, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.docs, nil
}

func docsFromStrings(ss ...string) []Document {
	out := make([]Document, len(ss))
	for i, s := range ss {
		out[i] = Document{Content: s}
	}
	return out
}

func decodeSearchMarkdown(t *testing.T, c toolsy.Chunk) string {
	t.Helper()
	require.Equal(t, toolsy.MimeTypeJSON, c.MimeType)
	var out SearchMarkdownWire
	require.NoError(t, json.Unmarshal(c.Data, &out))
	return out.Results
}

func TestAsSearchTool_FormatsMarkdown(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings("a", "b", "c")}
	tool, err := AsSearchTool(r)
	require.NoError(t, err)
	var result string
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(c toolsy.Chunk) error {
				result = decodeSearchMarkdown(t, c)
				return nil
			},
		),
	)
	require.Equal(t, "1. a\n2. b\n3. c", result)
}

func TestAsSearchTool_MaxResults(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings("a", "b", "c")}
	tool, err := AsSearchTool(r, WithMaxResults(2))
	require.NoError(t, err)
	var result string
	_ = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(c toolsy.Chunk) error {
			result = decodeSearchMarkdown(t, c)
			return nil
		},
	)
	require.Equal(t, "1. a\n2. b", result)
}

func TestAsSearchTool_MaxBytesTruncate(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings(strings.Repeat("x", 100))}
	tool, err := AsSearchTool(r, WithMaxBytes(20))
	require.NoError(t, err)
	var wire []byte
	_ = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(c toolsy.Chunk) error {
			wire = append([]byte(nil), c.Data...)
			return nil
		},
	)
	require.LessOrEqual(t, len(wire), 20+len(textprocessor.TruncationSuffix)+2)
	require.Contains(t, string(wire), "[Truncated]")
}

func TestAsSearchTool_MaxBytesUTF8Safe(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings("привет мир")}
	tool, err := AsSearchTool(r, WithMaxBytes(17))
	require.NoError(t, err)
	var wire []byte
	_ = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(c toolsy.Chunk) error {
			wire = append([]byte(nil), c.Data...)
			return nil
		},
	)
	require.LessOrEqual(t, len(wire), 17+len(textprocessor.TruncationSuffix)+2)
	require.Contains(t, string(wire), "[Truncated]")
	require.True(t, utf8.Valid(wire), "expected valid UTF-8 wire, got %q", wire)
}

func TestAsSearchTool_RetrieverError(t *testing.T) {
	r := &mockRetriever{err: errors.New("backend down")}
	tool, err := AsSearchTool(r)
	require.NoError(t, err)
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
}

func TestAsSearchTool_EmptyResult(t *testing.T) {
	r := &mockRetriever{docs: nil}
	tool, err := AsSearchTool(r)
	require.NoError(t, err)
	var result string
	_ = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(c toolsy.Chunk) error {
			result = decodeSearchMarkdown(t, c)
			return nil
		},
	)
	require.Equal(t, "No results found.", result)
}

func TestAsSearchTool_ReadOnlyManifest(t *testing.T) {
	tool, err := AsSearchTool(&mockRetriever{docs: docsFromStrings("a")})
	require.NoError(t, err)
	require.True(t, tool.Manifest().ReadOnly)
}

func TestAsSearchTool_ShapeDocumentsJSON(t *testing.T) {
	r := &mockRetriever{docs: []Document{{Content: "a", SourceURI: "doc://1"}}}
	tool, err := AsSearchTool(r, WithResultShape(ShapeDocumentsJSON))
	require.NoError(t, err)
	var payload SearchDocumentsWire
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(c toolsy.Chunk) error {
				require.NoError(t, json.Unmarshal(c.Data, &payload))
				return nil
			},
		),
	)
	require.Len(t, payload.Documents, 1)
	require.Equal(t, "a", payload.Documents[0].Content)
}

func TestAsSearchTool_WithScopeFilter(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings("public", "secret")}
	tool, err := AsSearchTool(r, WithScopeFilter(func(_ context.Context, docs []Document) []Document {
		out := docs[:0]
		for _, d := range docs {
			if d.Content != "secret" {
				out = append(out, d)
			}
		}
		return out
	}))
	require.NoError(t, err)
	var result string
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(c toolsy.Chunk) error {
				result = decodeSearchMarkdown(t, c)
				return nil
			},
		),
	)
	require.Equal(t, "1. public", result)
}

func TestAsSearchTool_WithResultFormatter(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings("a")}
	tool, err := AsSearchTool(r, WithResultFormatter(func(docs []Document) (any, error) {
		return map[string]int{"count": len(docs)}, nil
	}))
	require.NoError(t, err)
	var payload map[string]int
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(c toolsy.Chunk) error {
				require.NoError(t, json.Unmarshal(c.Data, &payload))
				return nil
			},
		),
	)
	require.Equal(t, 1, payload["count"])
}

func TestAsSearchTool_ValidatorOnly_ShapeMarkdown(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings("a")}
	tool, err := AsSearchTool(r, WithHostResultValidator(func(v any) error {
		payload, ok := v.(SearchMarkdownWire)
		if !ok {
			return errors.New("expected SearchMarkdownWire envelope")
		}
		if payload.Results == "" {
			return errors.New("expected non-empty results")
		}
		return nil
	}))
	require.NoError(t, err)
	var out SearchMarkdownWire
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(c toolsy.Chunk) error {
				require.NoError(t, json.Unmarshal(c.Data, &out))
				return nil
			},
		),
	)
	require.Contains(t, out.Results, "a")
}

func TestAsSearchTool_ValidatorOnly_Reject(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings("a")}
	tool, err := AsSearchTool(r, WithHostResultValidator(func(_ any) error {
		return assert.AnError
	}))
	require.NoError(t, err)
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestAsSearchTool_WithMaxBytes_ShapeDocumentsJSON_ExceedsWireBudget(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings(strings.Repeat("x", 200), strings.Repeat("y", 200))}
	tool, err := AsSearchTool(r, WithResultShape(ShapeDocumentsJSON), WithMaxBytes(80))
	require.NoError(t, err)
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "80")
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestCapDocumentsForWire_CanceledReturnsInternal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := capDocumentsForWire(ctx, docsFromStrings(strings.Repeat("x", 100)), &options{maxBytes: 50})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestCapDocumentsForWire_ExceedsWireBudget(t *testing.T) {
	_, err := capDocumentsForWire(
		context.Background(),
		docsFromStrings(strings.Repeat("x", 100)),
		&options{maxBytes: 50},
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "search results exceeds")
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestAsSearchTool_ShapeDocumentsJSON_SingleWireTruncSuffix(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings(strings.Repeat("x", 45))}
	tool, err := AsSearchTool(r, WithResultShape(ShapeDocumentsJSON), WithMaxBytes(80))
	require.NoError(t, err)
	var wire []byte
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), 80+len(textprocessor.TruncationSuffix)+2)
	require.LessOrEqual(t, strings.Count(string(wire), "[Truncated]"), 1)
	var payload SearchDocumentsWire
	require.NoError(t, json.Unmarshal(wire, &payload))
	if len(payload.Documents) > 0 {
		require.NotContains(t, payload.Documents[0].Content, "[Truncated]")
	}
}

func TestAsSearchTool_TripleIoC_MaxBytesFormatterValidator(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings("secret")}
	tool, err := AsSearchTool(r,
		WithMaxBytes(80),
		WithResultFormatter(func(docs []Document) (any, error) {
			return map[string]string{"tag": docs[0].Content}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]string)
			if !ok || payload["tag"] == "" {
				return errors.New("invalid tag")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var wire []byte
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), 80+len(textprocessor.TruncationSuffix)+2)
}

func TestAsSearchTool_WithMaxBytes_WithResultFormatter(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings("x")}
	tool, err := AsSearchTool(r,
		WithMaxBytes(60),
		WithResultFormatter(func(_ []Document) (any, error) {
			return map[string]string{"blob": strings.Repeat("z", 500)}, nil
		}),
	)
	require.NoError(t, err)
	var wire []byte
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), 60+len(textprocessor.TruncationSuffix)+2)
}

func TestAsSearchTool_FormatterAndValidator(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings("secret")}
	tool, err := AsSearchTool(r,
		WithResultFormatter(func(docs []Document) (any, error) {
			return map[string]string{"tag": docs[0].Content}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]string)
			if !ok {
				return errors.New("expected formatter output map")
			}
			if payload["tag"] == "" {
				return errors.New("empty tag")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var payload map[string]string
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(c toolsy.Chunk) error {
				return json.Unmarshal(c.Data, &payload)
			},
		),
	)
	require.Equal(t, "secret", payload["tag"])
}

func TestAsSearchTool_WithHostResultValidator_ShapeDocumentsJSON(t *testing.T) {
	r := &mockRetriever{docs: docsFromStrings("a")}
	tool, err := AsSearchTool(r,
		WithResultShape(ShapeDocumentsJSON),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(SearchDocumentsWire)
			if !ok {
				return errors.New("expected SearchDocumentsWire envelope")
			}
			if len(payload.Documents) != 1 {
				return errors.New("expected one document")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(toolsy.Chunk) error { return nil },
		),
	)
}

func TestScopeFilter_BeforeMaxResults(t *testing.T) {
	secret := make([]Document, 10)
	public := make([]Document, 5)
	for i := range secret {
		secret[i] = Document{Content: "secret"}
	}
	for i := range public {
		public[i] = Document{Content: "public"}
	}
	all := make([]Document, 0, len(secret)+len(public))
	all = append(all, secret...)
	all = append(all, public...)
	r := &mockRetriever{docs: all}
	tool, err := AsSearchTool(r,
		WithMaxResults(10),
		WithScopeFilter(func(_ context.Context, docs []Document) []Document {
			out := docs[:0]
			for _, d := range docs {
				if d.Content == "public" {
					out = append(out, d)
				}
			}
			return out
		}),
	)
	require.NoError(t, err)
	var result string
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(c toolsy.Chunk) error {
				result = decodeSearchMarkdown(t, c)
				return nil
			},
		),
	)
	require.Contains(t, result, "public")
	require.NotContains(t, result, "secret")
}

func TestAsSearchTool_MaxResultsZeroUnlimited(t *testing.T) {
	docs := docsFromStrings("a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k")
	r := &mockRetriever{docs: docs}
	tool, err := AsSearchTool(r, WithMaxResults(0))
	require.NoError(t, err)
	var result string
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(c toolsy.Chunk) error {
				result = decodeSearchMarkdown(t, c)
				return nil
			},
		),
	)
	for _, letter := range []string{"a", "k"} {
		require.Contains(t, result, letter)
	}
}
