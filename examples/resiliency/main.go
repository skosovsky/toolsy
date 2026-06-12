// Package main shows wrapping a toolsy HTTP tool with routery policies (timeout, retry, bulkhead)
// and calling it via Session.RunCall (toolsy v1.0 host-facing API).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/skosovsky/routery"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

const (
	policyTimeout       = 8 * time.Second
	policyRetryAttempts = 3
	policyRetryBackoff  = 80 * time.Millisecond
	policyBulkheadLimit = 4
)

// httpGetArgs matches the http_get tool input schema.
type httpGetArgs struct {
	URL string `json:"url"`
}

// httpGetResult matches the http_get tool JSON result (status + body).
type httpGetResult struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// execReq carries typed args plus [*toolsy.RunEnv] so credentials and attachments
// reach the underlying toolkit while [routery.Executor] stays generic.
type execReq struct {
	Env  *toolsy.RunEnv
	Args httpGetArgs
}

func main() {
	if err := run(); err != nil {
		log.SetOutput(os.Stderr)
		log.Println(err)
		os.Exit(1)
	}
}

func run() error {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	defer srv.Close()

	tools, err := httptool.AsTools(
		httptool.WithAllowedDomains([]string{"127.0.0.1"}),
		httptool.WithAllowPrivateIPs(true),
	)
	if err != nil {
		return fmt.Errorf("httptool: %w", err)
	}

	baseGet, err := findHTTPGetTool(tools)
	if err != nil {
		return err
	}

	reliable := buildReliableExecutor(baseGet)
	wrapped, err := buildWrappedTool(reliable)
	if err != nil {
		return err
	}

	reg, err := toolsy.NewRegistryBuilder().Add(wrapped).Use(toolsy.WithErrorFormatter()).Build()
	if err != nil {
		return fmt.Errorf("registry: %w", err)
	}

	sess, err := toolsy.NewSession(reg)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}

	call := toolsy.ToolCall{
		ToolName: wrapped.Manifest().Name,
		Input: toolsy.ToolInput{
			CallID:   "demo",
			ArgsJSON: fmt.Appendf(nil, `{"url":%q}`, srv.URL),
		},
		Env: toolsy.NewRunEnv(sess),
	}

	outcome, err := sess.RunCall(context.Background(), call)
	if err != nil {
		return fmt.Errorf("infrastructure failure: %w", err)
	}
	if outcome.ExecutionError != nil {
		te, ok := toolsy.AsToolError(outcome.ExecutionError)
		if ok {
			return fmt.Errorf("tool failure [%s]: %s", te.Code, te.Reason)
		}
		return fmt.Errorf("tool failure: %w", outcome.ExecutionError)
	}

	out, decErr := toolsy.DecodeOutcomeAs[httpGetResult](outcome)
	if decErr != nil {
		return fmt.Errorf("decode outcome: %w", decErr)
	}
	_, err = fmt.Fprintf(os.Stdout, "status=%d body=%s\n", out.Status, out.Body)
	return err
}

func findHTTPGetTool(tools []toolsy.Tool) (toolsy.Tool, error) {
	for _, t := range tools {
		if t.Manifest().Name == "http_get" {
			return t, nil
		}
	}
	return nil, errors.New("http_get tool not found")
}

func buildReliableExecutor(baseGet toolsy.Tool) routery.Executor[execReq, httpGetResult] {
	base := routery.ExecutorFunc[execReq, httpGetResult](func(ctx context.Context, req execReq) (httpGetResult, error) {
		raw, marshalErr := json.Marshal(req.Args)
		if marshalErr != nil {
			return httpGetResult{}, marshalErr
		}
		var resultChunk toolsy.Chunk
		execToolErr := baseGet.Execute(ctx, req.Env, toolsy.ToolInput{ArgsJSON: raw}, func(c toolsy.Chunk) error {
			resultChunk = c
			return nil
		})
		if execToolErr != nil {
			return httpGetResult{}, execToolErr
		}
		decoded, decErr := toolsy.DecodeChunkAs[httpGetResult](resultChunk)
		if decErr != nil {
			return httpGetResult{}, decErr
		}
		return *decoded, nil
	})

	return routery.Apply(base,
		routery.Timeout[execReq, httpGetResult](policyTimeout),
		routery.RetryIf[execReq, httpGetResult](policyRetryAttempts, policyRetryBackoff, retryOnNetTimeout),
		routery.Bulkhead[execReq, httpGetResult](policyBulkheadLimit),
	)
}

func retryOnNetTimeout(_ context.Context, _ execReq, execErr error) bool {
	if execErr == nil {
		return false
	}
	var ne net.Error
	return errors.As(execErr, &ne) && ne.Timeout()
}

func buildWrappedTool(reliable routery.Executor[execReq, httpGetResult]) (toolsy.Tool, error) {
	return toolsy.NewTypedTool(toolsy.TypedToolSpec[httpGetArgs, httpGetResult]{
		Name:        "http_get_resilient",
		Description: "HTTP GET with routery timeout, retry, and bulkhead",
		Handler: func(ctx context.Context, run *toolsy.RunEnv, args httpGetArgs) (httpGetResult, error) {
			return reliable.Execute(ctx, execReq{Env: run, Args: args})
		},
	})
}
