package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// TestNestedMatchInLambda is a regression test for the pool-based
// matchDescs (and recordDescs, mergeDescs) frame-local index calculation.
//
// The bug: addMatchDesc originally returned `frame.matchDescsEnd -
// frame.matchDescsStart` BEFORE append, treating the frame-local count
// as the new entry's index. When a child frame wrote to the pool between
// a parent frame's two addMatchDesc calls (e.g. when an alternative's
// body is a lambda that itself has a case), the parent's second idx
// would point to the child's entry in the parent's Proto.MatchDescs
// view, not the parent's second entry.
//
// The fix: compute idx AFTER the append as `len(pool) - 1 - frame.start`.
// Child entries become gap indices that parent's bytecode never refers
// to; parent's idx is its absolute pool position within its range.
//
// The test distinguishes parent/child by using different constructor
// names (Outer* vs Inner*) — same-name constructors would hide the bug
// because the wrong matchDesc would still match by constructor name.
func TestNestedMatchInLambda(t *testing.T) {
	src := `import Prelude

form Outer := {
  OuterA: Int -> Outer;
  OuterB: Int -> Outer
}

form Inner := {
  InnerA: Int -> Inner;
  InnerB: Int -> Inner
}

f := \o. case o {
  OuterA x => \i. case i {
    InnerA y => x + y;
    InnerB z => x - z
  };
  OuterB x => \i. case i {
    InnerA y => x * y;
    InnerB z => x - (z - 1)
  }
}

main := f (OuterB 100) (InnerA 3)
`
	eng := NewEngine()
	stdlib.Prelude(eng)
	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	// OuterB 100 → InnerA 3 → 100 * 3 = 300.
	// Bug symptom: non-exhaustive pattern match on InnerA 3 (parent's
	// second alt's matchDesc collides with child's matchDesc in pool).
	hv, ok := result.Value.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %+v", result.Value, result.Value)
	}
	if hv.Inner != int64(300) {
		t.Errorf("expected 300, got %v", hv.Inner)
	}
}
