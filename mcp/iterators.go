package mcp

import (
	"context"
	"errors"
	"iter"
)

const (
	defaultPaginationMaxPages       = 1000
	defaultPaginationMaxCursorBytes = 1024 * 1024
)

// PaginationLimits bounds discovery pagination controlled by an MCP server.
type PaginationLimits struct {
	MaxPages       int // MaxPages limits fetched pages; zero selects the default.
	MaxCursorBytes int // MaxCursorBytes limits cumulative cursor bytes; zero selects the default.
}

type paginationState struct {
	limits      PaginationLimits
	seen        map[string]struct{}
	pages       int
	cursorBytes int
}

func (l PaginationLimits) normalized() PaginationLimits {
	if l.MaxPages <= 0 {
		l.MaxPages = defaultPaginationMaxPages
	}
	if l.MaxCursorBytes <= 0 {
		l.MaxCursorBytes = defaultPaginationMaxCursorBytes
	}
	return l
}

// IterateCursor is a generic helper for MCP cursor-based pagination.
// fetch returns (items, nextCursor, error). Iteration stops on error or when nextCursor is empty.
func IterateCursor[T any](
	ctx context.Context,
	fetch func(ctx context.Context, cursor string) (items []T, nextCursor string, err error),
) iter.Seq2[T, error] {
	return IterateCursorWithLimits(ctx, PaginationLimits{
		MaxPages:       0,
		MaxCursorBytes: 0,
	}, fetch)
}

// IterateCursorWithLimits applies bounded page and cumulative cursor budgets.
func IterateCursorWithLimits[T any](
	ctx context.Context,
	limits PaginationLimits,
	fetch func(ctx context.Context, cursor string) (items []T, nextCursor string, err error),
) iter.Seq2[T, error] {
	limits = limits.normalized()
	return func(yield func(T, error) bool) {
		var cursor string
		state := paginationState{
			limits:      limits,
			seen:        make(map[string]struct{}),
			pages:       0,
			cursorBytes: 0,
		}
		for {
			if err := ctx.Err(); err != nil {
				yieldPaginationError(yield, err)
				return
			}
			items, nextCursor, err := fetch(ctx, cursor)
			state.pages++
			if err != nil {
				yieldPaginationError(yield, err)
				return
			}
			if !yieldPaginationItems(yield, items) {
				return
			}
			if nextCursor == "" {
				break
			}
			if err := state.acceptCursor(nextCursor); err != nil {
				yieldPaginationError(yield, err)
				return
			}
			cursor = nextCursor
		}
	}
}

func (s *paginationState) acceptCursor(cursor string) error {
	if s.pages >= s.limits.MaxPages {
		return &InvalidPayloadError{Subject: "pagination", Err: errors.New("page limit exceeded")}
	}
	if len(cursor) > s.limits.MaxCursorBytes-s.cursorBytes {
		return &InvalidPayloadError{
			Subject: "pagination cursor",
			Err:     errors.New("cumulative cursor byte limit exceeded"),
		}
	}
	if _, exists := s.seen[cursor]; exists {
		return &InvalidPayloadError{Subject: "pagination cursor", Err: errors.New("cursor cycle detected")}
	}
	s.cursorBytes += len(cursor)
	s.seen[cursor] = struct{}{}
	return nil
}

func yieldPaginationError[T any](yield func(T, error) bool, err error) {
	var zero T
	yield(zero, err)
}

func yieldPaginationItems[T any](yield func(T, error) bool, items []T) bool {
	for _, item := range items {
		if !yield(item, nil) {
			return false
		}
	}
	return true
}
