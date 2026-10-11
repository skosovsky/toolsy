package sdkcheck_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
)

// cancelledOAuth is a synthetic public extension-point probe, not an OAuth flow.
type cancelledOAuth struct {
	reject atomic.Bool
	cancel context.CancelFunc
}

func (h *cancelledOAuth) TokenSource(context.Context) (oauth2.TokenSource, error) {
	if h.reject.Load() {
		return nil, nil //nolint:nilnil // SDK OAuthHandler explicitly permits no initial token source.
	}
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "synthetic", TokenType: "Bearer"}), nil
}

func (h *cancelledOAuth) Authorize(ctx context.Context, _ *http.Request, response *http.Response) error {
	_ = response.Body.Close()
	h.cancel()
	return ctx.Err()
}

func TestCharacterizationOAuthClosureCauseCollapse(t *testing.T) {
	// Arrange: start an authenticated SDK session, then cancel authorization during a call.
	server := sdk.NewServer(&sdk.Implementation{Name: "cause-probe", Version: "1"}, nil)
	handler := auth.RequireBearerToken(func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if token != "synthetic" {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{UserID: "host", Expiration: time.Now().Add(time.Hour)}, nil
	}, nil)(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	host := httptest.NewServer(handler)
	defer host.Close()
	oauth := &cancelledOAuth{}
	client := sdk.NewClient(&sdk.Implementation{Name: "cause-probe", Version: "1"}, nil)
	session, err := client.Connect(
		t.Context(),
		&sdk.StreamableClientTransport{Endpoint: host.URL, HTTPClient: host.Client(), OAuthHandler: oauth},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	oauth.cancel = cancel
	oauth.reject.Store(true)
	// Act: after cancellation fails the connection, subsequent public calls expose closure.
	_, first := session.ListTools(ctx, nil)
	// Wait for the public session completion boundary, rather than race its teardown.
	completed := make(chan error, 1)
	go func() { completed <- session.Wait() }()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("SDK auth cancellation did not close the session")
	}
	_, second := session.ListTools(t.Context(), nil)
	// Assert: typed cancellation is not recoverable through a closed session's cause chain.
	if first == nil || !errors.Is(ctx.Err(), context.Canceled) || !errors.Is(second, sdk.ErrConnectionClosed) ||
		errors.Is(second, context.Canceled) {
		t.Fatalf("SDK OAuth closure characterization changed: first=%v second=%v context=%v", first, second, ctx.Err())
	}
}

func TestCharacterizationWrappedJSONRPCErrorDataLoss(t *testing.T) {
	// Arrange: SDK public error/message types, no adapter or private implementation.
	cause := &jsonrpc.Error{
		Code:    jsonrpc.CodeInvalidParams,
		Message: "untrusted",
		Data:    json.RawMessage(`{"reason":"safe"}`),
	}
	wrapped := fmt.Errorf("safe outer diagnostic: %w", cause)
	id, err := jsonrpc.MakeID("probe")
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	raw, err := jsonrpc.EncodeMessage(&jsonrpc.Response{ID: id, Error: wrapped})
	if err != nil {
		t.Fatal(err)
	}
	message, err := jsonrpc.DecodeMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	response, ok := message.(*jsonrpc.Response)
	if !ok {
		t.Fatal("SDK response type changed")
	}
	// Assert: code and outer text survive, Data does not. Host must project top-level public errors explicitly.
	var wire *jsonrpc.Error
	if !errors.As(response.Error, &wire) || wire.Code != jsonrpc.CodeInvalidParams || len(wire.Data) != 0 ||
		wire.Message != wrapped.Error() {
		t.Fatalf("wrapped SDK error characterization changed: %s", raw)
	}
}
