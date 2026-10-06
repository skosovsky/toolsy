package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/format"
)

type hostFetch func(context.Context, string) ([]Document, error)

func (f hostFetch) Retrieve(ctx context.Context, q string) ([]Document, error) { return f(ctx, q) }

func TestThinRetrieverNeverRetries(t *testing.T) {
	// Arrange.
	cause := errors.New("index unavailable")
	calls := 0
	tool, err := AsSearchTool(
		hostFetch(func(context.Context, string) ([]Document, error) { calls++; return nil, cause }),
	)
	require.NoError(t, err)
	// Act.
	err = tool.Execute(
		t.Context(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(toolsy.Chunk) error { t.Fatal("error must not yield"); return nil },
	)
	// Assert.
	require.Equal(t, 1, calls)
	require.ErrorIs(t, err, cause)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.False(t, te.Retryable)
}

func TestThinRetrieverCancellationAtCallbackBoundaries(t *testing.T) {
	for _, stage := range []string{"provider", "filter", "formatter", "validator"} {
		t.Run(stage, func(t *testing.T) {
			// Arrange.
			ctx, cancel := context.WithCancelCause(t.Context())
			cause := errors.New("host cancel")
			calls := make(map[string]int)
			stop := func(name string) {
				calls[name]++
				if stage == name {
					cancel(cause)
				}
			}
			tool, err := AsSearchTool(hostFetch(func(context.Context, string) ([]Document, error) {
				stop("provider")
				return []Document{{Content: "x"}}, nil
			}), WithScopeFilter(func(_ context.Context, docs []Document) []Document { stop("filter"); return docs }), WithResultFormatter(func(docs []Document) (any, error) {
				stop("formatter")
				return SearchDocumentsWire{Documents: docs}, nil
			}), WithHostResultValidator(func(any) error { stop("validator"); return nil }))
			require.NoError(t, err)
			// Act.
			err = tool.Execute(
				ctx,
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
				func(toolsy.Chunk) error { t.Fatal("cancelled must not yield"); return nil },
			)
			// Assert: stop is terminal, custom cause preserved and downstream callbacks skipped.
			require.ErrorIs(t, err, context.Canceled)
			require.ErrorIs(t, err, cause)
			for _, name := range []string{"provider", "filter", "formatter", "validator"} {
				require.LessOrEqual(t, calls[name], 1)
			}
			require.Equal(t, 1, calls[stage])
			if stage == "provider" {
				require.Zero(t, calls["filter"])
			}
			if stage == "provider" || stage == "filter" {
				require.Zero(t, calls["formatter"])
			}
			if stage != "validator" {
				require.Zero(t, calls["validator"])
			}
		})
	}
}

func TestFinalWireBoundaryAllShapes(t *testing.T) {
	for _, shape := range []ResultShape{ShapeMarkdown, ShapeDocumentsJSON} {
		for _, hostDTO := range []bool{false, true} {
			t.Run(fmt.Sprintf("shape=%d/custom=%t", shape, hostDTO), func(t *testing.T) {
				// Arrange: real encoded byte budget includes Unicode escaping and envelope.
				docs := []Document{{ID: "id", Content: "<界", SourceURI: "doc://1"}}
				o := options{resultShape: shape}
				o.applyDefaults()
				if hostDTO {
					o.resultFormatter = func([]Document) (any, error) { return map[string]string{"public": "<界"}, nil }
				}
				raw, err := encodeSearchResult(t.Context(), docs, &o)
				require.NoError(t, err)
				exact := len(raw)
				o.maxBytes = exact
				// Act/Assert: inclusive limit retains exact representation; +1 rejects whole output.
				same, err := encodeSearchResult(t.Context(), docs, &o)
				require.NoError(t, err)
				require.Equal(t, raw, same)
				require.True(t, json.Valid(same))
				o.maxBytes--
				rejected, err := encodeSearchResult(t.Context(), docs, &o)
				require.Nil(t, rejected)
				require.ErrorIs(t, err, toolsy.ErrValidation)
				var limit *format.WireLimitError
				require.ErrorAs(t, err, &limit)
				require.Equal(t, exact, limit.Size)
				require.Equal(t, exact-1, limit.Limit)
			})
		}
	}
}

func TestThinRetrieverRejectsNilPorts(t *testing.T) {
	for _, r := range []DocumentRetriever{nil, (*mockRetriever)(nil), hostFetch(nil)} {
		// Arrange/Act: no missing port may reach dispatch.
		tool, err := AsSearchTool(r)
		// Assert.
		require.Nil(t, tool)
		require.Error(t, err)
	}
}
