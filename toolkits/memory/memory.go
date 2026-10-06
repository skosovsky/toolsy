package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/skosovsky/toolsy/internal/jsonschemax"

	"github.com/skosovsky/toolsy"
)

const (
	factsStateKey  = "toolsy.memory.facts"
	statusPinned   = "Success: fact pinned"
	statusUnpinned = "Success: fact unpinned"
	statusIgnored  = "Ignored: key not found"
)

// Scratchpad configures memory tools behavior. Session state is stored in env.StateStore.
type Scratchpad struct {
	mu     sync.Mutex // serializes calls through this instance, not other writers
	limits options
}

// NewScratchpad creates a new scratchpad with optional configuration.
func NewScratchpad(opts ...Option) *Scratchpad {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	o.applyDefaults()
	return &Scratchpad{mu: sync.Mutex{}, limits: o}
}

// AsTools returns the three memory tools (pin, read all, unpin).
func (s *Scratchpad) AsTools() ([]toolsy.Tool, error) {
	if s == nil || s.limits.maxFacts <= 0 || s.limits.maxKeyBytes <= 0 || s.limits.maxValueBytes <= 0 ||
		s.limits.maxStoreBytes <= 0 ||
		s.limits.maxOutputBytes <= 0 {
		return nil, errors.New("toolkit/memory: limits must be positive")
	}
	for _, status := range []string{statusPinned, statusUnpinned, statusIgnored} {
		if err := s.checkOutput(statusResult{Status: status}); err != nil {
			return nil, err
		}
	}
	memRWReq := toolsy.WithRequirements(toolsy.ToolRequirements{ //nolint:exhaustruct_v5 // Permissions host-defined
		MemoryAccess: toolsy.MemoryAccessReadWrite,
		NeedsSession: true,
	})
	memReadReq := toolsy.WithRequirements(toolsy.ToolRequirements{ //nolint:exhaustruct_v5 // Permissions host-defined
		MemoryAccess: toolsy.MemoryAccessRead,
		NeedsSession: true,
	})
	pinTool, err := toolsy.NewTool("memory_pin_fact", "Save a fact to session memory", s.pinHandler, memRWReq)
	if err != nil {
		return nil, fmt.Errorf("toolkit/memory: build pin tool: %w", err)
	}
	readTool, err := toolsy.NewTool(
		"memory_read_all",
		"Read all stored facts",
		s.readHandler,
		toolsy.WithReadOnly(),
		memReadReq,
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/memory: build read tool: %w", err)
	}
	unpinTool, err := toolsy.NewTool("memory_unpin_fact", "Remove a fact from session memory", s.unpinHandler, memRWReq)
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

func (s *Scratchpad) pinHandler(ctx context.Context, run *toolsy.RunEnv, args pinArgs) (statusResult, error) {
	if err := s.checkOutput(statusResult{Status: statusPinned}); err != nil {
		return statusResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, err := s.loadFacts(ctx, run)
	if err != nil {
		return statusResult{}, err
	}
	if s.limits.maxFacts > 0 && len(facts) >= s.limits.maxFacts {
		if _, exists := facts[args.Key]; !exists {
			return statusResult{}, toolsy.NewValidationError("memory limit reached")
		}
	}
	facts[args.Key] = args.Value
	if err := s.saveFacts(ctx, run, facts); err != nil {
		return statusResult{}, err
	}
	return statusResult{Status: statusPinned}, nil
}

type readResult struct {
	Facts string `json:"facts"`
}

func (s *Scratchpad) readHandler(ctx context.Context, run *toolsy.RunEnv, _ struct{}) (readResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, err := s.loadFacts(ctx, run)
	if err != nil {
		return readResult{}, err
	}
	if len(facts) == 0 {
		result := readResult{Facts: "No facts stored."}
		return result, s.checkOutput(result)
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
	result := readResult{Facts: strings.TrimSuffix(b.String(), "\n")}
	return result, s.checkOutput(result)
}

type unpinArgs struct {
	Key string `json:"key"`
}

func (s *Scratchpad) unpinHandler(ctx context.Context, run *toolsy.RunEnv, args unpinArgs) (statusResult, error) {
	for _, status := range []string{statusUnpinned, statusIgnored} {
		if err := s.checkOutput(statusResult{Status: status}); err != nil {
			return statusResult{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, err := s.loadFacts(ctx, run)
	if err != nil {
		return statusResult{}, err
	}
	if _, exists := facts[args.Key]; !exists {
		return statusResult{Status: statusIgnored}, nil
	}
	delete(facts, args.Key)
	if err := s.saveFacts(ctx, run, facts); err != nil {
		return statusResult{}, err
	}
	return statusResult{Status: statusUnpinned}, nil
}

func (s *Scratchpad) loadFacts(ctx context.Context, run *toolsy.RunEnv) (map[string]string, error) {
	if run == nil || run.StateStore == nil {
		return nil, toolsy.NewValidationError("run.StateStore is required")
	}
	raw, err := run.StateStore.Load(ctx, factsStateKey)
	if err != nil {
		return nil, toolsy.NewInternalError(fmt.Errorf("toolkit/memory: load facts: %w", err))
	}
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	if !utf8.Valid(raw) {
		return nil, toolsy.NewInternalError(errors.New("toolkit/memory: stored state is not UTF-8"))
	}
	if len(raw) > s.limits.maxStoreBytes {
		return nil, toolsy.NewInternalError(errors.New("toolkit/memory: stored state exceeds byte limit"))
	}
	decoded, err := jsonschemax.Decode(raw)
	if err != nil {
		return nil, toolsy.NewInternalError(err)
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, toolsy.NewInternalError(errors.New("toolkit/memory: stored state must be a JSON object"))
	}
	for _, value := range object {
		if _, ok := value.(string); !ok {
			return nil, toolsy.NewInternalError(errors.New("toolkit/memory: stored facts must be strings"))
		}
	}
	facts := make(map[string]string)
	if err := json.Unmarshal(raw, &facts); err != nil {
		return nil, toolsy.NewInternalError(fmt.Errorf("toolkit/memory: decode facts: %w", err))
	}
	if err := s.validateFacts(facts); err != nil {
		return nil, toolsy.NewInternalError(fmt.Errorf("toolkit/memory: invalid stored state: %w", err))
	}
	return facts, nil
}

func (s *Scratchpad) saveFacts(ctx context.Context, run *toolsy.RunEnv, facts map[string]string) error {
	if run == nil || run.StateStore == nil {
		return toolsy.NewValidationError("run.StateStore is required")
	}
	if err := s.validateFacts(facts); err != nil {
		return err
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		return toolsy.NewInternalError(fmt.Errorf("toolkit/memory: encode facts: %w", err))
	}
	if len(raw) > s.limits.maxStoreBytes {
		return toolsy.NewValidationError("memory serialized state byte limit exceeded")
	}
	if err := run.StateStore.Save(ctx, factsStateKey, raw); err != nil {
		return toolsy.NewInternalError(fmt.Errorf("toolkit/memory: save facts: %w", err))
	}
	return nil
}

func (s *Scratchpad) validateFacts(facts map[string]string) error {
	if len(facts) > s.limits.maxFacts {
		return toolsy.NewValidationError("memory fact count limit exceeded")
	}
	for key, value := range facts {
		if key == "" || !utf8.ValidString(key) || !utf8.ValidString(value) {
			return toolsy.NewValidationError("memory requires nonempty UTF-8 keys and UTF-8 values")
		}
		if len(key) > s.limits.maxKeyBytes || len(value) > s.limits.maxValueBytes {
			return toolsy.NewValidationError("memory fact byte limit exceeded")
		}
	}
	return nil
}
func (s *Scratchpad) checkOutput(result any) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return toolsy.NewInternalError(err)
	}
	if len(raw) > s.limits.maxOutputBytes {
		return toolsy.NewValidationError("memory response byte limit exceeded")
	}
	return nil
}
