package mcp

import (
	"context"
	"errors"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/toolsy"
)

// ImportConfig configures adapter admission and projection, not SDK transports.
type ImportConfig struct {
	MaxPages         int
	MaxTools         int
	MaxSnapshotBytes int
	MaxInFlight      int
	ProgressBuffer   int
	UpdateBuffer     int
	Classify         func(context.Context, *sdk.Tool) ([]toolsy.ToolOption, error)
}

// ClientHooks composes callbacks before Connect. Install and Bind are single-use
// for a client/session respectively; the host retains SDK connection ownership.
type ClientHooks struct {
	mu       sync.Mutex
	client   *sdk.Client
	config   ImportConfig
	sessions map[*sdk.ClientSession]*Importer
	seen     map[*sdk.ClientSession]bool
}

// ComposeClient preserves host handlers and installs manual MRTR on an owned
// options copy. Explicit automatic MRTR is incompatible with single attempts.
func ComposeClient(options *sdk.ClientOptions, config ImportConfig) (*ClientHooks, *sdk.ClientOptions, error) {
	if err := config.defaults(); err != nil {
		return nil, nil, err
	}
	var owned sdk.ClientOptions
	if options != nil {
		owned = *options
	}
	if owned.MultiRoundTrip != nil && !owned.MultiRoundTrip.Disabled {
		return nil, nil, errors.New("mcp adapter requires disabled automatic MRTR")
	}
	owned.MultiRoundTrip = &sdk.MultiRoundTripOptions{Disabled: true}
	h := &ClientHooks{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
		config:   config,
		sessions: make(map[*sdk.ClientSession]*Importer),
		seen:     make(map[*sdk.ClientSession]bool),
	}
	owned.ProgressNotificationHandler = composeCallback(
		h,
		owned.ProgressNotificationHandler,
		func(i *Importer, req *sdk.ProgressNotificationClientRequest) { i.progress(req.Params) },
	)
	owned.ToolListChangedHandler = composeCallback(
		h,
		owned.ToolListChangedHandler,
		func(i *Importer, _ *sdk.ToolListChangedRequest) { i.invalidate("tools", "") },
	)
	owned.ResourceListChangedHandler = composeCallback(
		h,
		owned.ResourceListChangedHandler,
		func(i *Importer, _ *sdk.ResourceListChangedRequest) { i.invalidate("resources", "") },
	)
	owned.PromptListChangedHandler = composeCallback(
		h,
		owned.PromptListChangedHandler,
		func(i *Importer, _ *sdk.PromptListChangedRequest) { i.invalidate("prompts", "") },
	)
	owned.ResourceUpdatedHandler = composeCallback(
		h,
		owned.ResourceUpdatedHandler,
		func(i *Importer, req *sdk.ResourceUpdatedNotificationRequest) {
			if req.Params != nil {
				i.invalidate("resource", req.Params.URI)
			}
		},
	)
	return h, &owned, nil
}

func composeCallback[P sdk.Params](
	h *ClientHooks,
	host func(context.Context, *sdk.ClientRequest[P]),
	adapter func(*Importer, *sdk.ClientRequest[P]),
) func(context.Context, *sdk.ClientRequest[P]) {
	return func(ctx context.Context, req *sdk.ClientRequest[P]) {
		if i := h.lookup(req.Session); i != nil {
			adapter(i, req)
		}
		if host != nil {
			host(ctx, req)
		}
	}
}

const defaultSnapshotBytes = 8 << 20
const toolCallMethod = "tools/call"

func (c *ImportConfig) defaults() error {
	if c.MaxPages == 0 {
		c.MaxPages = 128
	}
	if c.MaxTools == 0 {
		c.MaxTools = 10000
	}
	if c.MaxSnapshotBytes == 0 {
		c.MaxSnapshotBytes = defaultSnapshotBytes
	}
	if c.MaxInFlight == 0 {
		c.MaxInFlight = 32
	}
	if c.ProgressBuffer == 0 {
		c.ProgressBuffer = 64
	}
	if c.UpdateBuffer == 0 {
		c.UpdateBuffer = 64
	}
	if c.MaxPages < 1 || c.MaxTools < 1 || c.MaxSnapshotBytes < 1 || c.MaxInFlight < 1 ||
		c.ProgressBuffer < 1 || c.UpdateBuffer < 1 {
		return errors.New("mcp adapter limits must be positive")
	}
	return nil
}

// Install must run before the first Connect; SDK middleware handles terminal
// retirement without wrapping Connection (which would lose sessionUpdated).
func (h *ClientHooks) Install(client *sdk.Client) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if client == nil || h.client != nil {
		return errors.New("mcp hooks require one client")
	}
	h.client = client
	client.AddSendingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			if method == "server/discover" || method == "initialize" {
				h.observe(req)
			}
			result, err := next(ctx, method, req)
			if method == toolCallMethod {
				h.retireRequest(req)
			}
			return result, err
		}
	})
	return nil
}

func (h *ClientHooks) observe(req sdk.Request) {
	session, ok := req.GetSession().(*sdk.ClientSession)
	if !ok {
		return
	}
	h.mu.Lock()
	if h.seen[session] {
		h.mu.Unlock()
		return
	}
	h.seen[session] = true
	h.mu.Unlock()
	// SDK Wait has no cancellable variant. Observe every connection, including
	// failed/unused handshakes that the host never binds, until host disposal.
	go func() {
		err := session.Wait()
		h.mu.Lock()
		importer := h.sessions[session]
		delete(h.sessions, session)
		delete(h.seen, session)
		h.mu.Unlock()
		if importer != nil {
			importer.closeWith(err)
		}
	}()
}
func (h *ClientHooks) retireRequest(req sdk.Request) {
	session, ok := req.GetSession().(*sdk.ClientSession)
	if !ok {
		return
	}
	params, ok := req.GetParams().(interface{ GetProgressToken() any })
	if !ok {
		return
	}
	if i := h.lookup(session); i != nil {
		i.retire(params.GetProgressToken())
	}
}

func (h *ClientHooks) lookup(session *sdk.ClientSession) *Importer {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[session]
}

// Bind attaches an authenticated authority identity, never inferred from a
// remote cacheScope or metadata. Changing identity requires a new binding.
func (h *ClientHooks) Bind(session *sdk.ClientSession, identity string) (*Importer, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.client == nil || session == nil || identity == "" || h.sessions[session] != nil || !h.seen[session] {
		return nil, errors.New("mcp binding requires installed hooks, session and unique authority identity")
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	i := &Importer{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
		session:   session,
		identity:  identity,
		config:    h.config,
		ctx:       ctx,
		cancel:    cancel,
		updates:   make(chan Invalidation, h.config.UpdateBuffer),
		admission: make(chan struct{}, h.config.MaxInFlight),
		calls:     make(map[string]*progressCall),
	}
	h.sessions[session] = i
	return i, nil
}
