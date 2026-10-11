package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

var (
	ErrClosed      = errors.New("mcp adapter closed")
	ErrStale       = errors.New("mcp snapshot revoked")
	ErrAdmission   = errors.New("mcp adapter admission limit")
	ErrQueueFull   = errors.New("mcp adapter notification queue full")
	ErrUnsupported = errors.New("mcp adapter unsupported projection or capability")
)

// BridgeError preserves the underlying SDK/core cause without logging its
// potentially secret-bearing message. No bridge error authorizes redispatch.
type BridgeError struct {
	Kind  string
	Cause error
}

func (e *BridgeError) Error() string                  { return "mcp adapter: " + e.Kind }
func (e *BridgeError) Unwrap() error                  { return e.Cause }
func (e *BridgeError) Format(state fmt.State, _ rune) { _, _ = fmt.Fprint(state, e.Error()) }

// NeedsInputError is a single-attempt intermediate result. Only the host may
// continue with WithRound; RequestState and answers are not approval grants.
type NeedsInputError struct {
	Method string
	Result sdk.Result
}

func (*NeedsInputError) Error() string                    { return "mcp adapter: input required" }
func (e *NeedsInputError) Format(state fmt.State, _ rune) { _, _ = fmt.Fprint(state, e.Error()) }

// Round carries host-controlled MRTR continuation data in a call context.
type Round struct {
	RequestState   string               `json:"requestState,omitempty"`
	InputResponses sdk.InputResponseMap `json:"inputResponses,omitempty"`
}
type roundKey struct{}

// WithRound snapshots continuation responses; model arguments cannot set them.
func WithRound(ctx context.Context, round Round) (context.Context, error) {
	owned, err := cloneRound(round)
	if err != nil {
		return nil, &BridgeError{Kind: "round", Cause: err}
	}
	return context.WithValue(ctx, roundKey{}, owned), nil
}

// cloneRound refuses host values whose SDK DTO ownership round-trip changes
// their serialized representation. Unlike inbound SDK loss, this is detectable
// before dispatch. Do not silently round host int64/json.Number/raw payloads.
func cloneRound(round Round) (Round, error) {
	original, err := json.Marshal(round)
	if err != nil {
		return Round{}, err
	}
	owned, err := cloneSDK(round)
	if err != nil {
		return Round{}, err
	}
	copied, err := json.Marshal(owned)
	if err != nil {
		return Round{}, err
	}
	if !bytes.Equal(original, copied) {
		return Round{}, &BridgeError{Kind: "round representation unsupported by SDK ownership", Cause: ErrUnsupported}
	}
	return owned, nil
}

// Invalidation is an adapter-local generation change, not an SDK subscription ACK.
type Invalidation struct {
	Kind       string
	URI        string
	Generation uint64
}

// ToolSnapshot owns descriptors independently of SDK cache and executable tools.
type ToolSnapshot struct {
	Identity    string
	Generation  uint64
	Digest      string
	Descriptors []*sdk.Tool
	Tools       []toolsy.Tool
}

// Importer borrows its SDK session. Close never closes that shared connection.
type Importer struct {
	session    *sdk.ClientSession
	identity   string
	config     ImportConfig
	ctx        context.Context
	cancel     context.CancelCauseFunc
	mu         sync.Mutex
	closed     bool
	generation uint64
	updates    chan Invalidation
	admission  chan struct{}
	calls      map[string]*progressCall
	sequence   atomic.Uint64
}

func (i *Importer) Close() error { i.closeWith(ErrClosed); return nil }
func (i *Importer) closeWith(cause error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return
	}
	i.closed = true
	i.generation++
	if cause == nil {
		cause = ErrClosed
	}
	i.cancel(cause)
	close(i.updates)
}

// Updates reports bounded local invalidations. Overflow closes the importer
// fail-closed instead of silently losing revocation. Host SDK handlers still run.
func (i *Importer) Updates() <-chan Invalidation { return i.updates }
func (i *Importer) Err() error                   { return context.Cause(i.ctx) }
func (i *Importer) Done() <-chan struct{}        { return i.ctx.Done() }

func (i *Importer) invalidate(kind, uri string) {
	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return
	}
	i.generation++
	event := Invalidation{Kind: kind, URI: uri, Generation: i.generation}
	select {
	case i.updates <- event:
		i.mu.Unlock()
	default:
		i.mu.Unlock()
		i.closeWith(ErrQueueFull)
	}
}

func (i *Importer) current() (uint64, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return i.generation, &BridgeError{Kind: "closed", Cause: context.Cause(i.ctx)}
	}
	return i.generation, nil
}

func (i *Importer) check(generation uint64) error {
	current, err := i.current()
	if err != nil {
		return err
	}
	if current != generation {
		return &BridgeError{Kind: "stale", Cause: ErrStale}
	}
	return nil
}

// Discover builds a transactional, bounded catalog. SDK TTL remains effective;
// v1.8.0 offers no public cache bypass. Calling Discover is not a force refresh.
func (i *Importer) Discover(ctx context.Context) (ToolSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return ToolSnapshot{}, &BridgeError{Kind: "discovery cancelled", Cause: err}
	}
	start, err := i.current()
	if err != nil {
		return ToolSnapshot{}, err
	}
	ctx, cancel := i.child(ctx)
	defer cancel()
	descriptors, err := i.fetchCatalog(ctx)
	if err != nil {
		return ToolSnapshot{}, err
	}
	var tools []toolsy.Tool
	for _, descriptor := range descriptors {
		tool, buildErr := i.proxy(ctx, descriptor, start+1)
		if buildErr != nil {
			return ToolSnapshot{}, &BridgeError{Kind: "tool construction", Cause: buildErr}
		}
		tools = append(tools, tool)
	}
	raw, err := json.Marshal(descriptors)
	if err != nil {
		return ToolSnapshot{}, &BridgeError{Kind: "snapshot", Cause: err}
	}
	domain, _ := json.Marshal([]any{"toolsy/mcp/sdk-snapshot/v1", i.identity, start + 1, "tools/list"})
	digest := sha256.Sum256(append(domain, raw...))
	i.mu.Lock()
	if i.closed || i.generation != start {
		i.mu.Unlock()
		return ToolSnapshot{}, ErrStale
	}
	if err := ctx.Err(); err != nil {
		i.mu.Unlock()
		return ToolSnapshot{}, &BridgeError{Kind: "discovery cancelled", Cause: err}
	}
	i.generation++
	i.mu.Unlock()
	return ToolSnapshot{
		Identity:    i.identity,
		Generation:  start + 1,
		Digest:      hex.EncodeToString(digest[:]),
		Descriptors: descriptors,
		Tools:       tools,
	}, nil
}

//nolint:gocognit // The aggregate catalog budget and duplicate/cursor checks form a single transaction.
func (i *Importer) fetchCatalog(ctx context.Context) ([]*sdk.Tool, error) {
	if result := i.session.InitializeResult(); result == nil || result.Capabilities == nil ||
		result.Capabilities.Tools == nil {
		return nil, &BridgeError{Kind: "tools capability", Cause: ErrUnsupported}
	}
	cursor := ""
	seenCursors := map[string]bool{}
	seenNames := map[string]bool{}
	var descriptors []*sdk.Tool
	total := 0
	for page := 0; ; page++ {
		if page >= i.config.MaxPages || seenCursors[cursor] {
			return nil, &BridgeError{Kind: "pagination", Cause: ErrAdmission}
		}
		seenCursors[cursor] = true
		result, listErr := i.session.ListTools(
			ctx,
			&sdk.ListToolsParams{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
				Cursor: cursor,
			},
		)
		if listErr != nil {
			return nil, &BridgeError{Kind: "discovery", Cause: listErr}
		}
		if result == nil {
			return nil, &BridgeError{Kind: "nil discovery", Cause: ErrUnsupported}
		}
		raw, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return nil, &BridgeError{Kind: "descriptor", Cause: marshalErr}
		}
		total += len(raw)
		if total > i.config.MaxSnapshotBytes || len(descriptors)+len(result.Tools) > i.config.MaxTools {
			return nil, &BridgeError{Kind: "catalog limit", Cause: ErrAdmission}
		}
		for _, descriptor := range result.Tools {
			if descriptor == nil || descriptor.Name == "" || seenNames[descriptor.Name] {
				return nil, &BridgeError{Kind: "duplicate or invalid tool", Cause: ErrUnsupported}
			}
			seenNames[descriptor.Name] = true
			owned, copyErr := cloneSDK(descriptor)
			if copyErr != nil {
				return nil, &BridgeError{Kind: "descriptor", Cause: copyErr}
			}
			descriptors = append(descriptors, owned)
		}
		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}
	return descriptors, nil
}

func (i *Importer) proxy(ctx context.Context, descriptor *sdk.Tool, generation uint64) (toolsy.Tool, error) {
	if i.config.Classify == nil {
		return nil, errors.New("mcp imported tools require explicit host classification")
	}
	classification, err := cloneSDK(descriptor)
	if err != nil {
		return nil, err
	}
	options, err := i.config.Classify(ctx, classification)
	if err != nil {
		return nil, err
	}
	schema, err := json.Marshal(descriptor.InputSchema)
	if err != nil {
		return nil, err
	}
	if descriptor.OutputSchema != nil {
		raw, outputErr := json.Marshal(descriptor.OutputSchema)
		if outputErr != nil {
			return nil, outputErr
		}
		decoded, outputErr := jsonschemax.Decode(raw)
		if outputErr != nil {
			return nil, outputErr
		}
		object, ok := decoded.(map[string]any)
		if !ok {
			return nil, ErrUnsupported
		}
		options = append(options, toolsy.WithOutputSchema(object))
	}
	name := descriptor.Name
	hasSchema := descriptor.OutputSchema != nil
	base, err := toolsy.NewProxyTool(name, descriptor.Description, schema,
		func(ctx context.Context, _ *toolsy.RunEnv, args []byte, yield func(toolsy.Chunk) error) error {
			if staleErr := i.check(generation); staleErr != nil {
				return staleErr
			}
			return i.call(ctx, generation, name, args, hasSchema, yield)
		}, options...)
	if err != nil {
		return nil, err
	}
	return &importTool{Tool: base, importer: i, generation: generation}, nil
}

type importTool struct {
	toolsy.Tool

	importer   *Importer
	generation uint64
}

func (*importTool) SupportsPreparedExecution() bool { return true }

func (t *importTool) Execute(
	ctx context.Context,
	env *toolsy.RunEnv,
	input toolsy.ToolInput,
	yield func(toolsy.Chunk) error,
) error {
	if len(input.Attachments) != 0 {
		return &BridgeError{Kind: "import attachments", Cause: ErrUnsupported}
	}
	if err := t.importer.check(t.generation); err != nil {
		return err
	}
	return t.Tool.Execute(ctx, env, input, func(chunk toolsy.Chunk) error {
		if err := t.importer.check(t.generation); err != nil {
			return err
		}
		return yield(chunk)
	})
}

func (i *Importer) child(ctx context.Context) (context.Context, context.CancelFunc) {
	child, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(i.ctx, func() { cancel(context.Cause(i.ctx)) })
	return child, func() { stop(); cancel(context.Canceled) }
}

// cloneSDK copies SDK-visible data only. v1.8.0 already loses exact numbers and
// null/presence before this boundary; copying cannot repair that loss.
// https://github.com/modelcontextprotocol/go-sdk/issues/1236
// https://github.com/modelcontextprotocol/go-sdk/issues/1310
func cloneSDK[T any](value T) (T, error) {
	var owned T
	raw, err := json.Marshal(value)
	if err == nil {
		err = json.Unmarshal(raw, &owned)
	}
	return owned, err
}

// ResourceTool preserves the resource-as-tool capability without another client.
// Prompts, templates, subscriptions and manual resource/prompt MRTR use host SDK
// session APIs directly; their wire lifecycle is entirely SDK-owned.
//
//nolint:gocognit // Resource capability, owned continuation and prepared execution form one constructor.
func (i *Importer) ResourceTool() (toolsy.Tool, error) {
	if result := i.session.InitializeResult(); result == nil || result.Capabilities == nil ||
		result.Capabilities.Resources == nil {
		return nil, &BridgeError{Kind: "resource capability", Cause: ErrUnsupported}
	}
	generation, err := i.current()
	if err != nil {
		return nil, err
	}
	base, err := toolsy.NewProxyTool(
		"mcp_read_resource",
		"Read an MCP resource",
		[]byte(
			`{"type":"object","properties":{"uri":{"type":"string"}},"required":["uri"],"additionalProperties":false}`,
		),
		func(ctx context.Context, _ *toolsy.RunEnv, args []byte, yield func(toolsy.Chunk) error) error {
			release, admissionErr := i.admit()
			if admissionErr != nil {
				return admissionErr
			}
			defer release()
			var params struct {
				URI string `json:"uri"`
			}
			if decodeErr := json.Unmarshal(args, &params); decodeErr != nil {
				return decodeErr
			}
			ctx, cancel := i.child(ctx)
			defer cancel()
			request := &sdk.ReadResourceParams{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
				URI: params.URI,
			}
			if round, ok := ctx.Value(roundKey{}).(Round); ok {
				owned, copyErr := cloneRound(round)
				if copyErr != nil {
					return copyErr
				}
				request.RequestState, request.InputResponses = owned.RequestState, owned.InputResponses
			}
			if staleErr := i.check(generation); staleErr != nil {
				return staleErr
			}
			result, readErr := i.session.ReadResource(ctx, request)
			if readErr != nil {
				return &BridgeError{Kind: "resource", Cause: readErr}
			}
			if result == nil {
				return ErrUnsupported
			}
			owned, copyErr := cloneSDK(result)
			if copyErr != nil {
				return copyErr
			}
			if owned.NeedsInput() {
				return &NeedsInputError{Method: "resources/read", Result: owned}
			}
			raw, encodeErr := json.Marshal(owned)
			if encodeErr != nil {
				return encodeErr
			}
			return yield(
				toolsy.Chunk{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
					Event:       toolsy.EventResult,
					MimeType:    toolsy.MimeTypeJSON,
					Data:        bytes.Clone(raw),
					TypedResult: owned,
				},
			)
		},
		toolsy.WithReadOnly(),
	)
	if err != nil {
		return nil, err
	}
	return &importTool{Tool: base, importer: i, generation: generation}, nil
}
