package recipe

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/skosovsky/toolsy"
)

// Profile binds the trusted request to the prepared invocation, never model text.
// Identity authenticates/maps BYOT values in the host. The clock and store are host-owned.
func Profile[S, C any, A comparable](
	store toolsy.OperationStore,
	codec toolsy.ResultCodec,
	issuer string,
	clock func() time.Time,
	identity func(S, C) (string, string, error),
) (*toolsy.OperationProfile, error) {
	if identity == nil {
		return nil, errors.New("host dispatch: identity mapper required")
	}
	return toolsy.NewOperationProfile(toolsy.OperationProfileConfig{
		Store: store, Codec: codec, Issuer: issuer, Clock: clock, Lease: time.Minute,
		Prepare: func(ctx context.Context, p toolsy.PreparedCall) (toolsy.OperationIntent, error) {
			r, ok := CurrentRequest[S, C, A](ctx)
			if !ok {
				return toolsy.OperationIntent{}, errors.New("host dispatch: trusted request missing")
			}
			typed, err := toolsy.TypedContext[S, C](p.Context)
			if err != nil {
				return toolsy.OperationIntent{}, err
			}
			subject, scope, err := identity(typed.Subject, typed.Scope)
			if err != nil {
				return toolsy.OperationIntent{}, err
			}
			dependency, err := json.Marshal(struct {
				Dependency string `json:"dependency"`
				Activity   A      `json:"activity"`
			}{r.DependencyFingerprint, r.ActivityIdentity})
			if err != nil {
				return toolsy.OperationIntent{}, err
			}
			return toolsy.OperationIntent{
				Namespace:         "host-dispatch",
				Subject:           subject,
				Scope:             scope,
				OperationID:       r.OperationID,
				AttemptID:         r.AttemptID,
				GrantID:           r.GrantID,
				PolicyFingerprint: r.PolicyFingerprint,
				CanonicalDigest:   string(dependency),
				CanonicalRules:    "prepared-json-and-attachments",
				DownstreamKey:     r.OperationID,
				DisplayJSON:       p.Input.ArgsJSON,
			}, nil
		},
	})
}
