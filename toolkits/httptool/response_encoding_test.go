package httptool

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestToolsPreserveUTF8OrRejectBeforeJSONReplacement(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, tc := range []struct {
			name, contentType string
			body              []byte
			valid             bool
		}{
			{"unicode", "text/plain; charset=utf-8", []byte("Привет 🌊\r\n\x00"), true},
			{"empty", "application/octet-stream", nil, true},
			{"valid_replacement_rune", "text/plain", []byte("\ufffd"), true},
			{"latin1_header_utf8_bytes", "text/plain; charset=iso-8859-1", []byte("café"), true},
			{"invalid_byte", "application/octet-stream", []byte{0xff}, false},
			{"latin1_byte", "text/plain; charset=iso-8859-1", []byte{0xe9}, false},
			{"incomplete_utf8", "text/plain", []byte{0xe2, 0x82}, false},
			{"utf16_bom", "text/plain; charset=utf-16", []byte{0xff, 0xfe, 'a', 0}, false},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				// Arrange: response headers never select decoding or replacement behavior.
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requests.Add(1)
					w.Header().Set("Content-Type", tc.contentType)
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write(tc.body)
				}))
				defer server.Close()
				tools, closeIdle, err := AsToolsWithCleanup(
					WithAllowedDomains([]string{"127.0.0.1"}),
					WithAllowPrivateIPs(true),
				)
				require.NoError(t, err)
				defer closeIdle()
				index := 0
				if method == http.MethodPost {
					index = 1
				}
				tool := toolsy.WithErrorFormatter()(tools[index])
				input, err := json.Marshal(getArgs{URL: server.URL})
				require.NoError(t, err)
				var chunks []toolsy.Chunk
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				// Act.
				err = tool.Execute(
					ctx,
					nil,
					toolsy.ToolInput{ArgsJSON: input},
					func(c toolsy.Chunk) error { chunks = append(chunks, c); return nil },
				)
				// Assert: an already-dispatched response cannot become an argument-repair chunk.
				require.EqualValues(t, 1, requests.Load())
				if tc.valid {
					require.NoError(t, err)
					require.Len(t, chunks, 1)
					result := decodeHTTPResult(t, chunks[0])
					require.Equal(t, http.StatusCreated, result.Status)
					require.True(t, bytes.Equal(tc.body, []byte(result.Body)))
					return
				}
				require.ErrorIs(t, err, ErrInvalidUTF8Response)
				require.Empty(t, chunks)
				var encoding *ResponseEncodingError
				require.ErrorAs(t, err, &encoding)
				require.Equal(t, method, encoding.Method)
				require.Equal(t, http.StatusCreated, encoding.Status)
				var phase *toolsy.ResultContractError
				require.ErrorAs(t, err, &phase)
				require.Equal(t, "http_response_encoding", phase.Kind)
				classified, ok := toolsy.AsToolError(err)
				require.True(t, ok)
				require.Equal(t, toolsy.CodeInternal, classified.Code)
				require.False(t, classified.Retryable)
				require.False(t, toolsy.ClientCorrectable(classified.Code))
			})
		}
	}
}

func TestPOSTResponseBudgetsPreserveAlreadyPerformedEffect(t *testing.T) {
	for _, tc := range []struct {
		name, body         string
		sourceCap, wireCap int
		wantKind           string
	}{
		{"body_exact", "é", 2, 100, ""},
		{"body_over", "é", 1, 100, "http_response_read"},
		{"escaped_wire_exact", "<<<<", 4, len(`{"status":200,"body":"\u003c\u003c\u003c\u003c"}`), ""},
		{"escaped_wire_one_over", "<<<<", 4, len(`{"status":200,"body":"\u003c\u003c\u003c\u003c"}`) - 1, "http_response_wire"},
		{"escaped_wire_over", "<<<<", 4, 30, "http_response_wire"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: the endpoint records its mutation before producing the response.
			var effects atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, _ *http.Request) { effects.Add(1); _, _ = w.Write([]byte(tc.body)) },
				),
			)
			defer server.Close()
			tools, closeIdle, err := AsToolsWithCleanup(
				WithAllowedDomains([]string{"127.0.0.1"}),
				WithAllowPrivateIPs(true),
				WithMaxResponseBody(tc.sourceCap),
				WithMaxWireBytes(tc.wireCap),
			)
			require.NoError(t, err)
			defer closeIdle()
			input, err := json.Marshal(postArgs{URL: server.URL, JSONBody: json.RawMessage(`{"approved":true}`)})
			require.NoError(t, err)
			yielded := 0
			// Act.
			err = toolsy.WithErrorFormatter()(
				tools[1],
			).Execute(t.Context(), nil, toolsy.ToolInput{ArgsJSON: input}, func(toolsy.Chunk) error { yielded++; return nil })
			// Assert: failure does not report rollback, dispatch twice or encourage self-correction.
			require.EqualValues(t, 1, effects.Load())
			if tc.wantKind == "" {
				require.NoError(t, err)
				require.Equal(t, 1, yielded)
				return
			}
			require.Error(t, err)
			require.Zero(t, yielded)
			require.ErrorIs(t, err, toolsy.ErrValidation)
			var phase *toolsy.ResultContractError
			require.ErrorAs(t, err, &phase)
			require.Equal(t, tc.wantKind, phase.Kind)
			classified, ok := toolsy.AsToolError(err)
			require.True(t, ok)
			require.Equal(t, toolsy.CodeInternal, classified.Code)
			require.False(t, classified.Retryable)
			require.False(t, toolsy.ClientCorrectable(classified.Code))
		})
	}
}

func TestResponseFailureCancellationPrecedesWireResult(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, failure := range []error{nil, toolsy.NewValidationError("wire result exceeds limit")} {
			// Arrange: cancellation can arrive after bounded reading but before wire formatting ends.
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			// Act.
			err := responseFailure(ctx, method, "http_response_wire", failure)
			// Assert: never return a success or a client-correctable wire failure after cancellation.
			require.ErrorIs(t, err, context.Canceled)
			require.NotErrorIs(t, err, toolsy.ErrValidation)
		}
	}
}
