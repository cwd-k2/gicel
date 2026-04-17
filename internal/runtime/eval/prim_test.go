// Prim tests — sorted-names cache behavior.
// Does NOT cover: prim invocation semantics (see eval_*_test.go).
package eval

import (
	"context"
	"testing"
)

// TestPrimRegistry_SortedNamesCache verifies that repeated SortedNames
// calls on an unchanged registry return the identical slice (same
// backing storage). The engine fingerprint path calls SortedNames once
// per fresh Engine; a missing cache shows up as per-call slice alloc.
func TestPrimRegistry_SortedNamesCache(t *testing.T) {
	r := NewPrimRegistry()
	noop := func(_ context.Context, ce CapEnv, _ []Value, _ Applier) (Value, CapEnv, error) {
		return nil, ce, nil
	}
	r.Register("foo", noop)
	r.Register("bar", noop)
	r.Register("baz", noop)

	a := r.SortedNames()
	b := r.SortedNames()

	// Same backing array: len and cap match, and the header points to
	// the same data (first element address).
	if len(a) != len(b) || cap(a) != cap(b) {
		t.Fatalf("cache miss: len/cap differ (a=%d/%d b=%d/%d)", len(a), cap(a), len(b), cap(b))
	}
	if len(a) > 0 && &a[0] != &b[0] {
		t.Error("SortedNames returned a different backing slice on repeat call — cache not hit")
	}

	// Content invariant.
	want := []string{"bar", "baz", "foo"}
	if len(a) != len(want) {
		t.Fatalf("got %d names, want %d", len(a), len(want))
	}
	for i, n := range want {
		if a[i] != n {
			t.Errorf("names[%d] = %q, want %q", i, a[i], n)
		}
	}
}

// TestPrimRegistry_SortedNamesInvalidation verifies that Register
// invalidates the cache so subsequent SortedNames reflects the new name.
func TestPrimRegistry_SortedNamesInvalidation(t *testing.T) {
	r := NewPrimRegistry()
	noop := func(_ context.Context, ce CapEnv, _ []Value, _ Applier) (Value, CapEnv, error) {
		return nil, ce, nil
	}
	r.Register("a", noop)
	before := r.SortedNames()
	if len(before) != 1 {
		t.Fatalf("before: got %d names, want 1", len(before))
	}

	r.Register("b", noop)
	after := r.SortedNames()
	if len(after) != 2 {
		t.Fatalf("after Register: got %d names, want 2 — cache not invalidated", len(after))
	}
	if after[0] != "a" || after[1] != "b" {
		t.Errorf("after = %v, want [a b]", after)
	}
}

// TestPrimRegistry_SortedNamesWarmZeroAlloc pins the zero-alloc invariant
// on repeat calls. This is the engine fingerprint hot-path property.
// A future refactor that drops the cache will fail here.
func TestPrimRegistry_SortedNamesWarmZeroAlloc(t *testing.T) {
	r := NewPrimRegistry()
	noop := func(_ context.Context, ce CapEnv, _ []Value, _ Applier) (Value, CapEnv, error) {
		return nil, ce, nil
	}
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		r.Register(n, noop)
	}
	// Warm the cache.
	_ = r.SortedNames()

	allocs := testing.AllocsPerRun(100, func() {
		_ = r.SortedNames()
	})
	if allocs != 0 {
		t.Fatalf("warm SortedNames allocated %v times per call; expected 0 "+
			"(the cache is supposed to return a shared slice).", allocs)
	}
}
