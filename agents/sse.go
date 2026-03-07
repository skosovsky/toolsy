package agents

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const streamStepsPath = "/ap/v1/agent/tasks/%s/steps?stream=true"

// streamStepsOnce performs a single GET to the steps SSE endpoint, parses events, and yields steps.
// It returns the last event id (for reconnect), whether a terminal step (IsLast) was seen,
// whether at least one step was yielded, and any error.
func (c *Client) streamStepsOnce(ctx context.Context, taskID, lastEventID string, yield func(Step, error) bool) (lastID string, done bool, yieldedAny bool, err error) {
	urlStr := c.baseURL + fmt.Sprintf(streamStepsPath, url.PathEscape(taskID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", false, false, fmt.Errorf("agents: stream steps request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	if c.opts.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.opts.BearerToken)
	}
	// #nosec G704 -- baseURL is from caller config; caller is responsible for trust.
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", false, false, fmt.Errorf("agents: stream steps: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", false, false, fmt.Errorf("agents: stream steps: status %d", resp.StatusCode)
	}
	lastID, done, yieldedAny, err = parseSSESteps(resp.Body, yield)
	if err != nil {
		return lastID, done, yieldedAny, err
	}
	return lastID, done, yieldedAny, nil
}

// parseSSESteps reads SSE from r, parses each event's data as Step, and calls yield(step, nil).
// Yields at most one error (and then stops). Returns last event id, whether a step had IsLast,
// whether at least one step was yielded, and any parse/read error.
func parseSSESteps(r io.Reader, yield func(Step, error) bool) (lastID string, done bool, yieldedAny bool, err error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(nil, 1024*1024)
	var data, id string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if data != "" {
				var step Step
				if jerr := json.Unmarshal([]byte(data), &step); jerr != nil {
					if !yield(Step{}, fmt.Errorf("agents: parse step: %w", jerr)) {
						return lastID, done, yieldedAny, nil
					}
					return lastID, done, yieldedAny, jerr
				}
				if id != "" {
					lastID = id
				}
				if step.IsLast {
					done = true
				}
				if !yield(step, nil) {
					return lastID, done, true, nil
				}
				yieldedAny = true
			}
			data = ""
			id = ""
			continue
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "event:") {
			// event type is optional; we only care about id and data
			continue
		}
		if strings.HasPrefix(line, "data:") {
			part := strings.TrimSpace(line[5:])
			if data != "" {
				data += "\n"
			}
			data += part
			continue
		}
		if strings.HasPrefix(line, "id:") {
			id = strings.TrimSpace(line[3:])
			continue
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return lastID, done, yieldedAny, fmt.Errorf("agents: read stream: %w", scanErr)
	}
	return lastID, done, yieldedAny, nil
}

// StreamSteps connects to GET /ap/v1/agent/tasks/{task_id}/steps?stream=true and returns an iterator over steps.
// On connection drop (e.g. EOF), it reconnects with Last-Event-ID after a 1s backoff (respecting context).
func (c *Client) StreamSteps(ctx context.Context, taskID string) iter.Seq2[Step, error] {
	return func(yield func(Step, error) bool) {
		var lastID string
		firstRun := true
		for {
			var done, yieldedAny bool
			var err error
			lastID, done, yieldedAny, err = c.streamStepsOnce(ctx, taskID, lastID, yield)
			if err != nil {
				yield(Step{}, err)
				return
			}
			if done {
				return
			}
			// After reconnect, if server returns no steps, avoid infinite loop.
			if !firstRun && !yieldedAny {
				return
			}
			firstRun = false
			if ctx.Err() != nil {
				return
			}
			// Backoff before reconnect to avoid hammering the server.
			timer := time.NewTimer(1 * time.Second)
			select {
			case <-timer.C:
				// continue with lastID
			case <-ctx.Done():
				timer.Stop()
				return
			}
			timer.Stop()
		}
	}
}
