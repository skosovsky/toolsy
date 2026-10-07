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

// CacheEligibility explicitly decides whether the current authorized call may
// reuse a stored result. Idempotence/read-only hints are not freshness guarantees.
// The host callback must be concurrency-safe and is evaluated on every attempt.
type CacheEligibility func(context.Context, PreparedCall) (bool, error)

// ReplaySourceMetadata identifies library replay provenance. Reducers must not
// apply replayed effect declarations again; current call correlation is retained.
const ReplaySourceMetadata = "toolsy.replay_source"

const (
	// ReplaySourceCache denotes reuse of a result under current host freshness policy.
	ReplaySourceCache = "result_cache"
	// ReplaySourceOperation denotes delivery of an already completed logical operation.
	ReplaySourceOperation = "completed_operation"
)

const defaultCacheResultLimit = 1 << 20

// ResultCache is an optional post-authorization execution profile. It offers no
// atomic dispatch or remote exactly-once guarantee; use an operation journal for
// those semantics. A successful result is persisted before delivery.
type ResultCache struct {
	store       ResultCacheStore
	eligibility CacheEligibility
	partition   CachePartition
	codec       ResultCodec
	maxBytes    int
}

// NewResultCache requires explicit eligibility, a host partition and complete outcome codec.
// maxBytes bounds encoded storage and replay; zero selects a bounded default.
func NewResultCache(
	store ResultCacheStore,
	eligibility CacheEligibility,
	partition CachePartition,
	codec ResultCodec,
	maxBytes int,
) (*ResultCache, error) {
	if store == nil || eligibility == nil || partition == nil || codec == nil {
		return nil, errors.New("toolsy: result cache requires store, eligibility, partition and codec")
	}
	if maxBytes < 0 {
		return nil, errors.New("toolsy: result cache limit cannot be negative")
	}
	if maxBytes == 0 {
		maxBytes = defaultCacheResultLimit
	}
	return &ResultCache{
		store:       store,
		eligibility: eligibility,
		partition:   partition,
		codec:       codec,
		maxBytes:    maxBytes,
	}, nil
}

// ExecutePrepared caches exactly one successful result after argument binding.
func (c *ResultCache) ExecutePrepared(
	ctx context.Context,
	call PreparedCall,
	invoke InvocationHandler,
	yield func(Chunk) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	eligible, err := c.eligibility(ctx, clonePreparedCall(call))
	if err != nil {
		return NewInternalError(fmt.Errorf("result cache eligibility: %w", err))
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if !eligible {
		return invoke(yield)
	}
	key, err := c.key(ctx, call)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	raw, found, err := c.store.Get(ctx, key)
	if err != nil {
		return NewInternalError(fmt.Errorf("result cache read: %w", err))
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if found {
		return replayResult(ctx, call, raw, c.codec, c.maxBytes, ReplaySourceCache, yield)
	}
	capture := resultCacheCapture{ctx: ctx, maxBytes: c.maxBytes, yield: yield, result: nil, terminal: false, err: nil}
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
		if capture.terminal {
			return nil
		}
		return NewInternalError(errors.New("result cache requires a terminal result"))
	}
	raw, err = c.codec.EncodeResult(*capture.result)
	if err != nil {
		return NewInternalError(fmt.Errorf("result cache encode: %w", err))
	}
	if len(raw) > c.maxBytes {
		return NewInternalError(errors.New("result cache output limit exceeded"))
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = c.store.Put(ctx, key, raw); err != nil {
		return NewInternalError(fmt.Errorf("result cache write: %w", err))
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return yield(cloneResultChunk(*capture.result))
}

type resultCacheCapture struct {
	ctx      context.Context
	maxBytes int
	yield    func(Chunk) error
	result   *Chunk
	terminal bool
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
	if c.terminal {
		c.err = NewInternalError(errors.New("result cache requires exactly one terminal result"))
		return c.err
	}
	c.terminal = true
	switch {
	case chunk.IsError:
		c.err = c.yield(chunk)
	case len(chunk.Data) > c.maxBytes:
		c.err = NewInternalError(errors.New("result cache output limit exceeded"))
	default:
		copyChunk := cloneResultChunk(chunk)
		c.result = &copyChunk
	}
	return c.err
}

func (c *ResultCache) key(ctx context.Context, call PreparedCall) (string, error) {
	partition, err := c.partition(ctx, clonePreparedCall(call))
	if err != nil {
		return "", NewInternalError(fmt.Errorf("result cache partition: %w", err))
	}
	if partition == "" {
		return "", NewInternalError(errors.New("result cache partition is required"))
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

func replayResult(
	ctx context.Context,
	call PreparedCall,
	raw []byte,
	codec ResultCodec,
	maxBytes int,
	source string,
	yield func(Chunk) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(raw) > maxBytes {
		return NewInternalError(errors.New("stored replay output limit exceeded"))
	}
	chunk, err := codec.DecodeResult(append([]byte(nil), raw...))
	if err != nil {
		return NewInternalError(fmt.Errorf("stored replay decode: %w", err))
	}
	if chunk.Event != EventResult || chunk.IsError {
		return NewInternalError(errors.New("invalid stored replay result"))
	}
	chunk, err = prepareChunk(chunk)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	chunk.CallID, chunk.ToolName = call.Input.CallID, call.Manifest.Name
	envelope := chunk.ToolEnvelope()
	if envelope.Metadata == nil {
		envelope.Metadata = make(map[string]any)
	}
	envelope.Metadata[ReplaySourceMetadata] = source
	chunk.Envelope = &envelope
	chunk = markReplayChunk(chunk)
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
