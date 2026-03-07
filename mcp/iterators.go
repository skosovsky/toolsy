package mcp

import (
	"context"
	"iter"
)

// IterateCursor is a generic helper for MCP cursor-based pagination.
// fetch returns (items, nextCursor, error). Iteration stops on error or when nextCursor is empty.
func IterateCursor[T any](
	ctx context.Context,
	fetch func(ctx context.Context, cursor string) (items []T, nextCursor string, err error),
) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var cursor string
		for {
			items, nextCursor, err := fetch(ctx, cursor)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, item := range items {
				if !yield(item, nil) {
					return
				}
			}
			if nextCursor == "" {
				break
			}
			cursor = nextCursor
		}
	}
}
