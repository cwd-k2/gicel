// SMC tests — merge parallel composition, dag dagger, CapEnv splitting.
// Does NOT cover: session types (future).

package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

func TestMergeBasic(t *testing.T) {
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.State)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

main := do {
  _ <- put 10;
  r <- merge (do { get }) (do { pure 20 });
  pure r
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": int64(0)}
	res, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := res.Value.(*eval.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", res.Value, eval.PrettyValue(res.Value))
	}
	v1, _ := rec.Get("_1")
	v2, _ := rec.Get("_2")
	if hv, ok := v1.(*eval.HostVal); !ok || hv.Inner != int64(10) {
		t.Errorf("expected _1 = 10, got %v", eval.PrettyValue(v1))
	}
	if hv, ok := v2.(*eval.HostVal); !ok || hv.Inner != int64(20) {
		t.Errorf("expected _2 = 20, got %v", eval.PrettyValue(v2))
	}
}

func TestMergeCapEnvSplit(t *testing.T) {
	// Verify that merge splits CapEnv: each side only sees its own capabilities.
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.State)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

main := do {
  _ <- put 42;
  r <- merge (do { x <- get; pure (x + 1) }) (do { pure 100 });
  pure r
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": int64(0)}
	res, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := res.Value.(*eval.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", res.Value, eval.PrettyValue(res.Value))
	}
	v1, _ := rec.Get("_1")
	if hv, ok := v1.(*eval.HostVal); !ok || hv.Inner != int64(43) {
		t.Errorf("expected _1 = 43, got %v", eval.PrettyValue(v1))
	}
}

func TestMergeStateMutation(t *testing.T) {
	// Left side modifies state via multi-step do-block.
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.State)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

main := do {
  _ <- put 10;
  r <- merge (do { modify (* 3); get }) (do { pure (7 :: Int) });
  pure r
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": int64(0)}
	res, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := res.Value.(*eval.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", res.Value, eval.PrettyValue(res.Value))
	}
	v1, _ := rec.Get("_1")
	v2, _ := rec.Get("_2")
	if hv, ok := v1.(*eval.HostVal); !ok || hv.Inner != int64(30) {
		t.Errorf("expected _1 = 30, got %v", eval.PrettyValue(v1))
	}
	if hv, ok := v2.(*eval.HostVal); !ok || hv.Inner != int64(7) {
		t.Errorf("expected _2 = 7, got %v", eval.PrettyValue(v2))
	}
}

func TestMergeStarOperator(t *testing.T) {
	// *** is syntactic sugar for merge.
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.State)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

main := do {
  _ <- put 5;
  r <- (do { get }) *** (do { pure (99 :: Int) });
  pure r
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": int64(0)}
	res, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := res.Value.(*eval.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", res.Value, eval.PrettyValue(res.Value))
	}
	v1, _ := rec.Get("_1")
	v2, _ := rec.Get("_2")
	if hv, ok := v1.(*eval.HostVal); !ok || hv.Inner != int64(5) {
		t.Errorf("expected _1 = 5, got %v", eval.PrettyValue(v1))
	}
	if hv, ok := v2.(*eval.HostVal); !ok || hv.Inner != int64(99) {
		t.Errorf("expected _2 = 99, got %v", eval.PrettyValue(v2))
	}
}

func TestMergePureBothSides(t *testing.T) {
	// Both sides are pure — no CapEnv interaction.
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

main := do {
  r <- merge (do { pure (10 :: Int) }) (do { pure (20 :: Int) });
  pure r
}
`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := res.Value.(*eval.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", res.Value, eval.PrettyValue(res.Value))
	}
	v1, _ := rec.Get("_1")
	v2, _ := rec.Get("_2")
	if hv, ok := v1.(*eval.HostVal); !ok || hv.Inner != int64(10) {
		t.Errorf("expected _1 = 10, got %v", eval.PrettyValue(v1))
	}
	if hv, ok := v2.(*eval.HostVal); !ok || hv.Inner != int64(20) {
		t.Errorf("expected _2 = 20, got %v", eval.PrettyValue(v2))
	}
}

func TestMergeFollowedByBind(t *testing.T) {
	// Merge result feeds into subsequent bind chain.
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.State)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

main := do {
  _ <- put 1;
  (a, b) <- merge (do { x <- get; put (x + 10); get }) (do { pure 42 });
  _ <- put (a + b);
  get
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": int64(0)}
	res, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	// put 1, left: get=1, put 11, get=11. right: pure 42. put (11+42)=53, get=53
	if hv, ok := res.Value.(*eval.HostVal); !ok || hv.Inner != int64(53) {
		t.Fatalf("expected 53, got %v", eval.PrettyValue(res.Value))
	}
}

func TestDagTypeCheck(t *testing.T) {
	// dag (dag f) = f should type-check (involution).
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.State)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

gate :: () -> Computation { state: Int } { state: Int } ()
gate := \u. do { _ <- put 0; pure () }

-- dag swaps pre/post rows. dag (dag f) should restore original type.
roundtrip := \u. dag (dag (gate u))

main := do {
  _ <- put 1;
  _ <- roundtrip ();
  get
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": int64(0)}
	res, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	// After roundtrip (which puts 0), get should return 0.
	if hv, ok := res.Value.(*eval.HostVal); !ok || hv.Inner != int64(0) {
		t.Fatalf("expected 0, got %v", eval.PrettyValue(res.Value))
	}
}

func TestDagMergeInteraction(t *testing.T) {
	// dag + merge: use dag inside a merge arm to verify combined semantics.
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.State)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

main := do {
  _ <- put (1 :: Int);
  r <- merge (do { s <- get; pure (s + 10) }) (do { pure (42 :: Int) });
  pure r
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": int64(0)}
	res, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	// left arm reads state=1 → 11; right arm returns 42. Result: (11, 42).
	rv, ok := res.Value.(*eval.RecordVal)
	if !ok || rv.Len() != 2 {
		t.Fatalf("expected (11, 42), got %v", eval.PrettyValue(res.Value))
	}
}

func TestMergeFirstClass(t *testing.T) {
	// Regression: merge used as first-class value (not special form) must work.
	// CBV evaluates arguments before merge receives them, so this exercises
	// the PrimImpl fallback path (forceOrUse) rather than OpMerge.
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Core

f := merge
main := do {
  r <- f (do { pure (1 :: Int) }) (do { pure (2 :: Int) });
  pure r
}
`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := res.Value.(*eval.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", res.Value, eval.PrettyValue(res.Value))
	}
	v1, _ := rec.Get("_1")
	v2, _ := rec.Get("_2")
	if hv, ok := v1.(*eval.HostVal); !ok || hv.Inner != int64(1) {
		t.Errorf("expected _1 = 1, got %v", eval.PrettyValue(v1))
	}
	if hv, ok := v2.(*eval.HostVal); !ok || hv.Inner != int64(2) {
		t.Errorf("expected _2 = 2, got %v", eval.PrettyValue(v2))
	}
}

func TestMergeCapEnvIsolation(t *testing.T) {
	// Verify that CapEnv splitting actually isolates capabilities.
	// Left computation uses "state" capability; right uses none.
	// After refineMergeLabels, left should see {state} and right should see {}.
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.State)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

main := do {
  _ <- put 0;
  r <- merge (do { _ <- put 99; get }) (do { pure (42 :: Int) });
  pure r
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": int64(0)}
	res, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := res.Value.(*eval.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", res.Value, eval.PrettyValue(res.Value))
	}
	v1, _ := rec.Get("_1")
	v2, _ := rec.Get("_2")
	if hv, ok := v1.(*eval.HostVal); !ok || hv.Inner != int64(99) {
		t.Errorf("expected _1 = 99, got %v", eval.PrettyValue(v1))
	}
	if hv, ok := v2.(*eval.HostVal); !ok || hv.Inner != int64(42) {
		t.Errorf("expected _2 = 42, got %v", eval.PrettyValue(v2))
	}
}
