package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type fragmentedReader struct {
	raw     []byte
	offset  int
	pattern []int
	step    int
}

func (r *fragmentedReader) Read(target []byte) (int, error) {
	if r.offset == len(r.raw) {
		return 0, io.EOF
	}
	limit := r.pattern[r.step%len(r.pattern)]
	r.step++
	if limit > len(target) {
		limit = len(target)
	}
	if remaining := len(r.raw) - r.offset; limit > remaining {
		limit = remaining
	}
	copy(target, r.raw[r.offset:r.offset+limit])
	r.offset += limit
	return limit, nil
}

func TestTask34SSEAcceptsBOMLineEndingsAndFragmentedFrames(t *testing.T) {
	for name, separator := range map[string]string{
		"LF":   "\n",
		"CR":   "\r",
		"CRLF": "\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			// Arrange.
			peer := newRPCPeer(t.Context(), nil, func(context.Context, []byte) error { return nil })
			pending, _, err := peer.beginRequest(MethodServerDiscover, DiscoverParams{
				Meta: requestMetaForContract(),
			})
			require.NoError(t, err)
			transport := NewStreamableHTTPTransport("https://example.test/mcp")
			transport.peer = peer
			response := []byte(`{"jsonrpc":"2.0","id":` + string(pending.ID()) +
				`,"result":{"resultType":"complete"}}`)
			stream := append([]byte{0xef, 0xbb, 0xbf}, []byte("data: ")...)
			stream = append(stream, response...)
			stream = append(stream, []byte(separator+separator)...)
			reader := &fragmentedReader{raw: stream, pattern: []int{1, 2, 5, 3, 8}}
			provenance := &sseRequestProvenance{
				requestID: bytes.Clone(pending.ID()),
				method:    MethodServerDiscover,
			}

			// Act.
			consumeErr := transport.consumeRequestSSE(t.Context(), reader, provenance)
			result, awaitErr := pending.Await(t.Context())

			// Assert.
			require.NoError(t, consumeErr)
			require.NoError(t, awaitErr)
			require.JSONEq(t, `{"resultType":"complete"}`, string(result))
			peer.close(ErrTransportClosed)
		})
	}
}

func TestTask34SSERejectsTruncatedUTF8AcrossFragments(t *testing.T) {
	// Arrange.
	transport := NewStreamableHTTPTransport("https://example.test/mcp")
	transport.peer = newRPCPeer(t.Context(), nil, func(context.Context, []byte) error { return nil })
	raw := append([]byte("data: "), 0xe2, 0x82)
	raw = append(raw, '\n', '\n')
	reader := &fragmentedReader{raw: raw, pattern: []int{1}}
	provenance := &sseRequestProvenance{
		requestID: json.RawMessage(`1`),
		method:    MethodServerDiscover,
	}

	// Act.
	err := transport.consumeRequestSSE(t.Context(), reader, provenance)

	// Assert.
	require.ErrorContains(t, err, "not valid UTF-8")
	transport.peer.close(ErrTransportClosed)
}
