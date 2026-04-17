// ModuleStore tests — sorted-names cache behavior.
// Does NOT cover: circular-dep detection (see engine_module_test.go).
package engine

import (
	"testing"
)

// TestModuleStore_SortedNamesCache verifies that repeated sortedModuleNames
// calls on an unchanged store return the identical slice (same backing
// storage). The engine fingerprint calls this once per fresh Engine.
func TestModuleStore_SortedNamesCache(t *testing.T) {
	s := newModuleStore()
	mod := &compiledModule{} // minimal; fields not read by sortedModuleNames
	s.Register("Foo", mod)
	s.Register("Bar", mod)
	s.Register("Baz", mod)

	a := s.sortedModuleNames()
	b := s.sortedModuleNames()

	if len(a) != len(b) || cap(a) != cap(b) {
		t.Fatalf("cache miss: len/cap differ (a=%d/%d b=%d/%d)", len(a), cap(a), len(b), cap(b))
	}
	if len(a) > 0 && &a[0] != &b[0] {
		t.Error("sortedModuleNames returned a different backing slice on repeat call — cache not hit")
	}

	want := []string{"Bar", "Baz", "Foo"}
	if len(a) != len(want) {
		t.Fatalf("got %d names, want %d", len(a), len(want))
	}
	for i, n := range want {
		if a[i] != n {
			t.Errorf("names[%d] = %q, want %q", i, a[i], n)
		}
	}
}

// TestModuleStore_SortedNamesInvalidation verifies that Register invalidates
// the cache so subsequent sortedModuleNames reflects the new module.
func TestModuleStore_SortedNamesInvalidation(t *testing.T) {
	s := newModuleStore()
	mod := &compiledModule{}
	s.Register("A", mod)
	before := s.sortedModuleNames()
	if len(before) != 1 {
		t.Fatalf("before: got %d names, want 1", len(before))
	}

	s.Register("B", mod)
	after := s.sortedModuleNames()
	if len(after) != 2 {
		t.Fatalf("after Register: got %d names, want 2 — cache not invalidated", len(after))
	}
	if after[0] != "A" || after[1] != "B" {
		t.Errorf("after = %v, want [A B]", after)
	}
}

// TestModuleStore_SortedNamesWarmZeroAlloc pins the zero-alloc invariant
// on repeat calls. Without caching, each call allocates a fresh slice
// and re-sorts. Regressing that (e.g. removing the cache field) fires here.
func TestModuleStore_SortedNamesWarmZeroAlloc(t *testing.T) {
	s := newModuleStore()
	mod := &compiledModule{}
	for _, n := range []string{"A", "B", "C", "D", "E", "F", "G", "H"} {
		s.Register(n, mod)
	}
	// Warm the cache.
	_ = s.sortedModuleNames()

	allocs := testing.AllocsPerRun(100, func() {
		_ = s.sortedModuleNames()
	})
	if allocs != 0 {
		t.Fatalf("warm sortedModuleNames allocated %v times per call; expected 0 "+
			"(the cache is supposed to return a shared slice).", allocs)
	}
}
