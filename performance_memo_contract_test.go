package toolsy

import (
	"reflect"
	"runtime"
	"testing"
	"unsafe"
)

func TestMemoryMemoPanicAndNestedLease(t *testing.T) {
	// Arrange: reflection's existing private-value panic must unwind the lease.
	drainIdleMemos()
	graph := map[string]any{"leaf": 1}
	for range 20 {
		graph = map[string]any{"next": graph}
	}
	source := struct{ private map[string]any }{graph}
	panicked := false
	// Act.
	func() {
		defer func() { panicked = recover() != nil }()
		_ = cloneReflectValue(reflect.ValueOf(source).Field(0))
	}()
	// Assert.
	if !panicked {
		t.Fatal("private reflection panic changed")
	}
	if len(idleCloneMemos) == 0 {
		t.Fatal("panic did not exercise acquired scratch")
	}
	assertIdleMemosClean(t)
	// Arrange: an outer traversal owns scratch while an independent inner operation runs.
	outer := valueCloner{}
	defer outer.release()
	root := map[string]any{"x": 1}
	for i := range 10 {
		root[string(rune(i))] = map[string]any{"x": 1}
	}
	_ = outer.clone(reflect.ValueOf(root))
	lease := outer.memo
	// Act.
	out := cloneMutableValue(root).(map[string]any)
	out["x"] = 2
	// Assert.
	if outer.memo != lease || root["x"] != 1 {
		t.Fatal("nested operation broke ownership")
	}
	for _, entry := range lease.entries {
		if entry.value.IsValid() && entry.value.Kind() == reflect.Map &&
			entry.value.Interface().(map[string]any)["x"] != 1 {
			t.Fatal("inner operation reused outer memo")
		}
	}
}

func TestMemoryMemoBoundWithoutGC(t *testing.T) {
	// Arrange: hold more leases than idle capacity and create oversize graphs.
	drainIdleMemos()
	owners := make([]valueCloner, cloneMemoIdle*3)
	root := map[string]any{}
	for i := range 2000 {
		root[string(rune(i))] = map[string]any{"x": i}
	}
	// Act.
	for i := range owners {
		_ = owners[i].clone(reflect.ValueOf(root))
	}
	for i := range owners {
		owners[i].release()
	}
	// Assert: no GC needed, no graph-dependent capacity or reference remains.
	if len(idleCloneMemos) != cloneMemoIdle {
		t.Fatal("idle capacity incorrect")
	}
	assertIdleMemosClean(t)
	t.Logf(
		"memo struct %d bytes, max idle payload %d bytes; channel header/runtime allocator excluded",
		unsafe.Sizeof(cloneMemo{}),
		cloneMemoIdle*unsafe.Sizeof(cloneMemo{}),
	)
}

func drainIdleMemos() {
	for {
		select {
		case <-idleCloneMemos:
		default:
			return
		}
	}
}

//nolint:gocognit // Audit every retained field and restore the idle store for later tests.
func assertIdleMemosClean(t *testing.T) {
	t.Helper()
	var held []*cloneMemo
	for {
		select {
		case memo := <-idleCloneMemos:
			held = append(held, memo)
			if memo.count != 0 || memo.overflow != nil {
				t.Fatal("scratch retained overflow")
			}
			for _, entry := range memo.entries {
				if entry.key != (cloneReference{}) || entry.value.IsValid() {
					t.Fatal("scratch retained graph")
				}
			}
			for _, slot := range memo.used {
				if slot != 0 {
					t.Fatal("scratch retained occupied index")
				}
			}
		default:
			for _, memo := range held {
				idleCloneMemos <- memo
			}
			return
		}
	}
}

func TestMemoryIdleAllocationBound(t *testing.T) {
	// Arrange: measure fixed scratch allocator footprint independently of graph/output.
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	// Act.
	idle := make(chan *cloneMemo, cloneMemoIdle)
	for range cloneMemoIdle {
		idle <- &cloneMemo{}
	}
	runtime.ReadMemStats(&after)
	// Assert: conservative channel allowance covers header and its four pointer slots.
	bound := uint64(cloneMemoIdle*32768 + 1024)
	allocated := after.TotalAlloc - before.TotalAlloc
	if allocated > bound {
		t.Fatalf("idle scratch allocation %d exceeds %d", allocated, bound)
	}
	t.Logf("idle scratch+channel allocation %d bytes; conservative bound %d bytes", allocated, bound)
	runtime.KeepAlive(idle)
}
