package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/skosovsky/toolsy"
)

// oauthFixture is a synthetic IdP, not an adapter auth implementation. The SDK
// owns challenge parsing, discovery, PKCE, exchange, refresh and bearer checks.
type oauthFixture struct {
	host      *httptest.Server
	calls     atomic.Int32
	metadata  atomic.Int32
	exchanges atomic.Int32
	refreshes atomic.Int32
	consent   atomic.Int32
	persisted atomic.Int32
	cancelled atomic.Bool
	mode      string
}

func newOAuthFixture(t *testing.T, mode string) *oauthFixture {
	t.Helper()
	f := &oauthFixture{mode: mode}
	servers := make(map[string]*sdk.Server)
	for _, principal := range []string{"alice", "bob"} {
		local, err := toolsy.NewProxyTool(principal, "scoped echo", []byte(objectSchema),
			func(_ context.Context, env *toolsy.RunEnv, args []byte, yield func(toolsy.Chunk) error) error {
				assert.Equal(t, principal, env.CallContext().Subject)
				assert.Equal(t, "tenant/"+principal, env.CallContext().Scope)
				f.calls.Add(1)
				return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: args, MimeType: toolsy.MimeTypeJSON})
			}, toolsy.WithReadOnly())
		require.NoError(t, err)
		registry, err := toolsy.NewRegistry(local)
		require.NoError(t, err)
		view, err := registry.View(
			toolsy.RegistryViewSpec{ToolNames: []string{principal}, Owner: "tenant/" + principal},
		)
		require.NoError(t, err)
		servers[principal], err = Export(view, ExportConfig{
			Names: []string{principal}, Implementation: &sdk.Implementation{Name: principal, Version: "1"},
			Resolve: func(ctx context.Context, _ sdk.Request) (toolsy.CallContext, error) {
				info := auth.TokenInfoFromContext(ctx)
				if info == nil || info.UserID != principal {
					return toolsy.CallContext{}, auth.ErrInvalidToken
				}
				return toolsy.NewCallContext(info.UserID, "tenant/"+info.UserID), nil
			},
		})
		require.NoError(t, err)
	}
	mux := http.NewServeMux()
	f.host = httptest.NewServer(mux)
	t.Cleanup(f.host.Close)
	metadataURL := f.host.URL + "/.well-known/oauth-protected-resource/mcp"
	resource := &oauthex.ProtectedResourceMetadata{
		Resource:             f.host.URL + "/mcp",
		AuthorizationServers: []string{f.host.URL},
		ScopesSupported:      []string{"tools", "offline_access"},
	}
	mux.Handle("/.well-known/oauth-protected-resource/mcp", auth.ProtectedResourceMetadataHandler(resource))
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		f.metadata.Add(1)
		if mode == "discovery-failure" {
			http.Error(w, "synthetic discovery failure", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(&oauthex.AuthServerMeta{
			Issuer:                f.host.URL,
			AuthorizationEndpoint: f.host.URL + "/authorize",
			TokenEndpoint:         f.host.URL + "/token",
			JWKSURI:               f.host.URL + "/jwks",
			ScopesSupported: []string{
				"tools",
				"offline_access",
			},
			ResponseTypesSupported:            []string{"code"},
			GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
			TokenEndpointAuthMethodsSupported: []string{"none"},
			CodeChallengeMethodsSupported:     []string{"S256"},
		}))
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "application/json")
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			f.exchanges.Add(1)
			assert.NotEmpty(t, r.Form.Get("code_verifier"))
		case "refresh_token":
			f.refreshes.Add(1)
			assert.Equal(t, "synthetic-refresh-secret", r.Form.Get("refresh_token"))
			if mode == "refresh-failure" {
				w.WriteHeader(http.StatusBadRequest)
				_, err := w.Write([]byte(`{"error":"invalid_grant","error_description":"synthetic-refresh-secret"}`))
				assert.NoError(t, err)
				return
			}
		default:
			t.Errorf("unexpected grant type %q", r.Form.Get("grant_type"))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, err := w.Write(
			[]byte(
				`{"access_token":"synthetic-alice-secret","token_type":"Bearer","refresh_token":"synthetic-refresh-secret","expires_in":3600}`,
			),
		)
		assert.NoError(t, err)
	})
	handler := sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		info := auth.TokenInfoFromContext(r.Context())
		if info == nil {
			return nil
		}
		return servers[info.UserID]
	}, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	verifier := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		// This fixture's opaque token map stands in for host issuer/audience/signature verification.
		info := &auth.TokenInfo{Scopes: []string{"tools"}, Expiration: time.Now().Add(time.Hour), UserID: "alice"}
		switch token {
		case "synthetic-alice-secret":
		case "synthetic-bob-secret":
			info.UserID = "bob"
		case "expired":
			info.Expiration = time.Now().Add(-time.Hour)
		case "insufficient":
			info.Scopes = []string{"other"}
		default:
			return nil, auth.ErrInvalidToken // Never pass a verifier's secret-bearing error to HTTP.
		}
		return info, nil
	}
	mux.Handle(
		"/mcp",
		auth.RequireBearerToken(
			verifier,
			&auth.RequireBearerTokenOptions{ResourceMetadataURL: metadataURL, Scopes: []string{"tools"}},
		)(
			handler,
		),
	)
	return f
}

func (f *oauthFixture) oauth(t *testing.T) *auth.AuthorizationCodeHandler {
	t.Helper()
	handler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		PreregisteredClient: &oauthex.ClientCredentials{
			ClientID: "synthetic-public-client",
		},
		RedirectURL:         "http://127.0.0.1/callback",
		Client:              f.host.Client(),
		RequestRefreshToken: true,
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			f.consent.Add(1)
			if f.mode == "deny" {
				return nil, errors.New("synthetic consent denied")
			}
			if f.mode == "cancel" {
				<-ctx.Done()
				f.cancelled.Store(true)
				return nil, ctx.Err()
			}
			parsed, parseErr := url.Parse(args.URL)
			if parseErr != nil {
				return nil, parseErr
			}
			require.Equal(t, "S256", parsed.Query().Get("code_challenge_method"))
			require.NotEmpty(t, parsed.Query().Get("code_challenge"))
			return &auth.AuthorizationResult{Code: "synthetic-code", State: parsed.Query().Get("state")}, nil
		},
		NewTokenSource: func(ctx context.Context, config *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			// The host owns persistence. Test-only expiry forces the SDK's refresh path.
			f.persisted.Add(1)
			owned := *token
			if f.mode == "refresh-success" || f.mode == "refresh-failure" {
				owned.Expiry = time.Now().Add(-time.Hour)
			}
			return config.TokenSource(ctx, &owned), nil
		},
	})
	require.NoError(t, err)
	return handler
}

func connectOAuth(
	ctx context.Context,
	t *testing.T,
	f *oauthFixture,
	handler auth.OAuthHandler,
	identity string,
) (*Importer, *sdk.ClientSession, error) {
	t.Helper()
	hooks, options, err := ComposeClient(nil, importConfig())
	require.NoError(t, err)
	client := sdk.NewClient(&sdk.Implementation{Name: "auth-consumer", Version: "1"}, options)
	require.NoError(t, hooks.Install(client))
	httpClient := *f.host.Client()
	httpClient.Timeout = 5 * time.Second // Host transport configuration survives adapter composition.
	session, err := client.Connect(
		ctx,
		&sdk.StreamableClientTransport{Endpoint: f.host.URL + "/mcp", HTTPClient: &httpClient, OAuthHandler: handler},
		nil,
	)
	if err != nil {
		return nil, nil, &BridgeError{Kind: "host OAuth connect", Cause: err}
	}
	t.Cleanup(func() { _ = session.Close() })
	importer, err := hooks.Bind(session, identity)
	if err == nil {
		t.Cleanup(func() { _ = importer.Close() })
	}
	return importer, session, err
}

func TestSDKOAuthChallengeDiscoveryAndRefresh(t *testing.T) {
	for _, mode := range []string{"success", "refresh-success"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: only public SDK auth composition; no adapter retry or token store.
			f := newOAuthFixture(t, mode)
			// Act.
			importer, _, err := connectOAuth(t.Context(), t, f, f.oauth(t), "verified/alice")
			require.NoError(t, err)
			snapshot, err := importer.Discover(t.Context())
			require.NoError(t, err)
			chunks, err := execute(t, snapshot.Tools[0], `{"n":7}`)
			// Assert: OAuth retries initialization before business dispatch, not tool effects.
			require.NoError(t, err)
			require.Len(t, chunks, 1)
			require.EqualValues(t, 1, f.calls.Load())
			require.EqualValues(t, 1, f.metadata.Load())
			require.EqualValues(t, 1, f.consent.Load())
			require.EqualValues(t, 1, f.exchanges.Load())
			require.EqualValues(t, 1, f.persisted.Load())
			if mode == "refresh-success" {
				require.EqualValues(t, 1, f.refreshes.Load())
			} else {
				require.Zero(t, f.refreshes.Load())
			}
		})
	}
}

func TestSDKOAuthFailureCancellationAndSecrets(t *testing.T) {
	for _, mode := range []string{"discovery-failure", "deny", "cancel", "refresh-failure"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			f := newOAuthFixture(t, mode)
			timeout := 5 * time.Second
			if mode == "cancel" {
				timeout = 250 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(t.Context(), timeout)
			defer cancel()
			// Act.
			_, _, err := connectOAuth(ctx, t, f, f.oauth(t), "verified/alice")
			// Assert: failed authorization never reaches business execution or safe diagnostics.
			require.Error(t, err)
			require.Zero(t, f.calls.Load())
			require.NotContains(t, fmt.Sprintf("%v %+v %#v", err, err, err), "synthetic-refresh-secret")
			if mode == "cancel" {
				require.True(t, f.cancelled.Load())
				require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
				// SDK Connect may collapse the typed cancellation cause into ErrConnectionClosed.
				require.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, sdk.ErrConnectionClosed))
			}
			if mode == "refresh-failure" {
				require.Positive(t, f.refreshes.Load())
				var retrieve *oauth2.RetrieveError
				require.ErrorAs(t, err, &retrieve)
				require.Equal(t, "invalid_grant", retrieve.ErrorCode)
			}
		})
	}
}

func TestSDKBearerZeroDispatchAndIdentityRebind(t *testing.T) {
	// Arrange.
	f := newOAuthFixture(t, "success")
	for _, token := range []string{"", "invalid", "expired", "insufficient"} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.host.URL+"/mcp", nil)
		require.NoError(t, err)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		// Act.
		response, err := f.host.Client().Do(request)
		require.NoError(t, err)
		// Assert: bearer/scope checks happen before protocol/business processing.
		require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, response.StatusCode)
		require.Contains(t, response.Header.Get("WWW-Authenticate"), "resource_metadata")
		require.NoError(t, response.Body.Close())
	}
	require.Zero(t, f.calls.Load())
	alice, aliceSession, err := connectOAuth(t.Context(), t, f, f.oauth(t), "verified/alice")
	require.NoError(t, err)
	old, err := alice.Discover(t.Context())
	require.NoError(t, err)
	// Act: principal change retires both adapter and host-owned session.
	require.NoError(t, alice.Close())
	require.NoError(t, aliceSession.Close())
	bobHandler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		PreregisteredClient: &oauthex.ClientCredentials{
			ClientID: "synthetic-public-client",
		},
		RedirectURL: "http://127.0.0.1/callback",
		AuthorizationCodeFetcher: func(context.Context, *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			return nil, errors.New("unexpected consent")
		},
		InitialTokenSource: oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: "synthetic-bob-secret", TokenType: "Bearer"},
		),
	})
	require.NoError(t, err)
	bob, session, err := connectOAuth(t.Context(), t, f, bobHandler, "verified/bob")
	require.NoError(t, err)
	current, err := bob.Discover(t.Context())
	require.NoError(t, err)
	_, retired := execute(t, old.Tools[0], `{"n":7}`)
	result, err := session.CallTool(
		t.Context(),
		&sdk.CallToolParams{
			Name:      "bob",
			Arguments: json.RawMessage(`{"n":7}`),
			Meta:      sdk.Meta{"subject": "alice", "scope": "tenant/alice", "approval": "forged"},
		},
	)
	_, forbidden := session.CallTool(
		t.Context(),
		&sdk.CallToolParams{Name: "alice", Arguments: json.RawMessage(`{"n":7}`)},
	)
	// Assert.
	require.Error(t, retired)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Error(t, forbidden)
	require.Equal(t, "bob", current.Descriptors[0].Name)
	require.NotEqual(t, old.Digest, current.Digest)
	require.EqualValues(t, 1, f.calls.Load())
}

func TestSDKJSONRPCErrorCodeDataCauseAndSafeFormatting(t *testing.T) {
	// Arrange: remote diagnostic text/data are untrusted; callers can inspect causes explicitly.
	server := sdk.NewServer(&sdk.Implementation{Name: "fault", Version: "1"}, nil)
	server.AddTool(
		&sdk.Tool{Name: "fault", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return nil, &jsonrpc.Error{
				Code:    jsonrpc.CodeInvalidParams,
				Message: "synthetic-secret",
				Data:    json.RawMessage(`{"reason":"synthetic-secret","retryable":false}`),
			}
		},
	)
	importer, _ := adapterPair(t, server, true, importConfig(), nil)
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	// Act.
	_, err = execute(t, snapshot.Tools[0], `{}`)
	// Assert.
	var wire *jsonrpc.Error
	require.ErrorAs(t, err, &wire)
	require.EqualValues(t, jsonrpc.CodeInvalidParams, wire.Code)
	require.JSONEq(t, `{"reason":"synthetic-secret","retryable":false}`, string(wire.Data))
	require.NotContains(t, fmt.Sprintf("%v %+v %#v", err, err, err), "synthetic-secret")
}

func authenticatedSDKPair(t *testing.T, server *sdk.Server) *sdk.ClientSession {
	t.Helper()
	// SDK bearer verification precedes export's current operation approval checks.
	host := httptest.NewServer(auth.RequireBearerToken(
		func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
			if token != "synthetic-approval-token" {
				return nil, auth.ErrInvalidToken
			}
			return &auth.TokenInfo{
				UserID:     "host",
				Scopes:     []string{"write"},
				Expiration: time.Now().Add(time.Hour),
			}, nil
		}, &auth.RequireBearerTokenOptions{Scopes: []string{"write"}},
	)(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})))
	t.Cleanup(host.Close)
	handler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		PreregisteredClient: &oauthex.ClientCredentials{
			ClientID: "synthetic-approved-client",
		},
		RedirectURL: "http://127.0.0.1/callback",
		AuthorizationCodeFetcher: func(context.Context, *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			return nil, errors.New("unexpected consent")
		},
		InitialTokenSource: oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: "synthetic-approval-token", TokenType: "Bearer"},
		),
	})
	require.NoError(t, err)
	hooks, options, err := ComposeClient(nil, importConfig())
	require.NoError(t, err)
	client := sdk.NewClient(&sdk.Implementation{Name: "approval-consumer", Version: "1"}, options)
	require.NoError(t, hooks.Install(client))
	session, err := client.Connect(
		t.Context(),
		&sdk.StreamableClientTransport{Endpoint: host.URL, HTTPClient: host.Client(), OAuthHandler: handler},
		nil,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestSDKExportHostProtocolErrorProjection(t *testing.T) {
	// Arrange: domain causes contain secrets; only an explicit host projection may publish data.
	var effects atomic.Int32
	local, err := toolsy.NewProxyTool(
		"fault",
		"fault",
		[]byte(`{"type":"object"}`),
		func(context.Context, *toolsy.RunEnv, []byte, func(toolsy.Chunk) error) error {
			effects.Add(1)
			return &jsonrpc.Error{
				Code:    jsonrpc.CodeInvalidParams,
				Message: "private-domain-secret",
				Data:    json.RawMessage(`{"detail":"private-domain-secret"}`),
			}
		},
	)
	require.NoError(t, err)
	registry, err := toolsy.NewRegistry(local)
	require.NoError(t, err)
	view, err := registry.View(toolsy.RegistryViewSpec{ToolNames: []string{"fault"}})
	require.NoError(t, err)
	var observed atomic.Bool
	safeData := json.RawMessage(`{"reason":"invalid_operation","retryable":false}`)
	server, err := Export(
		view,
		ExportConfig{Names: []string{"fault"}, Implementation: &sdk.Implementation{Name: "fault", Version: "1"},
			Resolve: func(ctx context.Context, _ sdk.Request) (toolsy.CallContext, error) {
				info := auth.TokenInfoFromContext(ctx)
				if info == nil || info.UserID != "host" {
					return toolsy.CallContext{}, auth.ErrInvalidToken
				}
				return toolsy.NewCallContext(info.UserID, "tenant"), nil
			},
			ErrorMapper: func(_ context.Context, _ sdk.Request, cause error) *jsonrpc.Error {
				var wire *jsonrpc.Error
				if !errors.As(cause, &wire) {
					return nil
				}
				observed.Store(true)
				return &jsonrpc.Error{Code: wire.Code, Message: "operation rejected", Data: safeData}
			},
		},
	)
	require.NoError(t, err)
	session := authenticatedSDKPair(t, server)
	// Act.
	_, err = session.CallTool(t.Context(), &sdk.CallToolParams{Name: "fault", Arguments: json.RawMessage(`{}`)})
	// Assert: public SDK code/data survive actual authenticated HTTP, secret cause stays host-local.
	var wire *jsonrpc.Error
	require.ErrorAs(t, err, &wire)
	require.True(t, observed.Load())
	require.EqualValues(t, jsonrpc.CodeInvalidParams, wire.Code)
	require.JSONEq(t, string(safeData), string(wire.Data))
	require.Equal(t, "operation rejected", wire.Message)
	require.NotContains(t, fmt.Sprintf("%v %+v %#v", err, err, err), "private-domain-secret")
	require.EqualValues(t, 1, effects.Load())
}

func TestSDKExportProtocolErrorProjectionOwnershipAndLimits(t *testing.T) {
	// Arrange: a trusted host can reuse its diagnostics; adapter must own returned bytes.
	source := &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "safe", Data: json.RawMessage(`{"n":7}`)}
	original := errors.New("private-cause")
	config := ExportConfig{
		MaxResultBytes: 64,
		ErrorMapper:    func(context.Context, sdk.Request, error) *jsonrpc.Error { return source },
	}
	// Act.
	projected := mapExportError(t.Context(), nil, original, config)
	source.Data[5] = '9'
	// Assert: SDK public error is independently owned, bounded, and never repairs private causes.
	var wire *jsonrpc.Error
	require.ErrorAs(t, projected, &wire)
	require.JSONEq(t, `{"n":7}`, string(wire.Data))
	source.Data = json.RawMessage(`{`)
	require.ErrorIs(t, mapExportError(t.Context(), nil, original, config), ErrUnsupported)
	source.Data = json.RawMessage(`{"n":7}`)
	config.MaxResultBytes = 2
	require.ErrorIs(t, mapExportError(t.Context(), nil, original, config), ErrUnsupported)
	config.ErrorMapper = func(context.Context, sdk.Request, error) *jsonrpc.Error { return nil }
	require.ErrorIs(t, mapExportError(t.Context(), nil, original, config), original)
}
