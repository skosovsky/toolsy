package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/skosovsky/toolsy"
)

// Scratchpad is a thread-safe in-memory key-value store for session facts.
type Scratchpad struct {
	mu       sync.RWMutex
	facts    map[string]string
	maxFacts int
}

// NewScratchpad creates a new scratchpad with optional configuration.
func NewScratchpad(opts ...Option) *Scratchpad {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return &Scratchpad{
		facts:    make(map[string]string),
		maxFacts: o.maxFacts,
	}
}

// AsTools returns the three memory tools (pin, read all, unpin) bound to this scratchpad.
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

func (s *Scratchpad) pinHandler(_ context.Context, args pinArgs) (statusResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.maxFacts > 0 && len(s.facts) >= s.maxFacts {
		if _, exists := s.facts[args.Key]; !exists {
			return statusResult{}, &toolsy.ClientError{
				Reason: "memory limit reached",
				Err:    toolsy.ErrValidation,
			}
		}
	}
	s.facts[args.Key] = args.Value
	return statusResult{Status: "Success: fact pinned"}, nil
}

type readResult struct {
	Facts string `json:"facts"`
}

func (s *Scratchpad) readHandler(_ context.Context, _ struct{}) (readResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.facts) == 0 {
		return readResult{Facts: "No facts stored."}, nil
	}
	var b strings.Builder
	for k, v := range s.facts {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(v)
		b.WriteString("\n")
	}
	return readResult{Facts: strings.TrimSuffix(b.String(), "\n")}, nil
}

type unpinArgs struct {
	Key string `json:"key"`
}

func (s *Scratchpad) unpinHandler(_ context.Context, args unpinArgs) (statusResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.facts[args.Key]; !exists {
		return statusResult{Status: "Ignored: key not found"}, nil
	}
	delete(s.facts, args.Key)
	return statusResult{Status: "Success: fact unpinned"}, nil
}
