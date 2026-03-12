package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestScratchpad_PinRead(t *testing.T) {
	s := NewScratchpad()
	tools, err := s.AsTools()
	require.NoError(t, err)
	require.Len(t, tools, 3)

	reg := toolsy.NewRegistry()
	for _, tool := range tools {
		reg.Register(tool)
	}

	// Pin a fact
	pinTool, _ := reg.GetTool("memory_pin_fact")
	require.NotNil(t, pinTool)
	require.NoError(t, pinTool.Execute(context.Background(), []byte(`{"key":"allergy","value":"penicillin"}`), func(_ toolsy.Chunk) error { return nil }))

	// Read and verify
	readTool, _ := reg.GetTool("memory_read_all")
	require.NotNil(t, readTool)
	var result string
	require.NoError(t, readTool.Execute(context.Background(), []byte(`{}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(readResult); ok {
				result = r.Facts
			}
		}
		return nil
	}))
	require.Contains(t, result, "allergy=penicillin")
}

func TestScratchpad_PinUnpinRead(t *testing.T) {
	s := NewScratchpad()
	tools, err := s.AsTools()
	require.NoError(t, err)
	reg := toolsy.NewRegistry()
	for _, tool := range tools {
		reg.Register(tool)
	}

	pinTool, _ := reg.GetTool("memory_pin_fact")
	readTool, _ := reg.GetTool("memory_read_all")
	unpinTool, _ := reg.GetTool("memory_unpin_fact")

	_ = pinTool.Execute(context.Background(), []byte(`{"key":"x","value":"y"}`), func(toolsy.Chunk) error { return nil })
	_ = unpinTool.Execute(context.Background(), []byte(`{"key":"x"}`), func(toolsy.Chunk) error { return nil })

	var result string
	_ = readTool.Execute(context.Background(), []byte(`{}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(readResult); ok {
				result = r.Facts
			}
		}
		return nil
	})
	require.Equal(t, "No facts stored.", result)
}

func TestScratchpad_UnpinNotFound(t *testing.T) {
	s := NewScratchpad()
	tools, err := s.AsTools()
	require.NoError(t, err)
	unpinTool := tools[2]

	var status string
	require.NoError(t, unpinTool.Execute(context.Background(), []byte(`{"key":"nonexistent"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(statusResult); ok {
				status = r.Status
			}
		}
		return nil
	}))
	require.Equal(t, "Ignored: key not found", status)
}

func TestScratchpad_ReadEmpty(t *testing.T) {
	s := NewScratchpad()
	tools, err := s.AsTools()
	require.NoError(t, err)
	readTool := tools[1]

	var result string
	require.NoError(t, readTool.Execute(context.Background(), []byte(`{}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(readResult); ok {
				result = r.Facts
			}
		}
		return nil
	}))
	require.Equal(t, "No facts stored.", result)
}

func TestScratchpad_PinOverwrite(t *testing.T) {
	s := NewScratchpad()
	tools, err := s.AsTools()
	require.NoError(t, err)
	pinTool := tools[0]
	readTool := tools[1]
	ctx := context.Background()
	yield := func(toolsy.Chunk) error { return nil }

	require.NoError(t, pinTool.Execute(ctx, []byte(`{"key":"x","value":"old"}`), yield))
	require.NoError(t, pinTool.Execute(ctx, []byte(`{"key":"x","value":"new"}`), yield))
	var result string
	require.NoError(t, readTool.Execute(ctx, []byte(`{}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(readResult); ok {
				result = r.Facts
			}
		}
		return nil
	}))
	require.Contains(t, result, "x=new")
	require.NotContains(t, result, "old")
}

func TestScratchpad_MaxFactsAllowsOverwrite(t *testing.T) {
	s := NewScratchpad(WithMaxFacts(2))
	tools, err := s.AsTools()
	require.NoError(t, err)
	pinTool := tools[0]
	readTool := tools[1]
	ctx := context.Background()
	yield := func(toolsy.Chunk) error { return nil }

	require.NoError(t, pinTool.Execute(ctx, []byte(`{"key":"a","value":"1"}`), yield))
	require.NoError(t, pinTool.Execute(ctx, []byte(`{"key":"b","value":"2"}`), yield))
	// Overwriting existing key "a" must succeed (no new key added)
	require.NoError(t, pinTool.Execute(ctx, []byte(`{"key":"a","value":"updated"}`), yield))
	var result string
	require.NoError(t, readTool.Execute(ctx, []byte(`{}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(readResult); ok {
				result = r.Facts
			}
		}
		return nil
	}))
	require.Contains(t, result, "a=updated")
	require.Contains(t, result, "b=2")
}

func TestScratchpad_MaxFacts(t *testing.T) {
	s := NewScratchpad(WithMaxFacts(2))
	tools, err := s.AsTools()
	require.NoError(t, err)
	pinTool := tools[0]

	ctx := context.Background()
	yield := func(toolsy.Chunk) error { return nil }

	require.NoError(t, pinTool.Execute(ctx, []byte(`{"key":"a","value":"1"}`), yield))
	require.NoError(t, pinTool.Execute(ctx, []byte(`{"key":"b","value":"2"}`), yield))
	err = pinTool.Execute(ctx, []byte(`{"key":"c","value":"3"}`), yield)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err), "expected ClientError")
}

func TestScratchpad_Concurrent(t *testing.T) {
	s := NewScratchpad()
	tools, err := s.AsTools()
	require.NoError(t, err)
	reg := toolsy.NewRegistry()
	for _, tool := range tools {
		reg.Register(tool)
	}

	pinTool, _ := reg.GetTool("memory_pin_fact")
	readTool, _ := reg.GetTool("memory_read_all")
	unpinTool, _ := reg.GetTool("memory_unpin_fact")
	ctx := context.Background()
	yield := func(toolsy.Chunk) error { return nil }

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		n := i
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("k%d", n)
			_ = pinTool.Execute(ctx, []byte(`{"key":"`+key+`","value":"v"}`), yield)
		}()
	}
	wg.Wait()
	for range 10 {
		wg.Go(func() {
			_ = readTool.Execute(ctx, []byte(`{}`), yield)
		})
	}
	wg.Wait()
	for i := range 10 {
		wg.Add(1)
		n := i
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("k%d", n)
			_ = unpinTool.Execute(ctx, []byte(`{"key":"`+key+`"}`), yield)
		}()
	}
	wg.Wait()
}
