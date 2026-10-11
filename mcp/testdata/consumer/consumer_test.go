package consumer_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/skosovsky/toolsy"
	bridge "github.com/skosovsky/toolsy/mcp"
)

func TestConsumerPublicAPIs(t *testing.T) {
	// Arrange: external package, without workspace replacements or hidden aliases.
	config := bridge.ImportConfig{
		Classify: func(context.Context, *sdk.Tool) ([]toolsy.ToolOption, error) { return nil, nil },
	}
	// Act.
	hooks, options, err := bridge.ComposeClient(nil, config)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "consumer", Version: "1"}, options)
	err = hooks.Install(client)
	// Assert.
	if err != nil || !options.MultiRoundTrip.Disabled {
		t.Fatalf("composition: %v", err)
	}
	if _, err = hooks.Bind(nil, "tenant"); err == nil {
		t.Fatal("nil session accepted")
	}
	if _, err = bridge.Export(nil, bridge.ExportConfig{}); err == nil {
		t.Fatal("unscoped export accepted")
	}
}

func TestConsumerHostAuthComposition(t *testing.T) {
	// Arrange: public SDK auth options and error types compose outside adapter package.
	handler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		PreregisteredClient: &oauthex.ClientCredentials{ClientID: "synthetic-client"},
		RedirectURL:         "http://127.0.0.1/callback",
		AuthorizationCodeFetcher: func(context.Context, *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			return nil, errors.New("host consent required")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := &sdk.StreamableClientTransport{Endpoint: "http://127.0.0.1/mcp", OAuthHandler: handler}
	cause := &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "untrusted-secret"}
	// Act.
	_, options, err := bridge.ComposeClient(
		nil,
		bridge.ImportConfig{
			Classify: func(context.Context, *sdk.Tool) ([]toolsy.ToolOption, error) { return nil, nil },
		},
	)
	mapped := &bridge.BridgeError{Kind: "protocol", Cause: cause}
	var recovered *jsonrpc.Error
	// Assert: the external graph uses SDK types without optional wrappers or lost cause.
	if err != nil || !options.MultiRoundTrip.Disabled || transport.OAuthHandler != handler ||
		transport.Endpoint != "http://127.0.0.1/mcp" {
		t.Fatalf("auth composition: %v", err)
	}
	if !errors.As(mapped, &recovered) || recovered.Code != jsonrpc.CodeInvalidParams {
		t.Fatal("protocol cause lost")
	}
	if strings.Contains(fmt.Sprintf("%+v", mapped), "untrusted-secret") {
		t.Fatal("unsafe diagnostics")
	}
}
