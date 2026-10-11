// Example auth composes an SDK OAuth client and bearer-protected scoped export.
// All tokens are synthetic and local; host verification/storage are deployment ports.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"

	"github.com/skosovsky/toolsy"
	bridge "github.com/skosovsky/toolsy/mcp"
)

const (
	fixtureUser    = "fixture-user"
	fixtureTenant  = "fixture-tenant"
	echoName       = "echo"
	accessScope    = "tools"
	requestTimeout = 5 * time.Second
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, &bridge.BridgeError{Kind: "host auth example", Cause: err})
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	server, err := scopedServer()
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	host := httptest.NewServer(mux)
	defer host.Close()
	metadataURL := host.URL + "/.well-known/oauth-protected-resource/mcp"
	mux.Handle(
		"/.well-known/oauth-protected-resource/mcp",
		auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
			Resource: host.URL + "/mcp", ScopesSupported: []string{accessScope},
		}),
	)
	// A real host verifies issuer/audience/signature or uses its trusted opaque-token store.
	verifier := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if token != "synthetic-local-token" {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{
			UserID:     fixtureUser,
			Scopes:     []string{accessScope},
			Expiration: time.Now().Add(time.Hour),
		}, nil
	}
	mux.Handle(
		"/mcp",
		auth.RequireBearerToken(
			verifier,
			&auth.RequireBearerTokenOptions{ResourceMetadataURL: metadataURL, Scopes: []string{accessScope}},
		)(
			sdk.NewStreamableHTTPHandler(
				func(*http.Request) *sdk.Server { return server },
				&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
			),
		),
	)
	// Host loads a previously obtained token. The stock handler also supports challenge/
	// discovery, code exchange and refresh; sdk_auth_test.go exercises those paths.
	handler, err := hostOAuthHandler(host.Client())
	if err != nil {
		return err
	}
	hooks, options, err := bridge.ComposeClient(
		nil,
		bridge.ImportConfig{Classify: func(context.Context, *sdk.Tool) ([]toolsy.ToolOption, error) {
			return []toolsy.ToolOption{toolsy.WithReadOnly()}, nil
		}},
	)
	if err != nil {
		return err
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "authenticated-import", Version: "1"}, options)
	if err = hooks.Install(client); err != nil {
		return err
	}
	httpClient := *host.Client()
	httpClient.Timeout = requestTimeout
	session, err := client.Connect(
		ctx,
		&sdk.StreamableClientTransport{Endpoint: host.URL + "/mcp", HTTPClient: &httpClient, OAuthHandler: handler},
		nil,
	)
	if err != nil {
		return err
	}
	defer session.Close()
	importer, err := hooks.Bind(session, "host-verified/fixture-user/fixture-tenant")
	if err != nil {
		return err
	}
	defer importer.Close()
	snapshot, err := importer.Discover(ctx)
	if err != nil {
		return err
	}
	imported, err := toolsy.NewRegistry(snapshot.Tools...)
	if err != nil {
		return err
	}
	return imported.Execute(
		ctx,
		toolsy.ToolCall{ToolName: echoName, Input: toolsy.ToolInput{ArgsJSON: []byte(`{"n":7}`)}},
		func(chunk toolsy.Chunk) error {
			if chunk.Event == toolsy.EventResult {
				fmt.Println(string(chunk.Data))
			}
			return nil
		},
	)
}

// hostOAuthHandler configures a host-owned token source and interaction port.
func hostOAuthHandler(httpClient *http.Client) (*auth.AuthorizationCodeHandler, error) {
	return auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		PreregisteredClient: &oauthex.ClientCredentials{
			ClientID: "fixture-client",
		},
		RedirectURL: "http://127.0.0.1/callback",
		Client:      httpClient,
		InitialTokenSource: oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: "synthetic-local-token", TokenType: "Bearer"},
		),
		AuthorizationCodeFetcher: func(context.Context, *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			return nil, errors.New("host interaction required")
		},
	})
}

func hostResolve(ctx context.Context, _ sdk.Request) (toolsy.CallContext, error) {
	info := auth.TokenInfoFromContext(ctx)
	if info == nil || info.UserID != fixtureUser {
		return toolsy.CallContext{}, auth.ErrInvalidToken
	}
	return toolsy.NewCallContext(info.UserID, fixtureTenant), nil
}

func scopedServer() (*sdk.Server, error) {
	local, err := toolsy.NewProxyTool(echoName, "Echo JSON", []byte(`{"type":"object"}`),
		func(_ context.Context, env *toolsy.RunEnv, args []byte, yield func(toolsy.Chunk) error) error {
			if env.CallContext().Subject != fixtureUser || env.CallContext().Scope != fixtureTenant {
				return auth.ErrInvalidToken
			}
			return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: args, MimeType: toolsy.MimeTypeJSON})
		}, toolsy.WithReadOnly())
	if err != nil {
		return nil, err
	}
	registry, err := toolsy.NewRegistry(local)
	if err != nil {
		return nil, err
	}
	view, err := registry.View(toolsy.RegistryViewSpec{ToolNames: []string{echoName}, Owner: fixtureTenant})
	if err != nil {
		return nil, err
	}
	return bridge.Export(
		view,
		bridge.ExportConfig{
			Names:          []string{echoName},
			Implementation: &sdk.Implementation{Name: "authenticated-export", Version: "1"},
			Resolve:        hostResolve,
		},
	)
}
