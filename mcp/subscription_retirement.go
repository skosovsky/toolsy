package mcp

import (
	"encoding/json"
	"errors"
	"time"
)

// SubscriptionLimits bounds live subscriptions and locally canceled ID history.
// MaxTracked reserves space for every active subscription's eventual retirement.
// At saturation new Listen calls fail; no live or retained route is evicted.
// After RetireTTL an ID is unknown again and strict protocol handling applies.
type SubscriptionLimits struct {
	MaxActive  int
	MaxTracked int
	RetireTTL  time.Duration
}

func (l SubscriptionLimits) normalized() (SubscriptionLimits, error) {
	if l.MaxActive < 0 || l.MaxTracked < 0 || l.RetireTTL < 0 {
		return l, errors.New("mcp: subscription limits cannot be negative")
	}
	if l.MaxActive == 0 {
		l.MaxActive = 64
	}
	if l.MaxTracked == 0 {
		l.MaxTracked = 1024
	}
	if l.RetireTTL == 0 {
		l.RetireTTL = time.Minute
	}
	if l.MaxActive > l.MaxTracked {
		return l, errors.New("mcp: subscription active limit exceeds tracked limit")
	}
	return l, nil
}

func WithSubscriptionLimits(limits SubscriptionLimits) ClientOption {
	return func(options *ClientOptions) { options.Subscriptions = limits }
}

// Caller holds c.mu. Expired entries remain bounded until the next Listen.
func (c *Client) pruneRetiredSubscriptions(now time.Time) {
	for key, until := range c.retiredSubscriptions {
		if !now.Before(until) {
			delete(c.retiredSubscriptions, key)
		}
	}
}

func (c *Client) subscriptionRoute(key string) (*subscriptionState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if state := c.subscriptions[key]; state != nil {
		return state, false
	}
	until, retired := c.retiredSubscriptions[key]
	if retired && !time.Now().Before(until) {
		delete(c.retiredSubscriptions, key)
		retired = false
	}
	return nil, retired
}

func locallyCanceledSubscription(state *subscriptionState) bool {
	return state.localCanceled || state.ctx != nil && state.ctx.Err() != nil && state.err == nil
}

func cancelSubscriptionLocally(state *subscriptionState) {
	state.mu.Lock()
	state.localCanceled = true
	state.mu.Unlock()
	state.cancel()
}

func (c *Client) validateRetiredSubscriptionNotification(kind InvalidationKind, raw json.RawMessage) {
	if kind != InvalidationResource {
		return
	}
	var updated ResourceUpdatedParams
	if json.Unmarshal(raw, &updated) != nil || updated.URI == "" {
		c.failAllSubscriptions(errors.New("invalid retired resource subscription notification"))
	}
}
