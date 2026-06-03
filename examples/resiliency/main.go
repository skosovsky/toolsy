// Package main shows wrapping a toolsy HTTP tool with routery policies (timeout, retry, bulkhead)
// and registering the wrapped tool in a registry.
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
	wrapped, err := buildWrappedTool(baseGet, reliable)
	if err != nil {
		return err
	}

	reg, err := toolsy.NewRegistryBuilder().Add(wrapped).Build()
	if err != nil {
		return fmt.Errorf("registry: %w", err)
	}

	call := toolsy.ToolCall{
		ToolName: wrapped.Manifest().Name,
		Input: toolsy.ToolInput{
			CallID:   "demo",
			ArgsJSON: fmt.Appendf(nil, `{"url":%q}`, srv.URL),
		},
	}
	var resultChunk toolsy.Chunk
	execErr := reg.Execute(context.Background(), call, func(c toolsy.Chunk) error {
		resultChunk = c
		return nil
	})
	if execErr != nil {
		return fmt.Errorf("execute: %w", execErr)
	}
	out, decErr := toolsy.DecodeChunkAs[httpGetResult](resultChunk)
	if decErr != nil {
		return fmt.Errorf("decode result: %w", decErr)
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

func buildWrappedTool(
	baseGet toolsy.Tool,
	reliable routery.Executor[execReq, httpGetResult],
) (toolsy.Tool, error) {
	manifest := baseGet.Manifest()
	return toolsy.NewDynamicTool(
		manifest.Name+"_resilient",
		manifest.Description+" (routery: timeout, retry, bulkhead)",
		manifest.Parameters,
		func(ctx context.Context, run *toolsy.RunEnv, argsJSON []byte, yield func(toolsy.Chunk) error) error {
			var args httpGetArgs
			if unmarshalArgsErr := json.Unmarshal(argsJSON, &args); unmarshalArgsErr != nil {
				return toolsy.NewJSONParseError(unmarshalArgsErr)
			}
			out, execReliableErr := reliable.Execute(ctx, execReq{Env: run, Args: args})
			if execReliableErr != nil {
				return execReliableErr
			}
			data, marshalOutErr := json.Marshal(out)
			if marshalOutErr != nil {
				return marshalOutErr
			}
			return yield(toolsy.Chunk{
				Event:    toolsy.EventResult,
				Data:     data,
				MimeType: toolsy.MimeTypeJSON,
			})
		},
	)
}
