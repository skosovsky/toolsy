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

// execReq carries validated args plus [toolsy.RunContext] so credentials and attachments
// reach the underlying toolkit while [routery.Executor] stays generic.
type execReq struct {
	Run  toolsy.RunContext
	Args map[string]any
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
	var result []byte
	execErr := reg.Execute(context.Background(), call, func(c toolsy.Chunk) error {
		result = append(result, c.Data...)
		return nil
	})
	if execErr != nil {
		return fmt.Errorf("execute: %w", execErr)
	}
	_, err = fmt.Fprintln(os.Stdout, string(result))
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

func buildReliableExecutor(baseGet toolsy.Tool) routery.Executor[execReq, any] {
	base := routery.ExecutorFunc[execReq, any](func(ctx context.Context, req execReq) (any, error) {
		raw, marshalErr := json.Marshal(req.Args)
		if marshalErr != nil {
			return nil, marshalErr
		}
		var buf []byte
		execToolErr := baseGet.Execute(ctx, req.Run, toolsy.ToolInput{ArgsJSON: raw}, func(c toolsy.Chunk) error {
			buf = append(buf, c.Data...)
			return nil
		})
		if execToolErr != nil {
			return nil, execToolErr
		}
		var v any
		unmarshalErr := json.Unmarshal(buf, &v)
		if unmarshalErr != nil {
			return nil, unmarshalErr
		}
		return v, nil
	})

	return routery.Apply(base,
		routery.Timeout[execReq, any](policyTimeout),
		routery.RetryIf[execReq, any](policyRetryAttempts, policyRetryBackoff, retryOnNetTimeout),
		routery.Bulkhead[execReq, any](policyBulkheadLimit),
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
	reliable routery.Executor[execReq, any],
) (toolsy.Tool, error) {
	manifest := baseGet.Manifest()
	return toolsy.NewDynamicTool(
		manifest.Name+"_resilient",
		manifest.Description+" (routery: timeout, retry, bulkhead)",
		manifest.Parameters,
		func(ctx context.Context, run toolsy.RunContext, argsJSON []byte, yield func(toolsy.Chunk) error) error {
			var args map[string]any
			if unmarshalArgsErr := json.Unmarshal(argsJSON, &args); unmarshalArgsErr != nil {
				return unmarshalArgsErr
			}
			out, execReliableErr := reliable.Execute(ctx, execReq{Run: run, Args: args})
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
