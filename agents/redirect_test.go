package agents

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

//nolint:gocognit // Integrated wire matrix checks independent origin, method and status combinations.
func TestRPCRejectsRedirects(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, sameOrigin := range []bool{false, true} {
			for _, operation := range []string{"create", "cancel"} {
				t.Run(operation+"/"+strconv.Itoa(status)+"/same="+strconv.FormatBool(sameOrigin), func(t *testing.T) {
					// Arrange.
					var targets, sources atomic.Int32
					targetHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						targets.Add(1)
						_, _ = w.Write([]byte(`{"artifacts":[],"task_id":"task-1","status":"pending"}`))
					})
					target := httptest.NewServer(targetHandler)
					t.Cleanup(target.Close)
					source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/target" {
							targetHandler(w, r)
							return
						}
						sources.Add(1)
						if r.Header.Get("Authorization") != "Bearer origin-secret" {
							t.Error("missing original credentials")
						}
						location := target.URL + "/target"
						if sameOrigin {
							location = "/target"
						}
						http.Redirect(w, r, location, status)
					}))
					t.Cleanup(source.Close)
					client := mustClient(t, source.URL, WithAllowPrivateIPs(true))
					// Act.
					var err error
					if operation == "create" {
						_, err = client.CreateTask(
							t.Context(),
							json.RawMessage(`{"secret":"body"}`),
							"Bearer origin-secret",
						)
					} else {
						err = client.CancelTask(t.Context(), "task-1", "Bearer origin-secret")
					}
					// Assert.
					require.Error(t, err)
					var refused *httptool.RedirectError
					require.ErrorAs(t, err, &refused)
					te, ok := toolsy.AsToolError(err)
					require.True(t, ok)
					require.Equal(t, toolsy.CodeRemoteExecution, te.Code)
					require.False(t, te.Retryable)
					require.False(t, toolsy.ClientCorrectable(te.Code))
					if operation == "create" {
						var outcome *RemoteOutcomeError
						require.ErrorAs(t, err, &outcome)
						require.Equal(t, OutcomeUnknown, outcome.Outcome)
					}
					require.EqualValues(t, 1, sources.Load())
					require.Zero(t, targets.Load(), "redirect must not dispatch a second request")
				})
			}
		}
	}
}

//nolint:gocognit // Integrated wire matrix checks independent origin, method and status combinations.
func TestPublicStreamRedirectOrigin(t *testing.T) {
	for _, sameOrigin := range []bool{false, true} {
		t.Run(strconv.FormatBool(sameOrigin), func(t *testing.T) {
			// Arrange.
			var targets atomic.Int32
			targetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				targets.Add(1)
				if r.Header.Get("Authorization") != "Bearer origin-secret" {
					t.Error("same-origin read lost credentials")
				}
				_, _ = w.Write([]byte(stepFrame("done", "completed", true)))
			})
			target := httptest.NewServer(targetHandler)
			t.Cleanup(target.Close)
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/target" {
					targetHandler(w, r)
					return
				}
				location := target.URL + "/target"
				if sameOrigin {
					location = "/target"
				}
				http.Redirect(w, r, location, http.StatusTemporaryRedirect)
			}))
			t.Cleanup(source.Close)
			policy := testPolicy()
			policy.MaxReconnects = 0
			client := mustClient(t, source.URL, WithAllowPrivateIPs(true), WithStreamPolicy(policy))
			var steps []Step
			var streamErr error
			// Act.
			for step, err := range client.StreamSteps(t.Context(), "t", "Bearer origin-secret") {
				if err != nil {
					streamErr = err
				} else {
					steps = append(steps, step)
				}
			}
			// Assert.
			if sameOrigin {
				require.NoError(t, streamErr)
				require.Len(t, steps, 1)
				require.True(t, steps[0].IsLast)
				require.EqualValues(t, 1, targets.Load())
			} else {
				var refused *httptool.RedirectError
				require.ErrorAs(t, streamErr, &refused)
				require.Empty(t, steps)
				require.Zero(t, targets.Load())
			}
		})
	}
}
