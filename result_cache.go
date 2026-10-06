package toolsy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// ResultCacheStore persists encoded successful results, not operation intents.
type ResultCacheStore interface {
	Get(context.Context, string) ([]byte, bool, error)
	Put(context.Context, string, []byte) error
}

// ResultCodec preserves a complete result including host-owned typed values.
// It must reject values it cannot round-trip without losing delivery semantics.
type ResultCodec interface {
	EncodeResult(Chunk) ([]byte, error)
	DecodeResult([]byte) (Chunk, error)
}

// CachePartition supplies a trusted scope/data partition after current policy.
// Include domain identity and dependency freshness affecting the result. Toolsy
// additionally binds the key to canonical input, attachments and manifest/view.
type CachePartition func(context.Context, PreparedCall) (string, error)

// CacheReplayMetadata marks cached declarations for host reducers. Replayed
// effects must not be applied as new effects; call correlation is still current.
const CacheReplayMetadata = "toolsy.cache_replay"

const defaultCacheResultLimit = 1 << 20

// ResultCache is an optional post-authorization execution profile. It offers no
// atomic dispatch or remote exactly-once guarantee; use an operation journal for
// those semantics. A successful result is persisted before delivery.
type ResultCache struct {
	store     ResultCacheStore
	partition CachePartition
	codec     ResultCodec
	maxBytes  int
}

// NewResultCache requires an explicit host partition and complete outcome codec.
// maxBytes bounds encoded storage and replay; zero selects a bounded default.
func NewResultCache(
	store ResultCacheStore,
	partition CachePartition,
	codec ResultCodec,
	maxBytes int,
) (*ResultCache, error) {
	if store == nil || partition == nil || codec == nil {
		return nil, errors.New("toolsy: result cache requires store, partition and codec")
	}
	if maxBytes < 0 {
		return nil, errors.New("toolsy: result cache limit cannot be negative")
	}
	if maxBytes == 0 {
		maxBytes = defaultCacheResultLimit
	}
	return &ResultCache{store: store, partition: partition, codec: codec, maxBytes: maxBytes}, nil
}

// ExecutePrepared caches exactly one successful result after argument binding.
func (c *ResultCache) ExecutePrepared(
	ctx context.Context,
	call PreparedCall,
	invoke InvocationHandler,
	yield func(Chunk) error,
) error {
	if !call.Manifest.Idempotent {
		return invoke(yield)
	}
	key, err := c.key(ctx, call)
	if err != nil {
		return err
	}
	raw, found, err := c.store.Get(ctx, key)
	if err != nil {
		return NewInternalError(fmt.Errorf("result cache read: %w", err))
	}
	if found {
		return c.replay(call, raw, yield)
	}
	capture := resultCacheCapture{ctx: ctx, maxBytes: c.maxBytes, yield: yield, result: nil, err: nil}
	err = invoke(capture.accept)
	if err != nil {
		return err
	}
	if capture.err != nil {
		return capture.err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if capture.result == nil {
		return NewValidationError("result cache requires a successful terminal result")
	}
	raw, err = c.codec.EncodeResult(*capture.result)
	if err != nil {
		return NewInternalError(fmt.Errorf("result cache encode: %w", err))
	}
	if len(raw) > c.maxBytes {
		return NewValidationError("result cache output limit exceeded")
	}
	if err = c.store.Put(ctx, key, raw); err != nil {
		return NewInternalError(fmt.Errorf("result cache write: %w", err))
	}
	return yield(cloneResultChunk(*capture.result))
}

type resultCacheCapture struct {
	ctx      context.Context
	maxBytes int
	yield    func(Chunk) error
	result   *Chunk
	err      error
}

func (c *resultCacheCapture) accept(chunk Chunk) error {
	if c.err != nil {
		return c.err
	}
	if c.err = c.ctx.Err(); c.err != nil {
		return c.err
	}
	if chunk.Event != EventResult {
		c.err = c.yield(chunk)
		if c.err == nil && chunk.Event == EventControl {
			c.err = ControlErrorFromSignal(chunk.Control)
		}
		return c.err
	}
	switch {
	case chunk.IsError:
		c.err = NewValidationError("unsuccessful result is not cacheable")
	case c.result != nil:
		c.err = NewValidationError("result cache requires exactly one successful result")
	case len(chunk.Data) > c.maxBytes:
		c.err = NewValidationError("result cache output limit exceeded")
	default:
		copyChunk := cloneResultChunk(chunk)
		c.result = &copyChunk
	}
	return c.err
}

func (c *ResultCache) key(ctx context.Context, call PreparedCall) (string, error) {
	partition, err := c.partition(ctx, clonePreparedCall(call))
	if err != nil {
		return "", err
	}
	if partition == "" {
		return "", NewPolicyDeniedError("result cache partition is required", "cache_partition")
	}
	manifestHash := sha256.New()
	if digestErr := writeManifestDigest(manifestHash, call.Manifest); digestErr != nil {
		return "", NewInternalError(digestErr)
	}
	attachments := make([]cacheAttachment, len(call.Input.Attachments))
	for i, attachment := range call.Input.Attachments {
		attachments[i] = cacheAttachment{MIME: attachment.MimeType, Data: attachment.Data}
	}
	keyData, err := json.Marshal(struct {
		Partition   string               `json:"partition"`
		Manifest    string               `json:"manifest"`
		Args        json.RawMessage      `json:"args"`
		Attachments []cacheAttachment    `json:"attachments"`
		View        RegistryViewSnapshot `json:"view"`
	}{partition, hex.EncodeToString(manifestHash.Sum(nil)), call.Input.ArgsJSON, attachments, call.View})
	if err != nil {
		return "", NewInternalError(err)
	}
	digest := sha256.Sum256(keyData)
	return hex.EncodeToString(digest[:]), nil
}

type cacheAttachment struct {
	MIME string `json:"mime"`
	Data []byte `json:"data"`
}

func (c *ResultCache) replay(call PreparedCall, raw []byte, yield func(Chunk) error) error {
	if len(raw) > c.maxBytes {
		return NewValidationError("result cache output limit exceeded")
	}
	chunk, err := c.codec.DecodeResult(append([]byte(nil), raw...))
	if err != nil {
		return NewInternalError(fmt.Errorf("result cache decode: %w", err))
	}
	if chunk.Event != EventResult || chunk.IsError {
		return NewValidationError("invalid cached result")
	}
	chunk, err = prepareChunk(chunk)
	if err != nil {
		return err
	}
	chunk.CallID, chunk.ToolName = call.Input.CallID, call.Manifest.Name
	envelope := chunk.ToolEnvelope()
	if envelope.Metadata == nil {
		envelope.Metadata = make(map[string]any)
	}
	envelope.Metadata[CacheReplayMetadata] = true
	chunk.Envelope = &envelope
	return yield(chunk)
}

func cloneResultChunk(chunk Chunk) Chunk {
	chunk.Data = append([]byte(nil), chunk.Data...)
	chunk.TypedResult = deepCloneValue(chunk.TypedResult)
	chunk.Effects = cloneAnyValues(chunk.Effects)
	chunk.Controls = append([]ControlSignal(nil), chunk.Controls...)
	for i, control := range chunk.Controls {
		if control != nil {
			cloned, ok := cloneMutableValue(control).(ControlSignal)
			if ok {
				chunk.Controls[i] = cloned
			}
		}
	}
	if chunk.Envelope != nil {
		chunk.Envelope = cloneToolEnvelope(chunk.Envelope)
	}
	return chunk
}

func cloneAnyValues(values []any) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = deepCloneValue(value)
	}
	return out
}

// MemoryResultCacheStore is a bounded-by-caller process-local test cache. It is
// not durable and does not coordinate dispatch; hosts own eviction/expiry policy.
type MemoryResultCacheStore struct {
	mu    sync.RWMutex
	items map[string][]byte
}

// NewMemoryResultCacheStore creates a process-local result store.
func NewMemoryResultCacheStore() *MemoryResultCacheStore {
	return &MemoryResultCacheStore{mu: sync.RWMutex{}, items: make(map[string][]byte)}
}

func (s *MemoryResultCacheStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, found := s.items[key]
	return append([]byte(nil), raw...), found, nil
}

func (s *MemoryResultCacheStore) Put(_ context.Context, key string, raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = append([]byte(nil), raw...)
	return nil
}
