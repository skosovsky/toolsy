package toolsy

import "testing"

type memoryNamedPointer *memoryNamedPointerNode
type memoryOtherPointer *memoryNamedPointerNode
type memoryNamedPointerNode struct {
	Value int
	Next  memoryNamedPointer
}

func TestMemoryNamedPointerIsolation(t *testing.T) {
	// Arrange: named pointer identity must survive root and mixed interface containers.
	source := memoryNamedPointer(&memoryNamedPointerNode{Value: 1})
	source.Next = source
	var nilPointer memoryNamedPointer
	graph := map[string]any{
		"first": source,
		"again": source,
		"list":  []any{source, nilPointer},
		"other": memoryOtherPointer(source),
	}
	// Act.
	root, ok := cloneMutableValue(source).(memoryNamedPointer)
	if !ok {
		t.Fatal("root named pointer type erased")
	}
	out := cloneMutableValue(graph).(map[string]any)
	first, ok := out["first"].(memoryNamedPointer)
	if !ok {
		t.Fatal("nested named pointer type erased")
	}
	first.Value = 2
	// Assert.
	if source.Value != 1 || root == source || root.Next != root || first.Next != first {
		t.Fatal("pointer ownership/cycle lost")
	}
	if out["again"].(memoryNamedPointer) != first || out["list"].([]any)[0].(memoryNamedPointer) != first {
		t.Fatal("named pointer alias lost")
	}
	if out["list"].([]any)[1].(memoryNamedPointer) != nil {
		t.Fatal("named typed nil lost")
	}
	other, ok := out["other"].(memoryOtherPointer)
	if !ok || other.Value != 1 || other.Next != first {
		t.Fatal("type-key identity domain changed")
	}
}

func TestMemoryNamedPointerGenericAndContext(t *testing.T) {
	// Arrange.
	source := memoryNamedPointer(&memoryNamedPointerNode{Value: 1})
	source.Next = source
	// Act.
	cloned := cloneTypedArgValue(source)
	cloned.Value = 2
	if source.Value != 1 {
		t.Fatal("generic snapshot aliases source")
	}
	ctx := preparedContextSnapshot(NewCallContext(source, NoScope{}))
	typed, err := TypedContext[memoryNamedPointer, NoScope](ctx)
	if err != nil {
		t.Fatal(err)
	}
	typed.Subject.Value = 3
	// Assert: generic type assertion must not fall back to the host pointer.
	if source.Value != 1 || cloned == source || typed.Subject == source || cloned.Next != cloned ||
		typed.Subject.Next != typed.Subject {
		t.Fatal("generic/context snapshot aliases source")
	}
	var nilPointer memoryNamedPointer
	if cloneTypedArgValue(nilPointer) != nil {
		t.Fatal("generic named nil changed")
	}
}
