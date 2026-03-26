package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type memStateStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStateStore() *memStateStore {
	return &memStateStore{data: make(map[string][]byte)}
}

func (s *memStateStore) Save(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cpy := append([]byte(nil), data...)
	s.data[key] = cpy
	return nil
}

func (s *memStateStore) Load(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	if !ok {
		return nil, nil
	}
	return append([]byte(nil), v...), nil
}

func mustBuildMemoryRegistry(t *testing.T, s *Scratchpad) *toolsy.Registry {
	t.Helper()
	tools, err := s.AsTools()
	require.NoError(t, err)
	reg, err := toolsy.NewRegistryBuilder().Add(tools...).Build()
	require.NoError(t, err)
	return reg
}

func executeAndDecode[T any](t *testing.T, reg *toolsy.Registry, state toolsy.StateStore, call toolsy.ToolCall) T {
	t.Helper()
	call.Run = toolsy.RunContext{State: state}
	var out T
	err := reg.Execute(context.Background(), call, func(c toolsy.Chunk) error {
		return json.Unmarshal(c.Data, &out)
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return out
}

func TestScratchpad_PinRead(t *testing.T) {
	s := NewScratchpad()
	reg := mustBuildMemoryRegistry(t, s)
	store := newMemStateStore()

	_ = executeAndDecode[statusResult](t, reg, store, toolsy.ToolCall{
		ID:       "1",
		ToolName: "memory_pin_fact",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"allergy","value":"penicillin"}`)},
	})
	read := executeAndDecode[readResult](t, reg, store, toolsy.ToolCall{
		ID:       "2",
		ToolName: "memory_read_all",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
	})
	require.Contains(t, read.Facts, "allergy=penicillin")
}

func TestScratchpad_PinUnpinRead(t *testing.T) {
	s := NewScratchpad()
	reg := mustBuildMemoryRegistry(t, s)
	store := newMemStateStore()

	_ = executeAndDecode[statusResult](t, reg, store, toolsy.ToolCall{
		ID:       "1",
		ToolName: "memory_pin_fact",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"x","value":"y"}`)},
	})
	_ = executeAndDecode[statusResult](t, reg, store, toolsy.ToolCall{
		ID:       "2",
		ToolName: "memory_unpin_fact",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"x"}`)},
	})
	read := executeAndDecode[readResult](t, reg, store, toolsy.ToolCall{
		ID:       "3",
		ToolName: "memory_read_all",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
	})
	require.Equal(t, "No facts stored.", read.Facts)
}

func TestScratchpad_UnpinNotFound(t *testing.T) {
	s := NewScratchpad()
	reg := mustBuildMemoryRegistry(t, s)
	store := newMemStateStore()

	status := executeAndDecode[statusResult](t, reg, store, toolsy.ToolCall{
		ID:       "1",
		ToolName: "memory_unpin_fact",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"nonexistent"}`)},
	})
	require.Equal(t, "Ignored: key not found", status.Status)
}

func TestScratchpad_ReadEmpty(t *testing.T) {
	s := NewScratchpad()
	reg := mustBuildMemoryRegistry(t, s)
	store := newMemStateStore()

	read := executeAndDecode[readResult](t, reg, store, toolsy.ToolCall{
		ID:       "1",
		ToolName: "memory_read_all",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
	})
	require.Equal(t, "No facts stored.", read.Facts)
}

func TestScratchpad_PinOverwrite(t *testing.T) {
	s := NewScratchpad()
	reg := mustBuildMemoryRegistry(t, s)
	store := newMemStateStore()

	_ = executeAndDecode[statusResult](t, reg, store, toolsy.ToolCall{
		ID:       "1",
		ToolName: "memory_pin_fact",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"x","value":"old"}`)},
	})
	_ = executeAndDecode[statusResult](t, reg, store, toolsy.ToolCall{
		ID:       "2",
		ToolName: "memory_pin_fact",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"x","value":"new"}`)},
	})
	read := executeAndDecode[readResult](t, reg, store, toolsy.ToolCall{
		ID:       "3",
		ToolName: "memory_read_all",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
	})
	require.Contains(t, read.Facts, "x=new")
	require.NotContains(t, read.Facts, "old")
}

func TestScratchpad_MaxFactsAllowsOverwrite(t *testing.T) {
	s := NewScratchpad(WithMaxFacts(2))
	reg := mustBuildMemoryRegistry(t, s)
	store := newMemStateStore()

	_ = executeAndDecode[statusResult](
		t,
		reg,
		store,
		toolsy.ToolCall{
			ID:       "1",
			ToolName: "memory_pin_fact",
			Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"a","value":"1"}`)},
		},
	)
	_ = executeAndDecode[statusResult](
		t,
		reg,
		store,
		toolsy.ToolCall{
			ID:       "2",
			ToolName: "memory_pin_fact",
			Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"b","value":"2"}`)},
		},
	)
	_ = executeAndDecode[statusResult](
		t,
		reg,
		store,
		toolsy.ToolCall{
			ID:       "3",
			ToolName: "memory_pin_fact",
			Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"a","value":"updated"}`)},
		},
	)

	read := executeAndDecode[readResult](
		t,
		reg,
		store,
		toolsy.ToolCall{ID: "4", ToolName: "memory_read_all", Input: toolsy.ToolInput{ArgsJSON: []byte(`{}`)}},
	)
	require.Contains(t, read.Facts, "a=updated")
	require.Contains(t, read.Facts, "b=2")
}

func TestScratchpad_MaxFacts(t *testing.T) {
	s := NewScratchpad(WithMaxFacts(2))
	reg := mustBuildMemoryRegistry(t, s)
	store := newMemStateStore()

	_ = executeAndDecode[statusResult](
		t,
		reg,
		store,
		toolsy.ToolCall{
			ID:       "1",
			ToolName: "memory_pin_fact",
			Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"a","value":"1"}`)},
		},
	)
	_ = executeAndDecode[statusResult](
		t,
		reg,
		store,
		toolsy.ToolCall{
			ID:       "2",
			ToolName: "memory_pin_fact",
			Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"b","value":"2"}`)},
		},
	)

	err := reg.Execute(context.Background(), toolsy.ToolCall{
		ID:       "3",
		ToolName: "memory_pin_fact",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"c","value":"3"}`)},
		Run:      toolsy.RunContext{State: store},
	}, func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
}

func TestScratchpad_RequiresStateStore(t *testing.T) {
	s := NewScratchpad()
	reg := mustBuildMemoryRegistry(t, s)

	err := reg.Execute(context.Background(), toolsy.ToolCall{
		ID:       "1",
		ToolName: "memory_read_all",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "run.State is required")
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestScratchpad_Concurrent(t *testing.T) {
	s := NewScratchpad()
	reg := mustBuildMemoryRegistry(t, s)
	store := newMemStateStore()

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		n := i
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("k%d", n)
			_ = executeAndDecode[statusResult](t, reg, store, toolsy.ToolCall{
				ID:       fmt.Sprintf("p-%d", n),
				ToolName: "memory_pin_fact",
				Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"key":"` + key + `","value":"v"}`)},
			})
		}()
	}
	wg.Wait()

	read := executeAndDecode[readResult](t, reg, store, toolsy.ToolCall{
		ID:       "read",
		ToolName: "memory_read_all",
		Input:    toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
	})
	require.Contains(t, read.Facts, "k0=v")
}
