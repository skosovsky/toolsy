// Package agents provides an Agent Protocol bridge for toolsy: REST/SSE client,
// task lifecycle, and delegation as toolsy.Tool (AsTool, AsBackgroundTool).
package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ClientOptions configures the Agent Protocol HTTP client.
type ClientOptions struct {
	HTTPClient *http.Client
}

// Client is the REST client for the Agent Protocol API.
type Client struct {
	baseURL string
	opts    ClientOptions
}

// WithHTTPClient sets a custom HTTP client (e.g. for TLS). If nil, [http.DefaultClient] is used.
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
	if c.opts.HTTPClient != nil {
		return c.opts.HTTPClient
	}
	return http.DefaultClient
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
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("agents: create task: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("agents: create task: status %d", resp.StatusCode)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
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
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("agents: cancel task: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// Drain body so the connection can be returned to the pool (Keep-Alive).
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("agents: cancel task: status %d", resp.StatusCode)
	}
	return nil
}
