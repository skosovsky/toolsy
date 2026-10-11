package sdkcheck_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCharacterizationPublicResourceSubscriptionLifecycle(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "absent-capability", true: "supported"}[supported],
			func(t *testing.T) { checkResourceSubscription(t, supported) },
		)
	}
}

//nolint:gocognit // One real-stream lifecycle exercises ACK narrowing, update delivery and explicit teardown.
func checkResourceSubscription(t *testing.T, supported bool) {
	t.Helper()
	// Arrange: observe ACK through public middleware, not a private handle.
	options := &mcp.ServerOptions{}
	if supported {
		options.SubscribeHandler = func(context.Context, *mcp.SubscribeRequest) error { return nil }
		options.UnsubscribeHandler = func(context.Context, *mcp.UnsubscribeRequest) error { return nil }
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "subscriptions", Version: "1"}, options)
	server.AddResource(
		&mcp.Resource{URI: "resource://one", Name: "one"},
		func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{}, nil
		},
	)
	ack := make(chan *mcp.SubscriptionsAcknowledgedParams, 8)
	updated := make(chan string, 8)
	client := mcp.NewClient(
		&mcp.Implementation{Name: "host", Version: "1"},
		&mcp.ClientOptions{
			MultiRoundTrip:         &mcp.MultiRoundTripOptions{Disabled: true},
			ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) { updated <- req.Params.URI },
		},
	)
	client.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "notifications/subscriptions/acknowledged" {
				if params, ok := req.GetParams().(*mcp.SubscriptionsAcknowledgedParams); ok {
					ack <- params
				}
			}
			return next(ctx, method, req)
		}
	})
	session := connect(t, server, client, true)
	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	defer cancel()
	finished := make(chan error, 1)
	// Act: Subscribe starts SDK-owned listening without an adapter subscription handle.
	go func() { finished <- session.Subscribe(ctx, &mcp.SubscribeParams{URI: "resource://one"}) }()
	var effective *mcp.SubscriptionsAcknowledgedParams
	select {
	case effective = <-ack:
	case <-ctx.Done():
		t.Fatal("resource subscription ACK missing")
	}
	// Assert: unsupported filters are narrowed; supported notification arrives.
	if supported {
		if len(effective.Notifications.ResourceSubscriptions) != 1 {
			t.Fatalf("effective filter: %+v", effective.Notifications)
		}
		if err := server.ResourceUpdated(
			ctx,
			&mcp.ResourceUpdatedNotificationParams{URI: "resource://one"},
		); err != nil {
			t.Fatal(err)
		}
		select {
		case uri := <-updated:
			if uri != "resource://one" {
				t.Fatalf("unexpected URI %s", uri)
			}
		case <-ctx.Done():
			t.Fatal("resource update missing")
		}
	} else if len(effective.Notifications.ResourceSubscriptions) != 0 {
		t.Fatalf("unsupported filter accepted: %+v", effective.Notifications)
	}
	if err := session.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: "resource://one"}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("listen completion: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Unsubscribe did not release Listen")
	}
	if _, err := session.ListResources(ctx, nil); err != nil {
		t.Fatalf("shared session was closed: %v", err)
	}
}

func TestCharacterizationSubscriptionFailureIsNotReportedOrRetried(t *testing.T) {
	// Arrange: a server fails its subscription handler before the ACK.
	var calls atomic.Int32
	server := mcp.NewServer(
		&mcp.Implementation{Name: "failed-listen", Version: "1"},
		&mcp.ServerOptions{SubscribeHandler: func(context.Context, *mcp.SubscribeRequest) error {
			calls.Add(1)
			return errors.New("fixture listen rejected")
		}, UnsubscribeHandler: func(context.Context, *mcp.UnsubscribeRequest) error { return nil }},
	)
	server.AddResource(
		&mcp.Resource{URI: "resource://one", Name: "one"},
		func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{}, nil
		},
	)
	session := connect(t, server, newClient(true), true)
	// Act: stream handler rejects listening after the public Subscribe returns.
	first := session.Subscribe(t.Context(), &mcp.SubscribeParams{URI: "resource://one"})
	waitSubscriptionCalls(t, &calls, 1)
	second := session.Subscribe(t.Context(), &mcp.SubscribeParams{URI: "resource://one"})
	// Assert: current SDK reports neither the listener error nor a retry on a repeated Subscribe.
	if first != nil || second != nil || calls.Load() != 1 {
		t.Fatalf("first=%v second=%v calls=%d", first, second, calls.Load())
	}
	if err := session.Unsubscribe(t.Context(), &mcp.UnsubscribeParams{URI: "resource://one"}); err != nil {
		t.Fatal(err)
	}
	third := session.Subscribe(t.Context(), &mcp.SubscribeParams{URI: "resource://one"})
	waitSubscriptionCalls(t, &calls, 2)
	if third != nil || calls.Load() != 2 {
		t.Fatalf("explicit reset: %v calls=%d", third, calls.Load())
	}
}

func waitSubscriptionCalls(t *testing.T, calls *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for calls.Load() < want {
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatalf("subscription dispatch count=%d want=%d", calls.Load(), want)
		}
	}
}
