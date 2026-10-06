package main

import (
	"context"
	"errors"
	"testing"

	"github.com/skosovsky/routery"

	"github.com/skosovsky/toolsy"
)

func TestWrappedRouteResultContract(t *testing.T) {
	// Arrange.
	cause := errors.New("route failed")
	for _, fixture := range []struct {
		name      string
		result    routery.BasicRouteResult[httpGetResult]
		err       error
		wantError bool
	}{
		{name: "handled", result: routery.BasicHandled(httpGetResult{Status: 200, Body: "ok"})},
		{name: "ignored", result: routery.BasicIgnored[httpGetResult](routery.BasicReasonNoMatch), wantError: true},
		{name: "fallthrough", result: routery.BasicRouteResult[httpGetResult]{Action: routery.ActionNext}, wantError: true},
		{name: "failure", err: cause, wantError: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			tool, err := buildWrappedTool(
				func(routery.RouteCall[execReq]) (routery.BasicRouteResult[httpGetResult], error) {
					return fixture.result, fixture.err
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			var chunks []toolsy.Chunk

			// Act.
			err = tool.Execute(context.Background(), toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(`{"url":"https://example.com"}`)},
				func(chunk toolsy.Chunk) error { chunks = append(chunks, chunk); return nil })

			// Assert.
			if fixture.wantError {
				if err == nil {
					t.Fatal("non-result route accepted")
				}
				if fixture.err != nil && !errors.Is(err, fixture.err) {
					t.Fatalf("lost route cause: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertRoutePayload(t, chunks, fixture.result.Payload)
		})
	}
}

func assertRoutePayload(t *testing.T, chunks []toolsy.Chunk, want httpGetResult) {
	t.Helper()
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks", len(chunks))
	}
	result, err := toolsy.DecodeChunkAs[httpGetResult](chunks[0])
	if err != nil {
		t.Fatal(err)
	}
	if *result != want {
		t.Fatalf("result changed: %+v", result)
	}
}
