package mcp

import (
	"context"
	"errors"
	"iter"
)

const (
	defaultPaginationMaxPages       = 1000
	defaultPaginationMaxCursorBytes = 1024 * 1024
	defaultPaginationMaxItems       = 10000
	defaultPaginationMaxBytes       = 16 * 1024 * 1024
)

// PaginationLimits bounds discovery pagination controlled by an MCP server.
type PaginationLimits struct {
	MaxPages       int // MaxPages limits fetched pages; zero selects the default.
	MaxItems       int // MaxItems bounds items in one complete discovery; zero selects 10000.
	MaxBytes       int // MaxBytes bounds aggregate raw tool-discovery response bytes; zero selects 16MiB.
	MaxCursorBytes int // MaxCursorBytes limits cumulative cursor bytes; zero selects the default.
}

type paginationState struct {
	limits      PaginationLimits
	seen        map[string]struct{}
	pages       int
	cursorBytes int
	items       int
}

func (l PaginationLimits) normalized() PaginationLimits {
	if l.MaxPages == 0 {
		l.MaxPages = defaultPaginationMaxPages
	}
	if l.MaxCursorBytes == 0 {
		l.MaxCursorBytes = defaultPaginationMaxCursorBytes
	}
	if l.MaxItems == 0 {
		l.MaxItems = defaultPaginationMaxItems
	}
	if l.MaxBytes == 0 {
		l.MaxBytes = defaultPaginationMaxBytes
	}
	return l
}

func (l PaginationLimits) validate() error {
	if l.MaxPages < 0 || l.MaxCursorBytes < 0 || l.MaxItems < 0 || l.MaxBytes < 0 {
		return errors.New("mcp: pagination limits cannot be negative")
	}
	return nil
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
		MaxItems:       0, MaxBytes: 0,
	}, fetch)
}

// IterateCursorWithLimits applies bounded page and cumulative cursor budgets.
func IterateCursorWithLimits[T any](
	ctx context.Context,
	limits PaginationLimits,
	fetch func(ctx context.Context, cursor string) (items []T, nextCursor string, err error),
) iter.Seq2[T, error] {
	if err := limits.validate(); err != nil {
		return errorSequence[T](err)
	}
	return iterateCursorWithValidatedLimits(ctx, limits.normalized(), fetch)
}

func iterateCursorWithValidatedLimits[T any](
	ctx context.Context,
	limits PaginationLimits,
	fetch func(context.Context, string) ([]T, string, error),
) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var cursor string
		state := paginationState{
			limits:      limits,
			seen:        make(map[string]struct{}),
			pages:       0,
			cursorBytes: 0,
			items:       0,
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
			if err := state.acceptItems(len(items)); err != nil {
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

func (s *paginationState) acceptItems(count int) error {
	if count > s.limits.MaxItems-s.items {
		return &InvalidPayloadError{Subject: "pagination items", Err: errors.New("aggregate item limit exceeded")}
	}
	s.items += count
	return nil
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
