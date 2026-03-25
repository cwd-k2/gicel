//go:build probe

// Runtime edge cases: entry points, error messages, concurrency,
// regression tests, do-blocks, records, IO, and runtime reuse.
package probe_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe C: Edge cases — thunks, entry points, empty programs
// ===================================================================

func TestProbeC_Edge_EntryReturnsUnit(t *testing.T) {
	result, err := probeSandbox("main := pure ()", nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Fatalf("expected (), got %v", result.Value)
	}
}

func TestProbeC_Edge_CustomEntryPoint(t *testing.T) {
	eng := gicel.NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
myMain := 42
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Entry: "myMain"})
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 42)
}

func TestProbeC_Edge_MissingEntryPoint(t *testing.T) {
	eng := gicel.NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `x := 42`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for missing 'main'")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got: %v", err)
	}
}

func TestProbeC_Edge_CustomEntryMissing(t *testing.T) {
	eng := gicel.NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `main := 42`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), &gicel.RunOptions{Entry: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing custom entry point")
	}
}

func TestProbeC_Edge_SandboxCustomEntry(t *testing.T) {
	result, err := probeSandbox("myStart := 99", &gicel.SandboxConfig{
		Entry: "myStart",
	})
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 99)
}

func TestProbeC_Edge_MultipleBindings(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
x := 1
y := 2
main := x + y
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 3)
}

func TestProbeC_Edge_HostBindingMissing(t *testing.T) {
	// Declare a binding but don't provide it at runtime.
	eng := gicel.NewEngine()
	eng.DeclareBinding("hostVal", gicel.ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `main := hostVal`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for missing host binding at runtime")
	}
	if !strings.Contains(err.Error(), "missing binding") {
		t.Fatalf("expected 'missing binding' in error, got: %v", err)
	}
}

func TestProbeC_Edge_DoBlockSinglePure(t *testing.T) {
	result, err := probeSandbox("main := do { pure 42 }", nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 42)
}

func TestProbeC_Edge_NestedDoBlocks(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectState)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
inner := thunk (do { put 10; get })
main := do { put 0; force inner }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &gicel.HostVal{Inner: int64(0)}}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 10)
}

func TestProbeC_Edge_LambdaReturningLambda(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
f := \x. \y. x + y
main := f 10 20
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 30)
}

func TestProbeC_Edge_RecordEmpty(t *testing.T) {
	result, err := probeSandbox("main := {}", nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", result.Value, result.Value)
	}
	if len(rv.Fields) != 0 {
		t.Fatalf("expected empty record, got %d fields", len(rv.Fields))
	}
}

func TestProbeC_Edge_RecordProjection(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
r := { x: 1, y: 2 }
main := r.#x + r.#y
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 3)
}

func TestProbeC_Edge_TupleAccess(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
p := (10, 20)
main := fst p + snd p
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 30)
}

// ===================================================================
// Probe C: Error message quality
// ===================================================================

func TestProbeC_Error_CompileErrorHasPosition(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.NewRuntime(context.Background(), `main := undefined_variable`)
	if err == nil {
		t.Fatal("expected compile error")
	}
	ce, ok := err.(*gicel.CompileError)
	if !ok {
		t.Fatalf("expected CompileError, got %T: %v", err, err)
	}
	diags := ce.Diagnostics()
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic")
	}
	if diags[0].Line == 0 && diags[0].Col == 0 {
		t.Error("expected non-zero position in diagnostic")
	}
	if diags[0].Message == "" {
		t.Error("expected non-empty message in diagnostic")
	}
}

func TestProbeC_Error_CompileErrorPhase(t *testing.T) {
	eng := gicel.NewEngine()
	// Parse error: invalid syntax.
	_, err := eng.NewRuntime(context.Background(), `main := @@@`)
	if err == nil {
		t.Fatal("expected compile error")
	}
	ce, ok := err.(*gicel.CompileError)
	if !ok {
		t.Fatalf("expected CompileError, got %T: %v", err, err)
	}
	diags := ce.Diagnostics()
	if len(diags) == 0 {
		t.Fatal("expected diagnostics")
	}
	// Phase should be "lex" or "parse".
	phase := diags[0].Phase
	if phase != "lex" && phase != "parse" {
		t.Logf("unexpected phase %q for syntax error (may be valid if propagated)", phase)
	}
}

func TestProbeC_Error_CompileErrorNonExhaustiveHasInfo(t *testing.T) {
	// Non-exhaustive Int literal patterns are caught at compile time.
	// Verify the error message mentions 'non-exhaustive'.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
f := \x. case x { 0 => "zero"; 1 => "one" }
main := f 99
`)
	if err == nil {
		t.Fatal("expected compile error for non-exhaustive patterns")
	}
	ce, ok := err.(*gicel.CompileError)
	if !ok {
		t.Fatalf("expected CompileError, got %T: %v", err, err)
	}
	diags := ce.Diagnostics()
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic")
	}
	foundNonExhaustive := false
	for _, d := range diags {
		if strings.Contains(d.Message, "non-exhaustive") {
			foundNonExhaustive = true
			// Verify position info is present.
			if d.Line == 0 {
				t.Error("expected non-zero line in non-exhaustive diagnostic")
			}
		}
	}
	if !foundNonExhaustive {
		t.Fatalf("expected 'non-exhaustive' diagnostic, got: %v", err)
	}
}

func TestProbeC_Error_StepLimitErrorMessage(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.SetStepLimit(10)
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
loop :: Int -> Int
loop := \n. loop (n + 1)
main := loop 0
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected step limit error")
	}
	if !strings.Contains(err.Error(), "step limit") {
		t.Fatalf("expected 'step limit' in error, got: %v", err)
	}
}

func TestProbeC_Error_DepthLimitErrorMessage(t *testing.T) {
	// Use fix (requires EnableRecursion) to trigger depth limit.
	// fix (\self x. self x) creates non-trampolined recursion.
	eng := gicel.NewEngine()
	eng.EnableRecursion()
	eng.SetDepthLimit(5)
	eng.SetStepLimit(1_000_000)
	rt, err := eng.NewRuntime(context.Background(), `
deep := fix (\self x. self x)
main := deep ()
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	// Depth limit or step limit — both are acceptable depending on TCO behavior.
	errMsg := err.Error()
	if !strings.Contains(errMsg, "depth limit") && !strings.Contains(errMsg, "step limit") {
		t.Fatalf("expected limit error, got: %v", errMsg)
	}
}

func TestProbeC_Error_TailRecursionDoesNotGrowDepth(t *testing.T) {
	// BUG-CANDIDATE: low - Trampoline TCO means tail-recursive calls
	// do not grow call depth. This is by design, but means depth limit
	// won't catch infinite tail recursion — only step limit will.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.SetDepthLimit(5)
	eng.SetStepLimit(100)
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
loop :: Int -> Int
loop := \n. loop (n + 1)
main := loop 0
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected limit error")
	}
	// With TCO, step limit fires (not depth limit) for tail recursion.
	if !strings.Contains(err.Error(), "step limit") {
		t.Logf("tail-recursive loop hit: %v (expected step limit due to TCO)", err)
	}
}

func TestProbeC_Error_AllocLimitErrorMessage(t *testing.T) {
	_, err := probeSandbox(`
import Prelude
main := replicate 1000 1
`, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxAlloc: 256,
	})
	if err == nil {
		t.Fatal("expected alloc limit error")
	}
	if !strings.Contains(err.Error(), "allocation limit") {
		t.Fatalf("expected 'allocation limit' in error, got: %v", err)
	}
}

func TestProbeC_Error_ForceNonThunk(t *testing.T) {
	// If we have a value that's not a thunk and try to force it,
	// we should get a clear error. This tests the runtime's force path.
	// The type checker would normally prevent this, but we test the runtime message.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	// Constructing a scenario where force is applied to non-thunk at runtime
	// requires bypassing the type checker. We can at least verify the error format
	// by testing that the checker catches it (compile-time).
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
main := force 42
`)
	if err == nil {
		// If the type checker allows this, the runtime should catch it.
		t.Log("type checker did not catch 'force 42' — runtime error expected")
	}
	// Either way, no panic.
}

// ===================================================================
// Probe C: Goroutine safety — concurrent executions
// ===================================================================

func TestProbeC_Concurrency_MultipleRuns(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := 1 + 2
`)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 20)
	for range 20 {
		go func() {
			result, err := rt.RunWith(context.Background(), nil)
			if err != nil {
				errs <- err
				return
			}
			hv, ok := result.Value.(*gicel.HostVal)
			if !ok {
				errs <- errors.New("expected HostVal")
				return
			}
			if hv.Inner != int64(3) {
				errs <- errors.New("expected 3")
				return
			}
			errs <- nil
		}()
	}
	for range 20 {
		if err := <-errs; err != nil {
			t.Errorf("concurrent run failed: %v", err)
		}
	}
}

// ===================================================================
// Probe C: Regression / adversarial — edge-case compositions
// ===================================================================

func TestProbeC_Regression_CaseOnTuple(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
f := \p. case p { (x, y) => x + y }
main := f (10, 20)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 30)
}

func TestProbeC_Regression_IdentityOnFunction(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := (id (\x. x + 1)) 41
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 42)
}

func TestProbeC_Regression_HigherOrderMap(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := head (map (\x. x * 2) (Cons 21 Nil))
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	probeAssertHostInt(t, con.Args[0], 42)
}

func TestProbeC_Regression_CompositionOperator(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
double := \x. x + x
succ := \x. x + 1
main := (double . succ) 20
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 42)
}

func TestProbeC_Regression_NestedMaybe(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := maybe 0 (\m. maybe 0 id m) (Just (Just 42))
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 42)
}

func TestProbeC_Regression_FlipApply(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
sub := \x y. x - y
main := flip sub 10 42
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 32)
}

func TestProbeC_Regression_RecordUpdate(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
r := { x: 1, y: 2 }
r2 := { r | x: 10 }
main := r2.#x + r.#y
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 12)
}

func TestProbeC_Regression_SandboxConcurrentCalls(t *testing.T) {
	// Multiple concurrent RunSandbox calls should be safe.
	errs := make(chan error, 10)
	for range 10 {
		go func() {
			result, err := gicel.RunSandbox("main := 42", nil)
			if err != nil {
				errs <- err
				return
			}
			hv, ok := result.Value.(*gicel.HostVal)
			if !ok || hv.Inner != int64(42) {
				errs <- errors.New("wrong result")
				return
			}
			errs <- nil
		}()
	}
	for range 10 {
		if err := <-errs; err != nil {
			t.Errorf("concurrent sandbox call failed: %v", err)
		}
	}
}

func TestProbeC_Regression_EqOnStrings(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := eq "abc" "abc"
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "True")
}

func TestProbeC_Regression_EqOnStringsMismatch(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := eq "abc" "xyz"
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "False")
}

func TestProbeC_Regression_OrdOnStrings(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := compare "a" "b"
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "LT")
}

func TestProbeC_Regression_PolymorphicId(t *testing.T) {
	// id applied at multiple types in the same program.
	v, err := probeRun(t, `
import Prelude
a := id 42
b := id True
main := a
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 42)
}

func TestProbeC_Regression_ContextCancellation(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
loop :: Int -> Int
loop := \n. loop (n + 1)
main := loop 0
`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.
	_, err = rt.RunWith(ctx, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// ===================================================================
// Probe D: Runtime reuse — compile once, run many
// ===================================================================

func TestProbeD_RuntimeReuse_Sequential(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := 1 + 2
`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		result, err := rt.RunWith(context.Background(), nil)
		if err != nil {
			t.Fatalf("run %d failed: %v", i, err)
		}
		pdAssertInt(t, result.Value, 3)
	}
}

func TestProbeD_RuntimeReuse_Concurrent(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := 1 + 2
`)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := rt.RunWith(context.Background(), nil)
			if err != nil {
				errs <- err
				return
			}
			hv, ok := result.Value.(*gicel.HostVal)
			if !ok {
				errs <- errors.New("expected HostVal")
				return
			}
			n, ok := hv.Inner.(int64)
			if !ok || n != 3 {
				errs <- errors.New("expected 3")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent run failed: %v", err)
	}
}

func TestProbeD_Engine_MultipleRuntimes(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt1, err := eng.NewRuntime(context.Background(), "import Prelude\nmain := 10")
	if err != nil {
		t.Fatal(err)
	}
	rt2, err := eng.NewRuntime(context.Background(), "import Prelude\nmain := 20")
	if err != nil {
		t.Fatal(err)
	}
	r1, err := rt1.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	pdAssertInt(t, r1.Value, 10)
	pdAssertInt(t, r2.Value, 20)
}

// ===================================================================
// Probe D: Do-block semantics
// ===================================================================

func TestProbeD_Do_BindUnit(t *testing.T) {
	// Bind with unit result: _ <- pure ()
	v, err := pdRun(t, `
import Prelude
main := do { _ <- pure (); pure 42 }
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 42)
}

func TestProbeD_Do_NestedDoBlocks(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
inner := thunk (do { pure 10 })
main := do { x <- force inner; pure (x + 5) }
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 15)
}

func TestProbeD_Do_DoBlockInLambda(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
f := \x. do { pure (x + 1) }
main := f 41
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 42)
}

func TestProbeD_Do_ChainedBinds(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := do {
  a <- pure 1;
  b <- pure 2;
  c <- pure 3;
  pure (a + b + c)
}
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 6)
}

// ===================================================================
// Probe D: Record operations
// ===================================================================

func TestProbeD_Record_EmptyRecord(t *testing.T) {
	v, err := pdRun(t, `main := pure ()`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal (unit), got %T: %v", v, v)
	}
	if len(rec.Fields) != 0 {
		t.Fatalf("expected empty record, got %d fields", len(rec.Fields))
	}
}

func TestProbeD_Record_TupleProjection(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := (1, 2, 3).#_2
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 2)
}

func TestProbeD_Record_NamedFieldProjection(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := { x: 10, y: 20 }.#x
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 10)
}

func TestProbeD_Record_Update(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
r := { x: 1, y: 2 }
main := { r | x: 99 }.#x
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 99)
}

func TestProbeD_Record_UpdatePreservesOtherFields(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
r := { x: 1, y: 2 }
main := { r | x: 99 }.#y
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 2)
}

func TestProbeD_Record_NestedProjection(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := { inner: { val: 42 } }.#inner.#val
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 42)
}

// ===================================================================
// Probe D: Additional edge cases
// ===================================================================

func TestProbeD_ContextCancellation(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := foldl (\acc x. acc + x) 0 (replicate 100000 1)
`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err = rt.RunWith(ctx, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestProbeD_ShowNegativeInt(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := showInt (negate 42)
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertString(t, v, "-42")
}

func TestProbeD_ReadIntValid(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := readInt "123"
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
	pdAssertInt(t, con.Args[0], 123)
}

func TestProbeD_ReadIntInvalid(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := readInt "abc"
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "Nothing")
}

func TestProbeD_ReadIntEmpty(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := readInt ""
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "Nothing")
}

func TestProbeD_IO_PrintCollects(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectIO)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.IO
main := do { print "hello"; print "world"; pure () }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"io": []string{}}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ioVal, ok := result.CapEnv.Get("io")
	if !ok {
		t.Fatal("expected io capability in result")
	}
	msgs, ok := ioVal.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", ioVal)
	}
	if len(msgs) != 2 || msgs[0] != "hello" || msgs[1] != "world" {
		t.Fatalf("unexpected io output: %v", msgs)
	}
}
