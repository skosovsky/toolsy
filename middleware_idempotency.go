package toolsy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// IdempotencyStore caches prior successful tool results by idempotency key.
type IdempotencyStore interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Put(ctx context.Context, key string, result []byte) error
}

// WithIdempotency enables duplicate-call protection for tools marked Idempotent in manifest.
func WithIdempotency(store IdempotencyStore, keyFn func(ToolManifest, ToolInput) string) Middleware {
	if store == nil {
		panic("toolsy: WithIdempotency requires non-nil store")
	}
	if keyFn == nil {
		keyFn = defaultIdempotencyKey
	}
	return func(next Tool) Tool {
		return &idempotencyTool{
			toolBase: toolBase{next: next},
			store:    store,
			keyFn:    keyFn,
		}
	}
}

type idempotencyTool struct {
	toolBase

	store IdempotencyStore
	keyFn func(ToolManifest, ToolInput) string
}

func (t *idempotencyTool) Execute(
	ctx context.Context,
	run RunContext,
	input ToolInput,
	yield func(Chunk) error,
) error {
	manifest := t.next.Manifest()
	if !manifest.Idempotent {
		return t.next.Execute(ctx, run, input, yield)
	}
	key := t.keyFn(manifest, input)
	if cached, ok, err := t.store.Get(ctx, key); err != nil {
		return &SystemError{Err: fmt.Errorf("toolsy: idempotency get: %w", err)}
	} else if ok {
		return yield(Chunk{
			Event:    EventResult,
			Data:     cached,
			MimeType: MimeTypeJSON,
		})
	}

	var captured []byte
	err := t.next.Execute(ctx, run, input, func(c Chunk) error {
		if c.Event == EventResult && !c.IsError && len(c.Data) > 0 {
			captured = append([]byte(nil), c.Data...)
		}
		return yield(c)
	})
	if err != nil || len(captured) == 0 {
		return err
	}
	if putErr := t.store.Put(ctx, key, captured); putErr != nil {
		return &SystemError{Err: fmt.Errorf("toolsy: idempotency put: %w", putErr)}
	}
	return nil
}

func defaultIdempotencyKey(m ToolManifest, input ToolInput) string {
	h := sha256.Sum256(append([]byte(m.Name+":"), input.ArgsJSON...))
	return hex.EncodeToString(h[:])
}

// MemoryIdempotencyStore is an in-process IdempotencyStore for tests and single-node deployments.
type MemoryIdempotencyStore struct {
	mu    sync.RWMutex
	items map[string][]byte
}

func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{ //nolint:exhaustruct // mu zero value is valid
		items: make(map[string][]byte),
	}
}

func (s *MemoryIdempotencyStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[key]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), v...), true, nil
}

func (s *MemoryIdempotencyStore) Put(_ context.Context, key string, result []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = append([]byte(nil), result...)
	return nil
}
