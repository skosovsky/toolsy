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

const (
	streamStepsPath = "/ap/v1/agent/tasks/%s/steps?stream=true"

	// maxSSEScanBytes is the max line buffer for [bufio.Scanner] when reading SSE (1 MiB).
	maxSSEScanBytes = 1024 * 1024
)

// streamStepsOnce performs a single GET to the steps SSE endpoint, parses events, and yields steps.
// It returns the last event id (for reconnect), whether a terminal step (IsLast) was seen,
// whether at least one step was yielded, and any error.
func (c *Client) streamStepsOnce(
	ctx context.Context,
	taskID, lastEventID string,
	authHeader string,
	yield func(Step, error) bool,
) (string, bool, bool, error) {
	urlStr := c.baseURL + fmt.Sprintf(streamStepsPath, url.PathEscape(taskID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", false, false, fmt.Errorf("agents: stream steps request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
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
	lastID, done, yieldedAny, err := parseSSESteps(resp.Body, yield)
	if err != nil {
		return lastID, done, yieldedAny, err
	}
	return lastID, done, yieldedAny, nil
}

// emitStepFromSSEData parses accumulated SSE data as JSON Step, updates lastID/done, and yields.
// If the consumer stops after a successful step yield, yieldStopped is true (caller should return
// yieldedAny=true). If the consumer stops after an error yield, yieldStopped is false (caller
// keeps the current yieldedAny).
func emitStepFromSSEData(
	data, id string,
	lastID *string,
	done *bool,
	yieldedAny *bool,
	yield func(Step, error) bool,
) (bool, bool, error) {
	var step Step
	if jerr := json.Unmarshal([]byte(data), &step); jerr != nil {
		var zero Step
		if !yield(zero, fmt.Errorf("agents: parse step: %w", jerr)) {
			return true, false, nil
		}
		return false, false, jerr
	}
	if id != "" {
		*lastID = id
	}
	if step.IsLast {
		*done = true
	}
	if !yield(step, nil) {
		return true, true, nil
	}
	*yieldedAny = true
	return false, false, nil
}

// consumeSSELine applies one non-empty SSE line to the data/id buffers (field: value format).
func consumeSSELine(line string, data, id *string) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "event:") {
		// event type is optional; we only care about id and data
		return
	}
	if strings.HasPrefix(line, "data:") {
		part := strings.TrimSpace(line[5:])
		if *data != "" {
			*data += "\n"
		}
		*data += part
		return
	}
	if strings.HasPrefix(line, "id:") {
		*id = strings.TrimSpace(line[3:])
	}
}

// parseSSESteps reads SSE from r, parses each event's data as Step, and calls yield(step, nil).
// Yields at most one error (and then stops). Returns last event id, whether a step had IsLast,
// whether at least one step was yielded, and any parse/read error.
func parseSSESteps(r io.Reader, yield func(Step, error) bool) (string, bool, bool, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(nil, maxSSEScanBytes)
	var data, id string
	var lastID string
	var done, yieldedAny bool
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			consumeSSELine(line, &data, &id)
			continue
		}
		// Blank line: end of SSE event.
		if data == "" {
			continue
		}
		stop, yieldStopped, emitErr := emitStepFromSSEData(data, id, &lastID, &done, &yieldedAny, yield)
		if emitErr != nil {
			return lastID, done, yieldedAny, emitErr
		}
		if stop {
			if yieldStopped {
				return lastID, done, true, nil
			}
			return lastID, done, yieldedAny, nil
		}
		data = ""
		id = ""
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return lastID, done, yieldedAny, fmt.Errorf("agents: read stream: %w", scanErr)
	}
	return lastID, done, yieldedAny, nil
}

// StreamSteps connects to GET /ap/v1/agent/tasks/{task_id}/steps?stream=true and returns an iterator over steps.
// On connection drop (e.g. EOF), it reconnects with Last-Event-ID after a 1s backoff (respecting context).
func (c *Client) StreamSteps(ctx context.Context, taskID, authHeader string) iter.Seq2[Step, error] {
	return func(yield func(Step, error) bool) {
		var lastID string
		firstRun := true
		var zero Step
		for {
			var done, yieldedAny bool
			var err error
			lastID, done, yieldedAny, err = c.streamStepsOnce(ctx, taskID, lastID, authHeader, yield)
			if err != nil {
				yield(zero, err)
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
