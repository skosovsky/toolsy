package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/toolkits/rag"
)

func empty(context.Context, string) ([]rag.Document, error) { return nil, nil }
func unavailablePolicy(docs []rag.Document, err error) bool {
	return errors.Is(err, errIndexUnavailable) || (err == nil && len(docs) == 0)
}

func TestHostRequiredProvidersAndPolicy(t *testing.T) {
	for _, cfg := range []knowledgeBase{
		{primary: nil, secondary: empty, supplemental: empty, allowFallback: unavailablePolicy},
		{primary: empty, secondary: nil, supplemental: empty, allowFallback: unavailablePolicy},
		{primary: empty, secondary: empty, supplemental: nil, allowFallback: unavailablePolicy},
		{primary: empty, secondary: empty, supplemental: empty, allowFallback: nil},
	} {
		// Arrange/Act.
		_, err := newKnowledgeBase(cfg.primary, cfg.secondary, cfg.supplemental, cfg.allowFallback)
		// Assert: error plus missing fallback cannot become nil,nil.
		require.Error(t, err)
	}
}

func TestHostFallbackIsExplicitAndCausal(t *testing.T) {
	for _, primaryErr := range []error{nil, errIndexUnavailable, errors.New("permission denied"), context.Canceled, context.DeadlineExceeded} {
		t.Run("primary="+errorName(primaryErr), func(t *testing.T) {
			// Arrange.
			secondaryErr := errors.New("secondary unavailable")
			secondaryCalls := 0
			retriever, err := newKnowledgeBase(
				func(context.Context, string) ([]rag.Document, error) { return nil, primaryErr },
				func(context.Context, string) ([]rag.Document, error) { secondaryCalls++; return nil, secondaryErr },
				empty,
				unavailablePolicy,
			)
			require.NoError(t, err)
			// Act.
			_, err = retriever.Retrieve(t.Context(), "query")
			// Assert.
			if primaryErr == nil || errors.Is(primaryErr, errIndexUnavailable) {
				require.Equal(t, 1, secondaryCalls)
				require.ErrorIs(t, err, secondaryErr)
				if primaryErr != nil {
					require.ErrorIs(t, err, primaryErr)
				}
			} else {
				require.Zero(t, secondaryCalls)
				require.ErrorIs(t, err, primaryErr)
			}
		})
	}
}

func errorName(err error) string {
	if err == nil {
		return "empty"
	}
	return err.Error()
}

func TestHostCancellationAndUnitIdentity(t *testing.T) {
	// Arrange: cancellation after a primary result is terminal despite permitted empty fallback.
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("host stopped")
	secondaryCalls := 0
	retriever, err := newKnowledgeBase(
		func(context.Context, string) ([]rag.Document, error) { cancel(cause); return nil, nil },
		func(context.Context, string) ([]rag.Document, error) { secondaryCalls++; return nil, nil },
		empty,
		unavailablePolicy,
	)
	require.NoError(t, err)
	// Act.
	_, err = retriever.Retrieve(ctx, "query")
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, cause)
	require.Zero(t, secondaryCalls)
	// Arrange: chunks in one source are distinct; unidentified units aren't discarded.
	docs := []rag.Document{
		{ID: "a", SourceURI: "doc://1"},
		{ID: "b", SourceURI: "doc://1"},
		{ID: "a", SourceURI: "doc://1"},
		{Content: "unknown"},
		{Content: "unknown"},
	}
	// Act.
	got := uniquePublicChunks(docs)
	// Assert.
	require.Len(t, got, 4)
	require.Equal(t, "a", got[0].ID)
	require.Equal(t, "b", got[1].ID)
	require.Equal(t, docs[3:], got[2:])
}

func TestHostPredicateCancellationIsTerminalForBothDecisions(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(fmt.Sprintf("allow=%t", allow), func(t *testing.T) {
			// Arrange: policy is a host callback and may synchronously cancel the parent.
			ctx, cancel := context.WithCancelCause(t.Context())
			cause := errors.New("policy stopped host")
			primaryErr := errors.New("primary unavailable")
			downstreamCalls := 0
			downstream := func(context.Context, string) ([]rag.Document, error) { downstreamCalls++; return nil, nil }
			r, err := newKnowledgeBase(
				func(context.Context, string) ([]rag.Document, error) { return nil, primaryErr },
				downstream,
				downstream,
				func([]rag.Document, error) bool { cancel(cause); return allow },
			)
			require.NoError(t, err)
			// Act.
			_, err = r.Retrieve(ctx, "query")
			// Assert: neither a false decision nor a failed primary can hide parent cancellation.
			require.ErrorIs(t, err, context.Canceled)
			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, primaryErr)
			require.Zero(t, downstreamCalls)
		})
	}
}
