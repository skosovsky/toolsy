// Package agents provides an Agent Protocol bridge for toolsy: REST/SSE client,
// task lifecycle, and delegation as toolsy.Tool (AsTool, AsBackgroundTool).
package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

// ClientOptions configures the Agent Protocol HTTP client.
type ClientOptions struct {
	HTTPClient      *http.Client
	allowPrivateIPs bool
}

// WithAllowPrivateIPs relaxes SSRF IP blocking on the default safe transport (tests and private networks).
func WithAllowPrivateIPs(allow bool) func(*ClientOptions) {
	return func(o *ClientOptions) {
		o.allowPrivateIPs = allow
	}
}

// Client is the REST client for the Agent Protocol API.
type Client struct {
	baseURL string
	opts    ClientOptions
}

// WithHTTPClient sets a custom HTTP client (e.g. for TLS timeout). Only Timeout is merged onto
// the default SSRF-safe client; custom Transport is ignored.
func WithHTTPClient(client *http.Client) func(*ClientOptions) {
	return func(o *ClientOptions) {
		o.HTTPClient = client
	}
}

// NewClient creates a client for the Agent Protocol server at baseURL. Options can customize the HTTP client.
func NewClient(baseURL string, opts ...func(*ClientOptions)) *Client {
	var o ClientOptions
	for _, opt := range opts {
		opt(&o)
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	return &Client{baseURL: baseURL, opts: o}
}

func (c *Client) httpClient() *http.Client {
	return httptool.MergeHTTPClient(defaultHTTPClient(c.opts.allowPrivateIPs), c.opts.HTTPClient)
}

// CreateTask sends POST /ap/v1/agent/tasks with body {"input": args}. Returns the created task or error.
func (c *Client) CreateTask(ctx context.Context, args json.RawMessage, authHeader string) (*Task, error) {
	body := createTaskRequest{Input: args}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("agents: marshal create task: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/ap/v1/agent/tasks", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("agents: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	// #nosec G704 -- baseURL is from caller config; caller is responsible for trust.
	resp, err := c.httpClient().Do(req) //nolint:bodyclose // closed via httptool.CloseResponseBody
	if err != nil {
		return nil, fmt.Errorf("agents: create task: %w", err)
	}
	defer httptool.CloseResponseBody(ctx, resp.Body)
	if !httptool.IsSuccessStatus(resp.StatusCode) {
		return nil, fmt.Errorf("agents: create task: status %d", resp.StatusCode)
	}
	bodyBytes, err := textprocessor.ReadLimitedBytes(ctx, resp.Body, defaultMaxResponseBytes)
	if err != nil {
		if strings.Contains(err.Error(), "read exceeds") {
			return nil, toolsy.NewValidationError("response body too large")
		}
		return nil, fmt.Errorf("agents: read create task response: %w", err)
	}
	// Try wrapped response {"task": {...}} first.
	var wrapped createTaskResponse
	if err := json.Unmarshal(bodyBytes, &wrapped); err == nil && wrapped.Task != nil {
		return wrapped.Task, nil
	}
	// Else decode as raw Task (server returns task object directly).
	var task Task
	if err := json.Unmarshal(bodyBytes, &task); err != nil {
		return nil, fmt.Errorf("agents: decode create task result: %w", err)
	}
	return &task, nil
}

// CancelTask sends POST /ap/v1/agent/tasks/{task_id}/cancel to cancel the task on the server.
func (c *Client) CancelTask(ctx context.Context, taskID string, authHeader string) error {
	reqURL := c.baseURL + "/ap/v1/agent/tasks/" + url.PathEscape(taskID) + "/cancel"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("agents: create cancel request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	// #nosec G704 -- baseURL is from caller config; caller is responsible for trust.
	resp, err := c.httpClient().Do(req) //nolint:bodyclose // closed via httptool.CloseResponseBody
	if err != nil {
		return fmt.Errorf("agents: cancel task: %w", err)
	}
	defer httptool.CloseResponseBody(ctx, resp.Body)
	if !httptool.IsSuccessStatus(resp.StatusCode) {
		return fmt.Errorf("agents: cancel task: status %d", resp.StatusCode)
	}
	return nil
}
