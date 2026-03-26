package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/skosovsky/toolsy"
)

const factsStateKey = "toolsy.memory.facts"

// Scratchpad configures memory tools behavior. Session state is stored in run.State.
type Scratchpad struct {
	maxFacts int
}

// NewScratchpad creates a new scratchpad with optional configuration.
func NewScratchpad(opts ...Option) *Scratchpad {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return &Scratchpad{maxFacts: o.maxFacts}
}

// AsTools returns the three memory tools (pin, read all, unpin).
func (s *Scratchpad) AsTools() ([]toolsy.Tool, error) {
	pinTool, err := toolsy.NewTool("memory_pin_fact", "Save a fact to session memory", s.pinHandler)
	if err != nil {
		return nil, fmt.Errorf("toolkit/memory: build pin tool: %w", err)
	}
	readTool, err := toolsy.NewTool("memory_read_all", "Read all stored facts", s.readHandler)
	if err != nil {
		return nil, fmt.Errorf("toolkit/memory: build read tool: %w", err)
	}
	unpinTool, err := toolsy.NewTool("memory_unpin_fact", "Remove a fact from session memory", s.unpinHandler)
	if err != nil {
		return nil, fmt.Errorf("toolkit/memory: build unpin tool: %w", err)
	}
	return []toolsy.Tool{pinTool, readTool, unpinTool}, nil
}

type pinArgs struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type statusResult struct {
	Status string `json:"status"`
}

func (s *Scratchpad) pinHandler(ctx context.Context, run toolsy.RunContext, args pinArgs) (statusResult, error) {
	facts, err := loadFacts(ctx, run)
	if err != nil {
		return statusResult{}, err
	}
	if s.maxFacts > 0 && len(facts) >= s.maxFacts {
		if _, exists := facts[args.Key]; !exists {
			return statusResult{}, &toolsy.ClientError{
				Reason:    "memory limit reached",
				Retryable: false,
				Err:       toolsy.ErrValidation,
			}
		}
	}
	facts[args.Key] = args.Value
	if err := saveFacts(ctx, run, facts); err != nil {
		return statusResult{}, err
	}
	return statusResult{Status: "Success: fact pinned"}, nil
}

type readResult struct {
	Facts string `json:"facts"`
}

func (s *Scratchpad) readHandler(ctx context.Context, run toolsy.RunContext, _ struct{}) (readResult, error) {
	facts, err := loadFacts(ctx, run)
	if err != nil {
		return readResult{}, err
	}
	if len(facts) == 0 {
		return readResult{Facts: "No facts stored."}, nil
	}
	keys := make([]string, 0, len(facts))
	for k := range facts {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(facts[k])
		b.WriteString("\n")
	}
	return readResult{Facts: strings.TrimSuffix(b.String(), "\n")}, nil
}

type unpinArgs struct {
	Key string `json:"key"`
}

func (s *Scratchpad) unpinHandler(ctx context.Context, run toolsy.RunContext, args unpinArgs) (statusResult, error) {
	facts, err := loadFacts(ctx, run)
	if err != nil {
		return statusResult{}, err
	}
	if _, exists := facts[args.Key]; !exists {
		return statusResult{Status: "Ignored: key not found"}, nil
	}
	delete(facts, args.Key)
	if err := saveFacts(ctx, run, facts); err != nil {
		return statusResult{}, err
	}
	return statusResult{Status: "Success: fact unpinned"}, nil
}

func loadFacts(ctx context.Context, run toolsy.RunContext) (map[string]string, error) {
	if run.State == nil {
		return nil, fmt.Errorf("toolkit/memory: %w", errMissingStateStore)
	}
	raw, err := run.State.Load(ctx, factsStateKey)
	if err != nil {
		return nil, fmt.Errorf("toolkit/memory: load facts: %w", err)
	}
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	facts := make(map[string]string)
	if err := json.Unmarshal(raw, &facts); err != nil {
		return nil, fmt.Errorf("toolkit/memory: decode facts: %w", err)
	}
	return facts, nil
}

func saveFacts(ctx context.Context, run toolsy.RunContext, facts map[string]string) error {
	if run.State == nil {
		return fmt.Errorf("toolkit/memory: %w", errMissingStateStore)
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		return fmt.Errorf("toolkit/memory: encode facts: %w", err)
	}
	if err := run.State.Save(ctx, factsStateKey, raw); err != nil {
		return fmt.Errorf("toolkit/memory: save facts: %w", err)
	}
	return nil
}

var errMissingStateStore = errors.New("run.State is required")
