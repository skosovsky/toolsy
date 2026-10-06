package mcp

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func TestHTTPRPCRejectsSameOriginBodyReplay(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			// Arrange.
			var targetCalls atomic.Int32
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/target" {
					targetCalls.Add(1)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				http.Redirect(w, r, "/target", status)
			}))
			t.Cleanup(origin.Close)
			transport := NewStreamableHTTPTransport(origin.URL, WithStreamableHTTPAllowPrivateIPs(true))
			require.NoError(t, transport.Start(t.Context()))
			pending, err := adversarialDeliveredDiscovery(t.Context(), t, transport)
			require.NoError(t, err)
			// Act.
			_, err = pending.Await(t.Context())
			// Assert.
			var refused *httptool.RedirectError
			require.ErrorAs(t, err, &refused)
			te, ok := toolsy.AsToolError(err)
			require.True(t, ok)
			require.Equal(t, toolsy.CodeRemoteExecution, te.Code)
			require.False(t, te.Retryable)
			require.False(t, toolsy.ClientCorrectable(te.Code))
			require.Zero(t, targetCalls.Load())
			require.NoError(t, transport.Close())
		})
	}
}
