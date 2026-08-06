package mcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIterateCursorWithLimits_RejectsUnboundedUniquePages(t *testing.T) {
	// Arrange.
	fetches := 0
	sequence := IterateCursorWithLimits(
		context.Background(),
		PaginationLimits{MaxPages: 3, MaxCursorBytes: 1024},
		func(context.Context, string) ([]int, string, error) {
			fetches++
			return nil, fmt.Sprintf("cursor-%d", fetches), nil
		},
	)

	// Act.
	var gotErr error
	for _, err := range sequence {
		gotErr = err
	}

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, gotErr, &invalid)
	require.Equal(t, 3, fetches)
}

func TestIterateCursorWithLimits_RejectsCumulativeCursorBytes(t *testing.T) {
	// Arrange.
	sequence := IterateCursorWithLimits(
		context.Background(),
		PaginationLimits{MaxPages: 10, MaxCursorBytes: 5},
		func(_ context.Context, cursor string) ([]int, string, error) {
			if cursor == "" {
				return nil, "abc", nil
			}
			return nil, "def", nil
		},
	)

	// Act.
	var gotErr error
	for _, err := range sequence {
		gotErr = err
	}

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, gotErr, &invalid)
	require.LessOrEqual(t, len(gotErr.Error()), maxDiagnosticBytes*2)
}
