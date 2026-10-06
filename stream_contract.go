package toolsy

import (
	"context"
	"errors"
	"fmt"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

// StreamSemantics is an explicit contract, never inferred from MIME.
type StreamSemantics string

const (
	StreamIndependent StreamSemantics = "independent_results"
	StreamTerminal    StreamSemantics = "terminal_result"
)

const defaultStreamOutputLimit = 1 << 20

const streamAbortedKind = "aborted"

// WithIndependentStream selects independent results/progress without a terminal
// promise. Aggregate consumers must declare their own assembly contract.
func WithIndependentStream() ToolOption {
	return func(c *ToolConfig) { c.Manifest.StreamSemantics = StreamIndependent }
}

// WithTerminalStream requires exactly one schema-valid final result and bounds
// raw output bytes. WithOutputSchema is required. Zero selects a bounded default.
func WithTerminalStream(maxBytes int) ToolOption {
	return func(c *ToolConfig) {
		c.Manifest.StreamSemantics = StreamTerminal
		c.Manifest.StreamMaxBytes = maxBytes
	}
}

// StreamContractError identifies a failed stream delivery/validation contract.
// Handler effects may already have happened; this error never authorizes retry.
type StreamContractError struct {
	Kind  string
	Cause error
}

func (e *StreamContractError) Error() string { return "toolsy: stream " + e.Kind }
func (e *StreamContractError) Unwrap() error { return e.Cause }

func buildStreamContract(cfg ToolConfig) (schemaValidator, int, error) {
	switch cfg.Manifest.StreamSemantics {
	case StreamIndependent:
		return nil, 0, nil
	case StreamTerminal:
		if len(cfg.Manifest.OutputSchema) == 0 {
			return nil, 0, errors.New("terminal stream requires output schema")
		}
		if cfg.Manifest.StreamMaxBytes < 0 {
			return nil, 0, errors.New("terminal stream limit cannot be negative")
		}
		limit := cfg.Manifest.StreamMaxBytes
		if limit == 0 {
			limit = defaultStreamOutputLimit
		}
		normalized, err := deepCopySchemaFromMap(cfg.Manifest.OutputSchema)
		if err != nil {
			return nil, 0, err
		}
		validator, err := jsonschemax.Compile(normalized)
		return validator, limit, err
	default:
		return nil, 0, errors.New("stream requires explicit semantics")
	}
}

func runStreamContract(
	ctx context.Context,
	validator schemaValidator,
	limit int,
	produce InvocationHandler,
	yield func(Chunk) error,
) error {
	if validator == nil {
		return produce(yield)
	}
	state := terminalStreamState{
		ctx:        ctx,
		validator:  validator,
		limit:      limit,
		yield:      yield,
		candidate:  nil,
		sticky:     nil,
		totalBytes: 0,
	}
	err := produce(state.accept)
	if state.sticky != nil {
		return state.sticky
	}
	if err != nil {
		if IsControlError(err) {
			return err
		}
		return &StreamContractError{Kind: streamAbortedKind, Cause: err}
	}
	if interrupt := ctx.Err(); interrupt != nil {
		return &StreamContractError{Kind: streamAbortedKind, Cause: interrupt}
	}
	if state.candidate == nil {
		return &StreamContractError{Kind: "missing_terminal", Cause: nil}
	}
	if err := yield(*state.candidate); err != nil {
		return &StreamContractError{Kind: streamAbortedKind, Cause: fmt.Errorf("terminal delivery: %w", err)}
	}
	return nil
}

type terminalStreamState struct {
	ctx        context.Context
	validator  schemaValidator
	limit      int
	yield      func(Chunk) error
	candidate  *Chunk
	sticky     error
	totalBytes int
}

func (s *terminalStreamState) accept(c Chunk) error {
	if s.sticky != nil {
		return s.sticky
	}
	if interrupt := s.ctx.Err(); interrupt != nil {
		return s.fail(streamAbortedKind, interrupt)
	}
	if len(c.Data) > s.limit-s.totalBytes {
		return s.fail("output_limit", nil)
	}
	s.totalBytes += len(c.Data)
	if c.Event == EventResult {
		return s.capture(c)
	}
	if err := s.yield(c); err != nil {
		return s.fail(streamAbortedKind, err)
	}
	if c.Event == EventControl {
		s.sticky = ControlErrorFromSignal(c.Control)
	}
	return s.sticky
}

func (s *terminalStreamState) capture(c Chunk) error {
	if c.IsError {
		return s.fail(streamAbortedKind, executionErrorFromChunk(c))
	}
	if s.candidate != nil {
		return s.fail("duplicate_terminal", nil)
	}
	value, err := jsonschemax.Decode(c.Data)
	if err != nil {
		return s.fail("schema_mismatch", err)
	}
	if err := s.validator.Validate(value); err != nil {
		return s.fail("schema_mismatch", err)
	}
	copied := cloneResultChunk(c)
	s.candidate = &copied
	return nil
}

func (s *terminalStreamState) fail(kind string, cause error) error {
	s.sticky = &StreamContractError{Kind: kind, Cause: cause}
	return s.sticky
}
