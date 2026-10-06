package mcp

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPAuthenticationFailurePreservesSafeHintsWithoutRetry(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			// Arrange. Credentials, free-text hints, other headers and body contain secrets.
			const secret = "task40-secret-marker"
			var requests, discovery atomic.Int32
			hint := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				discovery.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer hint.Close()
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, "Bearer "+secret, r.Header.Get("Authorization"))
				w.Header().Add("WWW-Authenticate", fmt.Sprintf(
					`Bearer resource_metadata="%s/metadata", error="invalid_token", error_description="%s", token="%s", realm="%s"`,
					hint.URL,
					secret,
					secret,
					secret,
				))
				w.Header().Add("WWW-Authenticate", "Basic "+secret)
				w.Header().Set("X-Private", secret)
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, secret)
			}))
			defer server.Close()
			transport := NewStreamableHTTPTransport(server.URL,
				WithStreamableHTTPAllowPrivateIPs(true), WithStreamableHTTPLogger(logger),
				WithStreamableHTTPRequestDecorator(func(request *http.Request) error {
					request.Header.Set("Authorization", "Bearer "+secret)
					return nil
				}))
			require.NoError(t, transport.Start(context.Background()))

			// Act. Host inspection is explicit; error logging omits untrusted values.
			_, err := httpMigrationRequest(transport)
			require.NoError(t, transport.Close())
			var failure *HTTPError
			require.ErrorAs(t, err, &failure)
			require.Len(t, failure.Challenges, 2)
			logger.Error("host auth failure", "error", err, "challenge", failure.Challenges[0])
			representations := fmt.Sprintf("%v %+v %#v %#v %s", failure, failure, failure, *failure, logs.String())

			// Assert. No discovery, token refresh or retry is activated by the hint.
			require.Equal(t, status, failure.StatusCode)
			require.Equal(t, []HTTPAuthChallenge{
				{Scheme: "Bearer", ErrorCode: "invalid_token", ResourceMetadata: hint.URL + "/metadata"},
				{Scheme: "Basic"},
			}, failure.Challenges)
			require.False(t, failure.ChallengesTruncated)
			require.NotContains(t, representations, secret)
			require.NotContains(t, representations, hint.URL)
			require.EqualValues(t, 1, requests.Load())
			require.Zero(t, discovery.Load())
		})
	}
}

func TestAuthenticationChallengeBoundsAndUnsafeParameters(t *testing.T) {
	tests := []struct {
		name      string
		headers   []string
		count     int
		truncated bool
		resource  string
	}{
		{
			name: "quoted comma",
			headers: []string{
				`Basic realm="a,b", Bearer resource_metadata="https://example.org/meta", error="insufficient_scope"`,
			},
			count:    2,
			resource: "https://example.org/meta",
		},
		{
			name:    "query discarded",
			headers: []string{`Bearer resource_metadata="https://example.org/meta?token=secret", error="secret"`},
			count:   1,
		},
		{
			name:    "credentials discarded",
			headers: []string{`Bearer resource_metadata="https://user:secret@example.org/meta"`},
			count:   1,
		},
		{
			name:    "fragment discarded",
			headers: []string{`Bearer resource_metadata="https://example.org/meta#secret"`},
			count:   1,
		},
		{name: "header budget", headers: []string{strings.Repeat("x", maxAuthHeaderBytes+1)}, truncated: true},
		{
			name:      "challenge budget",
			headers:   []string{"Basic, Bearer, Digest, Negotiate, Other"},
			count:     4,
			truncated: true,
		},
		{name: "malformed quotes", headers: []string{`Bearer resource_metadata="https://example.org/meta`}},
		{name: "controls", headers: []string{"Bearer error=invalid_token\nsecret"}},
		{
			name: "parameter budget",
			headers: []string{
				`Bearer resource_metadata="https://example.org/` + strings.Repeat("x", maxAuthParameterBytes) + `"`,
			},
			count: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange.
			headers := tt.headers
			// Act.
			challenges, truncated := parseAuthChallenges(headers)
			// Assert.
			require.Len(t, challenges, tt.count)
			require.Equal(t, tt.truncated, truncated)
			if len(challenges) > 0 {
				require.Equal(t, tt.resource, challenges[len(challenges)-1].ResourceMetadata)
			}
		})
	}
}

func TestAuthenticationFailureDoesNotDrainUnfinishedBody(t *testing.T) {
	// Arrange. The server sends authentication status but never completes its body.
	released := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", "Bearer error=invalid_token")
		w.WriteHeader(http.StatusUnauthorized)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-released:
		}
	}))
	defer server.Close()
	defer close(released)
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	defer func() { require.NoError(t, transport.Close()) }()
	completed := make(chan error, 1)

	// Act.
	go func() { _, err := httpMigrationRequest(transport); completed <- err }()

	// Assert. Known authentication failure must reach the host without body EOF.
	select {
	case err := <-completed:
		var failure *HTTPError
		require.ErrorAs(t, err, &failure)
		require.Equal(t, http.StatusUnauthorized, failure.StatusCode)
	case <-time.After(time.Second):
		t.Fatal("authentication failure blocked on response body")
	}
}
