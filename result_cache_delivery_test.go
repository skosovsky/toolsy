package toolsy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type observedResultStore struct {
	gets, puts       int
	raw              []byte
	found            bool
	getErr, putErr   error
	getHook, putHook func()
}

func (s *observedResultStore) Get(context.Context, string) ([]byte, bool, error) {
	s.gets++
	if s.getHook != nil {
		s.getHook()
	}
	return s.raw, s.found, s.getErr
}
func (s *observedResultStore) Put(_ context.Context, _ string, raw []byte) error {
	s.puts++
	if s.putHook != nil {
		s.putHook()
	}
	s.raw = append([]byte(nil), raw...)
	s.found = true
	return s.putErr
}

func TestResultCacheBusinessErrorDelivery(t *testing.T) {
	for _, path := range []string{"direct", "registry", "run-call"} {
		for _, eligible := range []bool{false, true} {
			t.Run(path+"/"+boolLabel(eligible), func(t *testing.T) {
				checkCacheBusinessDelivery(t, path, eligible)
			})
		}
	}
}

func checkCacheBusinessDelivery(t *testing.T, path string, eligible bool) {
	t.Helper()
	// Arrange: a business error is a terminal outcome, not cache corruption.
	store := &observedResultStore{}
	cache, err := NewResultCache(store, func(context.Context, PreparedCall) (bool, error) {
		return eligible, nil
	}, constantPartition, JSONResultCodec[string, string]{}, 0)
	require.NoError(t, err)
	business := NewBudgetExceededError("domain quota")
	terminal := NewErrorChunkFromErr(business)
	calls := 0
	tool, err := NewStreamTool(
		"business",
		"Business",
		func(_ context.Context, _ *RunEnv, _ struct{}, yield func(Chunk) error) error {
			calls++
			return yield(terminal)
		},
		WithIdempotent(),
		WithIndependentStream(),
	)
	require.NoError(t, err)
	registry, err := NewRegistryBuilder(WithExecutionProfile(cache)).Add(tool).Build()
	require.NoError(t, err)
	session, err := NewSession(registry)
	require.NoError(t, err)
	call := ToolCall{ToolName: "business", Input: ToolInput{ArgsJSON: []byte(`{}`)}}
	// Act: failures must neither populate the cache nor disappear on reuse.
	for range 2 {
		var chunks []Chunk
		yield := func(c Chunk) error { chunks = append(chunks, c); return nil }
		switch path {
		case "direct":
			err = tool.Execute(t.Context(), NewRunEnv(nil, WithRunExecutionProfile(cache)), call.Input, yield)
		case "registry":
			err = registry.Execute(t.Context(), call, yield)
		case "run-call":
			outcome, callErr := session.RunCall(t.Context(), call)
			err = callErr
			require.Equal(t, OutcomeBusinessError, outcome.Status)
			require.Equal(t, CodeBudgetExceeded, outcome.ExecutionError.Code)
		}
		// Assert: the no-cache delivery contract is preserved.
		require.NoError(t, err)
		if path != "run-call" {
			require.Len(t, chunks, 1)
			require.True(t, chunks[0].IsError)
			require.Equal(t, terminal.Data, chunks[0].Data)
			require.Equal(t, terminal.ToolEnvelope().Error, chunks[0].ToolEnvelope().Error)
		}
	}
	assert.Equal(t, 2, calls)
	assert.Zero(t, store.puts)
	if eligible {
		assert.Equal(t, 2, store.gets)
	} else {
		assert.Zero(t, store.gets)
	}
}

func boolLabel(v bool) string {
	if v {
		return "eligible"
	}
	return "ineligible"
}

func TestResultCacheEligibilityIsIndependentOfManifestHints(t *testing.T) {
	// Arrange: a host can approve stable reuse even without manifest hints.
	store := &observedResultStore{}
	eligible, checks, partitions, calls := false, 0, 0, 0
	cache, err := NewResultCache(store, func(_ context.Context, call PreparedCall) (bool, error) {
		checks++
		call.Input.ArgsJSON[0] = '!' // callback receives a detached snapshot.
		return eligible, nil
	}, func(context.Context, PreparedCall) (string, error) { partitions++; return "revision-1", nil }, JSONResultCodec[string, string]{}, 0)
	require.NoError(t, err)
	call := PreparedCall{
		Manifest: ToolManifest{Name: "stable", Idempotent: true, ReadOnly: true},
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}
	invoke := func(yield func(Chunk) error) error {
		calls++
		return yield(Chunk{Event: EventResult, Data: []byte(`"ok"`), MimeType: MimeTypeJSON, TypedResult: "ok"})
	}
	var received Chunk
	yield := func(c Chunk) error { received = c; return nil }
	// Act: hints cannot enable reuse; then host opts in for a tool without hints.
	for range 2 {
		require.NoError(t, cache.ExecutePrepared(t.Context(), call, invoke, yield))
	}
	assert.Zero(t, store.gets)
	assert.Zero(t, store.puts)
	assert.Zero(t, partitions)
	eligible = true
	call.Manifest.Idempotent, call.Manifest.ReadOnly = false, false
	for range 2 {
		require.NoError(t, cache.ExecutePrepared(t.Context(), call, invoke, yield))
	}
	// Assert: explicit current decision is authoritative and snapshot edits do not escape.
	assert.Equal(t, 4, checks)
	assert.Equal(t, 3, calls)
	assert.Equal(t, 2, partitions)
	assert.Equal(t, 1, store.puts)
	assert.Equal(t, ReplaySourceCache, received.ToolEnvelope().Metadata[ReplaySourceMetadata])
	assert.Equal(t, `{}`, string(call.Input.ArgsJSON))
}

func TestResultCacheInfrastructureFailuresAreNotInputRepair(t *testing.T) {
	sentinel := errors.New("host failure")
	for _, stage := range []string{"eligibility", "partition", "get", "put", "encode", "decode", "missing", "multiple", "wire-limit", "encoded-limit", "stored-limit", "invalid-stored"} {
		t.Run(stage, func(t *testing.T) {
			checkCacheInfrastructureFailure(t, stage, sentinel)
		})
	}
}

func checkCacheInfrastructureFailure(t *testing.T, stage string, sentinel error) {
	t.Helper()
	// Arrange: failures before/after dispatch must not grant argument correction.
	store := &observedResultStore{}
	eligibility := allowTestCacheReuse
	partition := constantPartition
	codec := ResultCodec(JSONResultCodec[string, string]{})
	limit := 0
	switch stage {
	case "eligibility":
		eligibility = func(context.Context, PreparedCall) (bool, error) { return false, sentinel }
	case "partition":
		partition = func(context.Context, PreparedCall) (string, error) { return "", sentinel }
	case "get":
		store.getErr = sentinel
	case "put":
		store.putErr = sentinel
	case "encode":
		codec = JSONResultCodec[int, string]{}
	case "decode":
		store.found, store.raw = true, []byte("!")
	case "stored-limit":
		store.found, store.raw, limit = true, []byte("long"), 1
	case "invalid-stored":
		store.found = true
		codec = fixedCacheCodec{chunk: NewErrorChunkFromErr(NewBudgetExceededError("quota"))}
	case "wire-limit", "encoded-limit":
		limit = 4
	}
	cache, err := NewResultCache(store, eligibility, partition, codec, limit)
	require.NoError(t, err)
	calls := 0
	call := PreparedCall{Manifest: ToolManifest{Name: "failure"}, Input: ToolInput{ArgsJSON: []byte(`{}`)}}
	// Act.
	err = cache.ExecutePrepared(t.Context(), call, func(yield func(Chunk) error) error {
		calls++
		if stage == "missing" {
			return nil
		}
		chunk := Chunk{Event: EventResult, Data: []byte(`"ok"`), MimeType: MimeTypeJSON, TypedResult: "ok"}
		if stage == "wire-limit" {
			chunk.Data = []byte(`"long"`)
		}
		if yieldErr := yield(chunk); yieldErr != nil {
			return yieldErr
		}
		if stage == "multiple" {
			return yield(chunk)
		}
		return nil
	}, func(Chunk) error { return nil })
	// Assert.
	require.Error(t, err)
	var toolErr *ToolError
	require.ErrorAs(t, err, &toolErr)
	assert.Equal(t, CodeInternal, toolErr.Code)
	assert.False(t, ClientCorrectable(toolErr.Code))
	assert.False(t, toolErr.Retryable)
	if stage == "eligibility" || stage == "partition" || stage == "get" || stage == "put" {
		require.ErrorIs(t, err, sentinel)
	}
	if stage == "eligibility" || stage == "partition" || stage == "get" || store.found && stage != "put" {
		assert.Zero(t, calls)
	}
}

type fixedCacheCodec struct{ chunk Chunk }

func (c fixedCacheCodec) EncodeResult(Chunk) ([]byte, error) { return []byte("stored"), nil }
func (c fixedCacheCodec) DecodeResult([]byte) (Chunk, error) { return c.chunk, nil }

type cancelResultCodec struct {
	ResultCodec

	encodeHook, decodeHook func()
}

func (c cancelResultCodec) EncodeResult(chunk Chunk) ([]byte, error) {
	if c.encodeHook != nil {
		c.encodeHook()
	}
	return c.ResultCodec.EncodeResult(chunk)
}
func (c cancelResultCodec) DecodeResult(raw []byte) (Chunk, error) {
	if c.decodeHook != nil {
		c.decodeHook()
	}
	return c.ResultCodec.DecodeResult(raw)
}

func TestResultCacheCallbackCancellationStopsFurtherWork(t *testing.T) {
	for _, stage := range []string{"eligibility", "partition", "get-miss", "get-hit", "decode", "encode", "put"} {
		t.Run(stage, func(t *testing.T) {
			// Arrange: a cooperative callback cancels but returns a successful value.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			store := &observedResultStore{}
			eligibility, partition := allowTestCacheReuse, constantPartition
			chunk := Chunk{Event: EventResult, Data: []byte(`"ok"`), MimeType: MimeTypeJSON, TypedResult: "ok"}
			codec := cancelResultCodec{ResultCodec: JSONResultCodec[string, string]{}}
			switch stage {
			case "eligibility":
				eligibility = func(context.Context, PreparedCall) (bool, error) { cancel(); return true, nil }
			case "partition":
				partition = func(context.Context, PreparedCall) (string, error) { cancel(); return "revision", nil }
			case "get-miss", "get-hit":
				store.getHook = cancel
			case "decode":
				codec.decodeHook = cancel
			case "encode":
				codec.encodeHook = cancel
			case "put":
				store.putHook = cancel
			}
			if stage == "get-hit" || stage == "decode" {
				var encodeErr error
				store.raw, encodeErr = codec.ResultCodec.EncodeResult(chunk)
				require.NoError(t, encodeErr)
				store.found = true
			}
			cache, err := NewResultCache(store, eligibility, partition, codec, 0)
			require.NoError(t, err)
			calls, deliveries := 0, 0
			// Act.
			err = cache.ExecutePrepared(
				ctx,
				PreparedCall{Manifest: ToolManifest{Name: "cancel"}, Input: ToolInput{ArgsJSON: []byte(`{}`)}},
				func(yield func(Chunk) error) error {
					calls++
					return yield(chunk)
				},
				func(Chunk) error { deliveries++; return nil },
			)
			// Assert: no dispatch after pre-handler cancel; no delivery after any cancel.
			require.ErrorIs(t, err, context.Canceled)
			assert.Zero(t, deliveries)
			if stage == "encode" || stage == "put" {
				assert.Equal(t, 1, calls)
			} else {
				assert.Zero(t, calls)
			}
			if stage == "put" {
				assert.Equal(t, 1, store.puts)
			} else {
				assert.Zero(t, store.puts)
			}
		})
	}
}

func TestResultCacheFailureTerminalAbortIsSticky(t *testing.T) {
	// Arrange: a producer ignores consumer abort and later declares success.
	store := &observedResultStore{}
	cache, err := NewResultCache(store, allowTestCacheReuse, constantPartition, JSONResultCodec[string, string]{}, 0)
	require.NoError(t, err)
	abort := errors.New("stop")
	deliveries := 0
	// Act.
	err = cache.ExecutePrepared(
		t.Context(),
		PreparedCall{Manifest: ToolManifest{Name: "abort"}, Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(yield func(Chunk) error) error {
			_ = yield(NewErrorChunkFromErr(NewBudgetExceededError("quota")))
			_ = yield(Chunk{Event: EventResult, Data: []byte(`"ok"`), MimeType: MimeTypeJSON})
			return nil
		},
		func(Chunk) error { deliveries++; return abort },
	)
	// Assert.
	require.ErrorIs(t, err, abort)
	assert.Equal(t, 1, deliveries)
	assert.Zero(t, store.puts)
}
