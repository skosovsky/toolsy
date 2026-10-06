package httptool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type credentialSpy struct{ calls int }

func (s *credentialSpy) GetAuth(context.Context, string) (string, error) {
	s.calls++
	return "Bearer secret", nil
}

func TestCredentialOriginBinding(t *testing.T) {
	var received []string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.Header.Get("Authorization"))
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()
	spy := &credentialSpy{}
	tools, err := AsTools(
		WithAllowedDomains([]string{"127.0.0.1"}),
		WithAllowPrivateIPs(true),
		WithCredentialOrigins([]string{source.URL}),
	)
	require.NoError(t, err)
	for _, endpoint := range []string{source.URL, target.URL} {
		input, marshalErr := json.Marshal(getArgs{URL: endpoint})
		require.NoError(t, marshalErr)
		err = tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil, toolsy.WithCredentials(spy)),
			toolsy.ToolInput{ArgsJSON: input},
			func(toolsy.Chunk) error { return nil },
		)
		require.NoError(t, err)
	}
	require.Equal(t, 1, spy.calls)
	require.Equal(t, []string{"Bearer secret", "", ""}, received)
}

func TestHTTPFinalWireLimitAfterEscaping(t *testing.T) {
	srv := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(strings.Repeat("<", 40))) },
		),
	)
	defer srv.Close()
	tools, err := AsTools(
		WithAllowedDomains([]string{"127.0.0.1"}),
		WithAllowPrivateIPs(true),
		WithMaxResponseBody(100),
		WithMaxWireBytes(100),
	)
	require.NoError(t, err)
	yielded := false
	input, err := json.Marshal(getArgs{URL: srv.URL})
	require.NoError(t, err)
	err = tools[0].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: input},
		func(toolsy.Chunk) error { yielded = true; return nil },
	)
	require.ErrorIs(t, err, toolsy.ErrValidation)
	require.False(t, yielded)
}

func TestPOSTOversizeRejectedBeforeDispatch(t *testing.T) {
	called := false
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusOK) }),
	)
	defer srv.Close()
	tools, err := AsTools(WithAllowedDomains([]string{"127.0.0.1"}), WithAllowPrivateIPs(true), WithMaxRequestBody(4))
	require.NoError(t, err)
	input, err := json.Marshal(postArgs{URL: srv.URL, JSONBody: json.RawMessage(`{"large":true}`)})
	require.NoError(t, err)
	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: input},
		func(toolsy.Chunk) error { return nil },
	)
	require.ErrorIs(t, err, toolsy.ErrValidation)
	require.False(t, called)
}

func TestPublicToolRedirectRefusalClassification(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(method+"/"+strconv.Itoa(status), func(t *testing.T) {
				// Arrange.
				var targets atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/target" {
						targets.Add(1)
						w.WriteHeader(http.StatusOK)
						return
					}
					location := "/target"
					if method == http.MethodGet {
						location = "http://localhost:1/target" // Not in the allowed host list.
					}
					http.Redirect(w, r, location, status)
				}))
				t.Cleanup(server.Close)
				tools, err := AsTools(WithAllowedDomains([]string{"127.0.0.1"}), WithAllowPrivateIPs(true))
				require.NoError(t, err)
				input, err := json.Marshal(postArgs{URL: server.URL, JSONBody: json.RawMessage(`{"secret":"body"}`)})
				require.NoError(t, err)
				index := 1
				if method == http.MethodGet {
					input, err = json.Marshal(getArgs{URL: server.URL})
					require.NoError(t, err)
					index = 0
				}
				delivered := false
				// Act.
				err = tools[index].Execute(t.Context(), toolsy.NewRunEnv(nil), toolsy.ToolInput{ArgsJSON: input},
					func(toolsy.Chunk) error { delivered = true; return nil })
				// Assert.
				var refused *RedirectError
				require.ErrorAs(t, err, &refused)
				te, ok := toolsy.AsToolError(err)
				require.True(t, ok)
				require.Equal(t, toolsy.CodeRemoteExecution, te.Code)
				require.False(t, te.Retryable)
				require.False(t, toolsy.ClientCorrectable(te.Code))
				require.False(t, delivered)
				require.Zero(t, targets.Load())
			})
		}
	}
}
