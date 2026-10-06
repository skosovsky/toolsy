package httptool

import (
	"context"
	"errors"
	"net"
	"slices"
	"testing"
	"time"
)

func TestPinnedDialAttemptsCheckedAddresses(t *testing.T) {
	// Arrange: first address is unavailable, second is usable; no name is passed to dial.
	ips := []net.IPAddr{{IP: net.ParseIP("192.0.2.1")}, {IP: net.ParseIP("192.0.2.2")}}
	var addresses []string
	firstFailure := errors.New("first unavailable")
	peer, usable := net.Pipe()
	defer peer.Close()
	defer usable.Close()
	// Act.
	connection, err := dialPinnedAddresses(t.Context(), "tcp", ips, "443", time.Second,
		func(ctx context.Context, _ string, address string) (net.Conn, error) {
			addresses = append(addresses, address)
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > time.Second {
				t.Error("missing attempt bound")
			}
			if len(addresses) == 1 {
				return nil, firstFailure
			}
			return usable, nil
		})
	// Assert.
	if err != nil || connection != usable || !slices.Equal(addresses, []string{"192.0.2.1:443", "192.0.2.2:443"}) {
		t.Fatalf("conn=%v err=%v addresses=%v", connection, err, addresses)
	}
}

func TestPinnedDialTotalTimeoutAndCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "canceled"}[canceled], func(t *testing.T) {
			// Arrange.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if canceled {
				cancel()
			}
			ips := []net.IPAddr{{IP: net.ParseIP("192.0.2.1")}, {IP: net.ParseIP("192.0.2.2")}}
			calls := 0
			started := time.Now()
			// Act.
			conn, err := dialPinnedAddresses(ctx, "tcp", ips, "443", 40*time.Millisecond,
				func(attempt context.Context, _, _ string) (net.Conn, error) {
					calls++
					<-attempt.Done()
					return nil, attempt.Err()
				})
			// Assert.
			expected := context.DeadlineExceeded
			if canceled {
				expected = context.Canceled
			}
			if conn != nil || !errors.Is(err, expected) || time.Since(started) > time.Second {
				t.Fatalf("conn=%v err=%v elapsed=%s", conn, err, time.Since(started))
			}
			if (canceled && calls != 0) || (!canceled && calls != 2) {
				t.Fatalf("attempts=%d canceled=%v", calls, canceled)
			}
		})
	}
}

func TestSafeDialChecksAllIPsBeforeAnyAttempt(t *testing.T) {
	// Arrange: one public address does not excuse a private address in the same answer.
	lookups, attempts := 0, 0
	dial := safeDialContextWithPorts(IsBlockedIP, false, time.Second, normalizeHostPolicy(nil, nil),
		func(context.Context, string) ([]net.IPAddr, error) {
			lookups++
			return []net.IPAddr{{IP: net.ParseIP("192.0.2.1")}, {IP: net.ParseIP("127.0.0.1")}}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			attempts++
			return nil, errors.New("must not dial")
		})
	// Act.
	conn, err := dial(t.Context(), "tcp", "fixture.invalid:443")
	// Assert.
	if conn != nil || err == nil || lookups != 1 || attempts != 0 {
		t.Fatalf("conn=%v err=%v lookups=%d attempts=%d", conn, err, lookups, attempts)
	}
}

func TestSafeDialPinsOneDNSAnswerAcrossAttempts(t *testing.T) {
	// Arrange.
	lookups, attempts := 0, 0
	peer, expected := net.Pipe()
	defer peer.Close()
	defer expected.Close()
	dial := safeDialContextWithPorts(IsBlockedIP, false, time.Second, normalizeHostPolicy(nil, nil),
		func(context.Context, string) ([]net.IPAddr, error) {
			lookups++
			return []net.IPAddr{{IP: net.ParseIP("192.0.2.1")}, {IP: net.ParseIP("192.0.2.2")}}, nil
		},
		func(_ context.Context, _ string, address string) (net.Conn, error) {
			attempts++
			if attempts == 1 {
				if address != "192.0.2.1:443" {
					t.Error(address)
				}
				return nil, errors.New("unavailable")
			}
			if address != "192.0.2.2:443" {
				t.Error(address)
			}
			return expected, nil
		})
	// Act.
	conn, err := dial(t.Context(), "tcp", "fixture.invalid:443")
	// Assert.
	if conn != expected || err != nil || lookups != 1 || attempts != 2 {
		t.Fatalf("conn=%v err=%v lookups=%d attempts=%d", conn, err, lookups, attempts)
	}
}
