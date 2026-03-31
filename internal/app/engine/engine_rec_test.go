// Rec (computation-level fixpoint) tests — stateful loops, base cases, named capabilities, nesting.
// Does NOT cover: fix (engine_gadt_fix_test.go), recursion scoping (engine_recursion_scope_test.go).

package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func runRec(t *testing.T, src string) *RunResult {
	t.Helper()
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.State)
	eng.Use(stdlib.Fail)
	eng.EnableRecursion()
	eng.SetStepLimit(100_000)
	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	return result
}

func runRecExpectError(t *testing.T, src string) error {
	t.Helper()
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.State)
	eng.EnableRecursion()
	eng.SetStepLimit(10_000)
	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		return err
	}
	_, err = rt.RunWith(context.Background(), nil)
	return err
}

func mustInt(t *testing.T, v eval.Value) int64 {
	t.Helper()
	hv, ok := v.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %v", v, v)
	}
	n, ok := hv.Inner.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T: %v", hv.Inner, hv.Inner)
	}
	return n
}

func mustCon(t *testing.T, v eval.Value, name string) *eval.ConVal {
	t.Helper()
	cv, ok := v.(*eval.ConVal)
	if !ok {
		t.Fatalf("expected ConVal(%s), got %T: %v", name, v, v)
	}
	if cv.Con != name {
		t.Fatalf("expected Con %q, got %q", name, cv.Con)
	}
	return cv
}

// ---------------------------------------------------------------------------
// 1. Basic rec: stateful countdown
// ---------------------------------------------------------------------------

func TestRec_StatefulCountdown(t *testing.T) {
	r := runRec(t, `
import Prelude
import Effect.State

main := do {
  put 0;
  rec (\self. do {
    x <- get;
    if x >= 10
       then pure x
       else do { put (x + 1); self }
  })
}
`)
	if n := mustInt(t, r.Value); n != 10 {
		t.Errorf("expected 10, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// 2. rec pure: immediate base case (no recursion)
// ---------------------------------------------------------------------------

func TestRec_PureImmediate(t *testing.T) {
	r := runRec(t, `
import Prelude
main := rec (\self. pure 42)
`)
	if n := mustInt(t, r.Value); n != 42 {
		t.Errorf("expected 42, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// 3. rec with state doubling (power of 2)
// ---------------------------------------------------------------------------

func TestRec_StateDoubling(t *testing.T) {
	r := runRec(t, `
import Prelude
import Effect.State

main := do {
  put 1;
  rec (\self. do {
    x <- get;
    if x >= 100
       then pure x
       else do { put (x * 2); self }
  })
}
`)
	if n := mustInt(t, r.Value); n != 128 {
		t.Errorf("expected 128, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// 4. rec returning a constructed value (ADT)
// ---------------------------------------------------------------------------

func TestRec_ReturnADT(t *testing.T) {
	r := runRec(t, `
import Prelude
import Effect.State

main := do {
  put 0;
  rec (\self. do {
    x <- get;
    if x >= 5
       then pure (Just x)
       else do { put (x + 1); self }
  })
}
`)
	cv := mustCon(t, r.Value, "Just")
	if n := mustInt(t, cv.Args[0]); n != 5 {
		t.Errorf("expected Just 5, got Just %d", n)
	}
}

// ---------------------------------------------------------------------------
// 5. rec with state accumulation (sum 1..10)
// ---------------------------------------------------------------------------

func TestRec_Accumulation(t *testing.T) {
	r := runRec(t, `
import Prelude
import Effect.State

main := do {
  put (0, 1);
  rec (\self. do {
    st <- get;
    sum := fst st;
    i := snd st;
    if i > 10
      then pure sum
      else do { put (sum + i, i + 1); self }
  })
}
`)
	if n := mustInt(t, r.Value); n != 55 {
		t.Errorf("expected 55, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// 6. rec with named capabilities
// ---------------------------------------------------------------------------

func TestRec_NamedCapabilities(t *testing.T) {
	r := runRec(t, `
import Prelude
import Effect.State

main := do {
  putAt @#counter 0;
  rec (\self. do {
    c <- getAt @#counter;
    if c >= 5
       then pure c
       else do { putAt @#counter (c + 1); self }
  })
}
`)
	if n := mustInt(t, r.Value); n != 5 {
		t.Errorf("expected 5, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// 7. rec step limit exhaustion (infinite loop protection)
// ---------------------------------------------------------------------------

func TestRec_StepLimitExhaustion(t *testing.T) {
	err := runRecExpectError(t, `
import Prelude
import Effect.State

main := do {
  put 0;
  rec (\self. do { modify (+ 1); self })
}
`)
	if err == nil {
		t.Fatal("expected step limit error for unconditional rec loop")
	}
	if !strings.Contains(err.Error(), "step limit") {
		t.Errorf("expected 'step limit' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 8. rec without --recursion flag is rejected
// ---------------------------------------------------------------------------

func TestRec_RequiresRecursionFlag(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.State)
	// NOT calling eng.EnableRecursion()
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := do { put 0; rec (\self. do { x <- get; pure x }) }
`)
	if err == nil {
		t.Fatal("expected compile error: rec without --recursion")
	}
	if !strings.Contains(err.Error(), "rec") {
		t.Errorf("expected error mentioning 'rec', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 9. rec preserves capability environment across iterations
// ---------------------------------------------------------------------------

func TestRec_PreservesCapEnv(t *testing.T) {
	r := runRec(t, `
import Prelude
import Effect.State

main := do {
  put "start";
  rec (\self. do {
    s <- get;
    if strlen s > 20
      then pure s
      else do { put (s <> "+"); self }
  })
}
`)
	hv, ok := r.Value.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", r.Value)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T", hv.Inner)
	}
	if len(s) <= 20 {
		t.Errorf("expected string longer than 20, got %q (len %d)", s, len(s))
	}
}

// ---------------------------------------------------------------------------
// 10. rec returning tuple
// ---------------------------------------------------------------------------

func TestRec_ReturnTuple(t *testing.T) {
	r := runRec(t, `
import Prelude
import Effect.State

main := do {
  put 0;
  rec (\self. do {
    x <- get;
    if x >= 3
       then pure (x, x * x)
       else do { put (x + 1); self }
  })
}
`)
	rec, ok := r.Value.(*eval.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal (tuple), got %T", r.Value)
	}
	v1, _ := rec.Get("_1")
	v2, _ := rec.Get("_2")
	if mustInt(t, v1) != 3 || mustInt(t, v2) != 9 {
		t.Errorf("expected (3, 9), got (%v, %v)", v1, v2)
	}
}

// ---------------------------------------------------------------------------
// 11. rec with fail effect (early exit via fail)
// ---------------------------------------------------------------------------

func TestRec_WithFailEffect(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.State)
	eng.Use(stdlib.Fail)
	eng.EnableRecursion()
	eng.SetStepLimit(100_000)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
import Effect.Fail

main := do {
  put 0;
  rec (\self. do {
    x <- get;
    if x >= 100 then failWith "too high"
    else if x >= 10 then pure x
    else do { put (x + 1); self }
  })
}
`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if n := mustInt(t, result.Value); n != 10 {
		t.Errorf("expected 10, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// 12. rec does not break existing thunk/force semantics
// ---------------------------------------------------------------------------

func TestRec_ThunkForceUnaffected(t *testing.T) {
	r := runRec(t, `
import Prelude
import Effect.State

mkThunk := \n. thunk (do { put n; pure n })

main := do {
  put 0;
  t := mkThunk 42;
  force t
}
`)
	if n := mustInt(t, r.Value); n != 42 {
		t.Errorf("expected 42, got %d", n)
	}
	// Check state was set by thunk
	v, ok := r.CapEnv.Get("state")
	if !ok {
		t.Fatal("expected 'state' in capEnv")
	}
	hv, ok := v.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", v)
	}
	if hv.Inner.(int64) != 42 {
		t.Errorf("expected state=42, got %v", hv.Inner)
	}
}

// ---------------------------------------------------------------------------
// 13. rec with modify (simpler than get/put pattern)
// ---------------------------------------------------------------------------

func TestRec_WithModify(t *testing.T) {
	r := runRec(t, `
import Prelude
import Effect.State

main := do {
  put 1;
  rec (\self. do {
    x <- get;
    if x >= 1000
       then pure x
       else do { modify (+ x); self }
  })
}
`)
	// 1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024
	if n := mustInt(t, r.Value); n != 1024 {
		t.Errorf("expected 1024, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// 14. rec producing a list via state accumulation
// ---------------------------------------------------------------------------

func TestRec_BuildList(t *testing.T) {
	r := runRec(t, `
import Prelude
import Effect.State

main := do {
  put (Nil :: List Int, 5);
  rec (\self. do {
    st <- get;
    acc := fst st;
    n := snd st;
    if n <= 0
      then pure acc
      else do { put (Cons n acc, n - 1); self }
  })
}
`)
	// Should produce [1, 2, 3, 4, 5]
	var elems []int64
	v := r.Value
	for {
		cv, ok := v.(*eval.ConVal)
		if !ok || cv.Con == "Nil" {
			break
		}
		elems = append(elems, mustInt(t, cv.Args[0]))
		v = cv.Args[1]
	}
	expected := []int64{1, 2, 3, 4, 5}
	if len(elems) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, elems)
	}
	for i, e := range expected {
		if elems[i] != e {
			t.Errorf("index %d: expected %d, got %d", i, e, elems[i])
		}
	}
}

// ---------------------------------------------------------------------------
// 15. rec type-checks correctly (check-only, no eval)
// ---------------------------------------------------------------------------

func TestRec_TypeCheck(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.State)
	eng.EnableRecursion()
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

main := do {
  put 0;
  rec (\self. do {
    x <- get;
    if x >= 10
       then pure x
       else do { put (x + 1); self }
  })
}
`)
	if err != nil {
		t.Fatalf("type check failed: %v", err)
	}
}
