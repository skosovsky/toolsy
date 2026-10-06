package agents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

const (
	streamStepsPath = "/ap/v1/agent/tasks/%s/steps?stream=true"

	// maxSSEScanBytes is the max line buffer for [bufio.Scanner] when reading SSE (1 MiB).
	maxSSEScanBytes        = 1024 * 1024
	maxSupportedEventBytes = 16 * 1024 * 1024
	defaultStreamTimeout   = 5 * time.Minute
)

// streamStepsOnce performs a single GET to the steps SSE endpoint, parses events, and yields steps.
// It returns the last event id (for reconnect), whether a terminal step (IsLast) was seen,
// whether at least one step was yielded, and any error.
func (c *Client) streamStepsOnce(
	ctx context.Context,
	taskID, lastEventID string,
	authHeader string,
	budget *streamBudget,
	state *streamState,
	yield func(Step, error) bool,
) (string, bool, error) {
	urlStr := c.baseURL + fmt.Sprintf(streamStepsPath, url.PathEscape(taskID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", false, fmt.Errorf("agents: stream steps request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	// #nosec G704 -- baseURL is from caller config; caller is responsible for trust.
	resp, err := c.httpClient().Do(req) //nolint:bodyclose // closed via httptool.CloseResponseBody
	if err != nil {
		return "", false, fmt.Errorf("agents: stream steps: %w", err)
	}
	defer httptool.CloseResponseBody(ctx, resp.Body)
	if !httptool.IsSuccessStatus(resp.StatusCode) {
		return "", false, fmt.Errorf("agents: stream steps: status %d", resp.StatusCode)
	}
	limitedBody := budget.reader(ctx, resp.Body)
	lastID, done, _, err := parseSSEStepsWithState(
		ctx,
		limitedBody,
		c.opts.streamPolicy.MaxEventBytes,
		state,
		yield,
	)
	if err != nil {
		return lastID, done, err
	}
	return lastID, done, nil
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
		return false, false, &RemoteOutcomeError{
			Phase:   PhaseObserve,
			Outcome: OutcomeMalformed,
			TaskID:  "",
			Step:    nil,
			Cause:   jerr,
		}
	}
	if err := validateStep(step); err != nil {
		return false, false, err
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
	if strings.HasPrefix(line, ":") {
		return
	}
	if strings.HasPrefix(line, "event:") {
		// event type is optional; we only care about id and data
		return
	}
	if strings.HasPrefix(line, "data:") {
		part := strings.TrimPrefix(line[5:], " ")
		if *data != "" {
			*data += "\n"
		}
		*data += part
		return
	}
	if line == "id" {
		*id = ""
		return
	}
	if strings.HasPrefix(line, "id:") {
		value := strings.TrimPrefix(line[3:], " ")
		if !strings.ContainsRune(value, 0) {
			*id = value
		}
	}
}

// parseSSESteps reads SSE from r, parses each event's data as Step, and calls yield(step, nil).
// Yields at most one error (and then stops). Returns last event id, whether a step had IsLast,
// whether at least one step was yielded, and any parse/read error.
func parseSSESteps(
	ctx context.Context,
	r io.Reader,
	maxStreamBytes int,
	yield func(Step, error) bool,
) (string, bool, bool, error) {
	return parseSSEStepsWithState(ctx, r, maxStreamBytes, nil, yield)
}

//nolint:gocognit,funlen // The bounded event parser keeps dispatch and EOF behavior in one place.
func parseSSEStepsWithState(
	ctx context.Context,
	r io.Reader,
	maxStreamBytes int,
	state *streamState,
	yield func(Step, error) bool,
) (string, bool, bool, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(nil, maxStreamBytes+1)
	scanner.Split(splitSSELines)
	var data, id string
	var lastID string
	if state != nil {
		lastID = state.lastID
	}
	var idPresent bool
	var done, yieldedAny bool
	eventBytes := 0
	for scanner.Scan() {
		if ctx.Err() != nil {
			return lastID, done, yieldedAny, ctx.Err()
		}
		token := scanner.Text()
		if !utf8.ValidString(token) {
			return lastID, done, yieldedAny, &RemoteOutcomeError{
				Phase:   PhaseObserve,
				Outcome: OutcomeMalformed,
				TaskID:  "",
				Step:    nil,
				Cause:   errors.New("agents: invalid UTF-8 SSE frame"),
			}
		}
		eventBytes += len(token)
		line := strings.TrimSuffix(strings.TrimSuffix(token, "\n"), "\r")
		if eventBytes > maxStreamBytes {
			return lastID, done, yieldedAny, textprocessor.NewReadLimitError("SSE event", maxStreamBytes, nil)
		}
		if line != "" {
			if (line == "id" || strings.HasPrefix(line, "id:")) && !strings.ContainsRune(line, 0) {
				idPresent = true
			}
			consumeSSELine(line, &data, &id)
			continue
		}
		// Blank line: end of SSE event.
		eventBytes = 0
		if data == "" {
			if idPresent {
				lastID = id
			}
			idPresent = false
			id = ""
			continue
		}
		if idPresent {
			lastID = id
		}
		idPresent = false
		if state != nil && id != "" {
			if previous, exists := state.seen[id]; exists {
				if previous != data {
					return lastID, done, yieldedAny, &RemoteOutcomeError{Phase: PhaseObserve,
						Outcome: OutcomeMalformed,
						TaskID:  state.taskID,
						Step:    nil,
						Cause:   errors.New("agents: conflicting event ID"),
					}
				}
				data = ""
				id = ""
				continue
			}
			state.seen[id] = data
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
		if done {
			return lastID, true, yieldedAny, nil
		}
		data = ""
		id = ""
	}
	scanErr := scanner.Err()
	if errors.Is(scanErr, bufio.ErrTooLong) {
		scanErr = textprocessor.NewReadLimitError("SSE event", maxStreamBytes, nil)
	}
	return lastID, done, yieldedAny, classifySSEScanError(ctx, scanErr, maxStreamBytes)
}

func classifySSEScanError(ctx context.Context, scanErr error, maxStreamBytes int) error {
	if scanErr == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if toolsy.IsContextInterrupt(scanErr) {
		return scanErr
	}
	if textprocessor.IsReadLimitExceeded(scanErr) {
		return fmt.Errorf(
			"agents: stream exceeds %d byte limit: %w",
			maxStreamBytes,
			scanErr,
		)
	}
	return fmt.Errorf("agents: read stream: %w", scanErr)
}

// StreamSteps reads the explicitly documented toolsy step-stream v1 extension.
// Reconnect resumes observations only; it never creates or executes remote tasks.
//
//nolint:gocognit // Sequential read/reconnect gates preserve callback cancellation and shared budget.
func (c *Client) StreamSteps(ctx context.Context, taskID, authHeader string) iter.Seq2[Step, error] {
	return func(yield func(Step, error) bool) {
		var zero Step
		if err := c.validateStreamPolicy(); err != nil {
			yield(zero, err)
			return
		}
		streamCtx, cancel := context.WithTimeout(ctx, c.opts.streamPolicy.Timeout)
		defer cancel()
		budget := streamBudget{remaining: c.maxSSEStreamBytes(), limit: c.maxSSEStreamBytes()}
		state := streamState{taskID: taskID, lastID: "", seen: make(map[string]string), stopped: false}
		for attempt := 0; ; attempt++ {
			last, done, err := c.streamStepsOnce(
				streamCtx,
				taskID,
				state.lastID,
				authHeader,
				&budget,
				&state,
				state.deliver(yield),
			)
			state.lastID = last
			if state.stopped {
				return
			}
			if streamCtx.Err() != nil {
				yield(zero, streamCtx.Err())
				return
			}
			if err != nil {
				yield(zero, remoteReadError(taskID, err))
				return
			}
			if done {
				return
			}
			if attempt >= c.opts.streamPolicy.MaxReconnects {
				yield(
					zero,
					&RemoteOutcomeError{
						Phase:   PhaseObserve,
						Outcome: OutcomeIncomplete,
						TaskID:  taskID,
						Step:    nil,
						Cause:   nil,
					},
				)
				return
			}
			timer := time.NewTimer(c.opts.streamPolicy.Backoff)
			select {
			case <-timer.C:
			case <-streamCtx.Done():
				timer.Stop()
				yield(zero, streamCtx.Err())
				return
			}
			timer.Stop()
		}
	}
}

func remoteReadError(taskID string, err error) error {
	if outcome, ok := errors.AsType[*RemoteOutcomeError](err); ok {
		outcome.TaskID = taskID
		return outcome
	}
	return &RemoteOutcomeError{Phase: PhaseObserve, Outcome: OutcomeUnknown, TaskID: taskID, Step: nil, Cause: err}
}

type streamBudget struct{ remaining, limit int }
type budgetReader struct {
	ctx    context.Context
	body   io.Reader
	budget *streamBudget
}

func (b *streamBudget) reader(ctx context.Context, body io.Reader) io.Reader {
	return &budgetReader{ctx: ctx, body: body, budget: b}
}

func (r *budgetReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.budget.remaining <= 0 {
		return 0, textprocessor.NewReadLimitError("SSE stream", r.budget.limit, nil)
	}
	if len(p) > r.budget.remaining {
		p = p[:r.budget.remaining]
	}
	n, err := r.body.Read(p)
	r.budget.remaining -= n
	return n, err
}

type streamState struct {
	taskID, lastID string
	seen           map[string]string
	stopped        bool
}

func (s *streamState) deliver(yield func(Step, error) bool) func(Step, error) bool {
	return func(step Step, err error) bool {
		var zero Step
		if err != nil {
			return yield(step, err)
		}
		if step.TaskID != s.taskID {
			s.stopped = true
			yield(
				zero,
				&RemoteOutcomeError{
					Phase:   PhaseObserve,
					Outcome: OutcomeMalformed,
					TaskID:  s.taskID,
					Step:    &step,
					Cause:   nil,
				},
			)
			return false
		}
		if !yield(step, nil) {
			s.stopped = true
			return false
		}
		return true
	}
}

// splitSSELines retains terminators so CRLF cannot evade physical event budgets.
// The supported extension accepts LF and CRLF frames, not bare-CR framing.
func splitSSELines(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i+1], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
