// Package testhttp supplies connection accounting for public HTTP adapter tests.
package testhttp

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Pool counts actual accepted and closed connections, rather than client allocations.
type Pool struct {
	Server *httptest.Server
	opened atomic.Int64
	closed atomic.Int64
}

// NewPool starts a keep-alive server and registers its cleanup.
func NewPool(t testing.TB, handler http.Handler) *Pool {
	t.Helper()
	pool := &Pool{Server: httptest.NewUnstartedServer(handler), opened: atomic.Int64{}, closed: atomic.Int64{}}
	pool.Server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			pool.opened.Add(1)
		}
		if state == http.StateClosed {
			pool.closed.Add(1)
		}
	}
	pool.Server.Start()
	t.Cleanup(pool.Server.Close)
	return pool
}

// AssertReusedAndClosed verifies reuse and explicit owned-idle disposal.
func (p *Pool) AssertReusedAndClosed(t testing.TB, closeIdle func()) {
	t.Helper()
	if opened := p.opened.Load(); opened != 1 {
		t.Fatalf("connections opened=%d, want 1", opened)
	}
	closeIdle()
	deadline := time.Now().Add(2 * time.Second)
	for p.closed.Load() != p.opened.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if closed := p.closed.Load(); closed != 1 {
		t.Fatalf("idle connections closed=%d, want 1", closed)
	}
}
