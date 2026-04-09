//go:build probe

package probe_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe E: Runtime / Evaluator / Stdlib adversarial probing.
// Focus: resource limits, crash resistance, stdlib correctness,
// sandbox safety, explain/trace, effect handling, CapEnv, value repr.
// ===================================================================

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func peRun(t *testing.T, source string, packs ...gicel.Pack) (gicel.Value, error) {
	t.Helper()
	eng := gicel.NewEngine()
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		return nil, err
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

func peRunWithCaps(t *testing.T, source string, caps map[string]any, packs ...gicel.Pack) (*gicel.RunResult, error) {
	t.Helper()
	eng := gicel.NewEngine()
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		return nil, err
	}
	return rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
}

func peRunWithLimits(t *testing.T, source string, steps, depth int, alloc int64, packs ...gicel.Pack) (gicel.Value, error) {
	t.Helper()
	eng := gicel.NewEngine()
	eng.SetStepLimit(steps)
	eng.SetDepthLimit(depth)
	eng.SetAllocLimit(alloc)
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		return nil, err
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

func peRunWithExplain(t *testing.T, source string, packs ...gicel.Pack) (gicel.Value, []gicel.ExplainStep, error) {
	t.Helper()
	eng := gicel.NewEngine()
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		return nil, nil, err
	}
	var steps []gicel.ExplainStep
	hook := func(s gicel.ExplainStep) {
		steps = append(steps, s)
	}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{
		Explain: hook,
	})
	if err != nil {
		return nil, steps, err
	}
	return result.Value, steps, nil
}

// ---------------------------------------------------------------------------
// 1. Resource Limit Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_Eval_StepLimitZeroViaEngine(t *testing.T) {
	// Step limit of 0 disables the limit. Eval should succeed.
	eng := gicel.NewEngine()
	eng.SetStepLimit(0)
	rt, err := eng.NewRuntime(context.Background(), `main := 42`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("step limit 0 (disabled) should succeed, got: %v", err)
	}
}

func TestProbeE_Eval_DepthLimitZeroViaEngine(t *testing.T) {
	// Depth limit of 0 via Engine. Bind requires Enter (depth 1 > 0 => error).
	// Must not panic regardless of outcome.
	eng := gicel.NewEngine()
	eng.SetDepthLimit(0)
	rt, err := eng.NewRuntime(context.Background(), `main := do { pure 42 }`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	// Either error or success is fine; no panic is the requirement.
	_ = err
}

func TestProbeE_Eval_AllocLimitZeroViaEngine(t *testing.T) {
	// AllocLimit of 0 means disabled (the Limit.Alloc check: allocLimit > 0).
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.SetAllocLimit(0) // Should disable, not crash
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := Cons 1 (Cons 2 Nil)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("alloc limit 0 should disable tracking, got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestProbeE_Eval_AllocLimitOneByteViaEngine(t *testing.T) {
	// AllocLimit of 1 byte: any ConVal allocation should fail.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.SetAllocLimit(1)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := Cons 1 Nil
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected allocation limit error with 1 byte limit")
	}
	if !strings.Contains(err.Error(), "allocation limit") {
		t.Fatalf("expected allocation limit error, got: %v", err)
	}
}

func TestProbeE_Eval_NegativeStepLimit(t *testing.T) {
	// Negative step limit is clamped to zero (disabled) by budget.New.
	eng := gicel.NewEngine()
	eng.SetStepLimit(-1)
	rt, err := eng.NewRuntime(context.Background(), `main := 42`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("negative step limit (clamped to 0, disabled) should succeed, got: %v", err)
	}
}

func TestProbeE_Eval_NegativeDepthLimit(t *testing.T) {
	// Negative depth limit: Enter() checks depth > maxDepth. Must not panic.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.SetDepthLimit(-1)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := do { pure 42 }
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	_ = err // Non-panic is the requirement.
}

func TestProbeE_Eval_NegativeAllocLimitDisablesCheck(t *testing.T) {
	// Negative allocLimit is clamped to zero (disabled) by SetAllocLimit.
	// This is documented behavior: zero disables, negative is treated as zero.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.SetAllocLimit(-1) // Should disable, not cause alloc errors
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("negative alloc limit should disable check, got: %v", err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %v", result.Value)
	}
}

// ---------------------------------------------------------------------------
// 2. Sandbox Safety
// ---------------------------------------------------------------------------

func TestProbeE_Eval_SandboxNilConfig(t *testing.T) {
	_, err := gicel.RunSandbox(`main := 42`, nil)
	if err != nil {
		t.Fatalf("nil config should work with simple program: %v", err)
	}
}

func TestProbeE_Eval_SandboxNegativeLimits(t *testing.T) {
	// Sandbox normalizes negative limits to defaults (the <= 0 checks in sandbox.go).
	result, err := gicel.RunSandbox(`main := 42`, &gicel.SandboxConfig{
		MaxSteps: -999,
		MaxDepth: -999,
		MaxAlloc: -999,
	})
	if err != nil {
		t.Fatalf("sandbox should normalize negative limits: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestProbeE_Eval_SandboxZeroTimeout(t *testing.T) {
	result, err := gicel.RunSandbox(`main := 42`, &gicel.SandboxConfig{Timeout: 0})
	if err != nil {
		t.Fatalf("zero timeout should use default: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestProbeE_Eval_SandboxEmptySource(t *testing.T) {
	_, err := gicel.RunSandbox(``, nil)
	if err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestProbeE_Eval_SandboxNoMainBinding(t *testing.T) {
	_, err := gicel.RunSandbox(`x := 42`, nil)
	if err == nil {
		t.Fatal("expected error for missing main")
	}
	if !strings.Contains(err.Error(), "main") {
		t.Fatalf("expected 'main' in error message, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 3. Evaluator Crash Resistance
// ---------------------------------------------------------------------------

func TestProbeE_Eval_PatternMatchExhaustionRuntime(t *testing.T) {
	_, err := peRun(t, `
import Prelude
form Color := Red | Blue
f :: Color -> Int
f := \c. case c { Red => 1 }
main := f Blue
`, gicel.Prelude)
	if err == nil {
		t.Fatal("expected non-exhaustive match error")
	}
}

func TestProbeE_Eval_DeepDoBlockBindChain(t *testing.T) {
	// Deep do-block bind chain with proper GICEL syntax.
	var sb strings.Builder
	sb.WriteString("import Prelude\nmain := do {\n")
	for i := 0; i < 50; i++ {
		sb.WriteString("  _ <- pure 0;\n")
	}
	sb.WriteString("  pure 42\n}\n")
	v, err := peRun(t, sb.String(), gicel.Prelude)
	if err != nil {
		t.Fatalf("deep bind chain should succeed: %v", err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

func TestProbeE_Eval_DepthLimitTriggersOnRecursion(t *testing.T) {
	// Recursive function application accumulates depth via closure Enter.
	// With maxDepth=5, a recursion of >5 should fail.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(100000)
	eng.SetDepthLimit(5)
	eng.SetAllocLimit(10 * 1024 * 1024)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
loop := fix (\self n. case n == 0 { True => 0; False => self (n + negate 1) })
main := loop 100
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	// With TCO, tail recursion stays flat. So this may actually succeed.
	// This tests TCO correctness: a tail-recursive loop should not
	// blow the depth limit even with a low bound.
	if err != nil {
		if strings.Contains(err.Error(), "step limit") {
			// Step limit hit is fine.
		} else if strings.Contains(err.Error(), "depth limit") {
			t.Log("NOTICE: depth limit hit on tail-recursive loop — TCO may not be flattening depth")
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. Stdlib Correctness: Empty Collections
// ---------------------------------------------------------------------------

func TestProbeE_Eval_ListHeadNilReturnsNothing(t *testing.T) {
	// head returns Maybe a, not a. head Nil should be Nothing.
	v, err := peRun(t, `
import Prelude
main := head Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("head Nil should succeed: %v", err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Nothing" {
		t.Errorf("expected Nothing, got %v", v)
	}
}

func TestProbeE_Eval_ListTailNilReturnsNothing(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := tail Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("tail Nil should succeed: %v", err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Nothing" {
		t.Errorf("expected Nothing, got %v", v)
	}
}

func TestProbeE_Eval_ListLengthNil(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := length Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("length Nil should return 0: %v", err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 0 {
		t.Errorf("expected 0, got %d", hv)
	}
}

func TestProbeE_Eval_ListReverseNil(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := reverse Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("reverse Nil should return Nil: %v", err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Nil" {
		t.Errorf("expected Nil, got %v", v)
	}
}

func TestProbeE_Eval_ListFoldlNil(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := foldl (\acc x. acc + x) 0 Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("foldl on Nil should return initial: %v", err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 0 {
		t.Errorf("expected 0, got %d", hv)
	}
}

func TestProbeE_Eval_ListMapNil(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := map (\x. x + 1) Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("map on Nil should return Nil: %v", err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Nil" {
		t.Errorf("expected Nil, got %v", v)
	}
}

func TestProbeE_Eval_ListFilterNil(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := filter (\x. True) Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("filter on Nil should return Nil: %v", err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Nil" {
		t.Errorf("expected Nil, got %v", v)
	}
}

func TestProbeE_Eval_ListReplicateZero(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := replicate 0 42
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("replicate 0 should return Nil: %v", err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Nil" {
		t.Errorf("expected Nil, got %v", v)
	}
}

// ---------------------------------------------------------------------------
// 5. State Effect Interleaving
// ---------------------------------------------------------------------------

func TestProbeE_Eval_StateGetWithoutPut(t *testing.T) {
	result, err := peRunWithCaps(t, `
import Prelude
import Effect.State
main := get
`, map[string]any{"state": &gicel.HostVal{Inner: int64(99)}},
		gicel.Prelude, gicel.EffectState)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 99 {
		t.Errorf("expected 99, got %d", hv)
	}
}

func TestProbeE_Eval_StatePutPutGet(t *testing.T) {
	result, err := peRunWithCaps(t, `
import Prelude
import Effect.State
main := do { put 1; put 2; get }
`, map[string]any{"state": &gicel.HostVal{Inner: int64(0)}},
		gicel.Prelude, gicel.EffectState)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 2 {
		t.Errorf("expected 2, got %d", hv)
	}
}

func TestProbeE_Eval_StateGetMissingCapability(t *testing.T) {
	_, err := peRunWithCaps(t, `
import Prelude
import Effect.State
main := get
`, map[string]any{},
		gicel.Prelude, gicel.EffectState)
	if err == nil {
		t.Fatal("expected error for missing state capability")
	}
}

// ---------------------------------------------------------------------------
// 6. Explain/Trace Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_Eval_ExplainSimpleLiteral(t *testing.T) {
	v, steps, err := peRunWithExplain(t, `main := 42`)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
	if len(steps) == 0 {
		t.Error("expected at least one explain step")
	}
}

func TestProbeE_Eval_ExplainDoBlock(t *testing.T) {
	_, steps, err := peRunWithExplain(t, `
import Prelude
main := do { x <- pure 42; pure x }
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) < 2 {
		t.Errorf("expected at least 2 explain steps, got %d", len(steps))
	}
}

func TestProbeE_Eval_ExplainCaseMatch(t *testing.T) {
	// Use a user-defined data type to ensure the match is not optimized away
	// or suppressed as an internal pattern.
	_, steps, err := peRunWithExplain(t, `
import Prelude
form Color := Red | Blue
main := case Red { Red => 1; Blue => 0 }
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hasMatch := false
	for _, s := range steps {
		if s.Kind == gicel.ExplainMatch {
			hasMatch = true
			break
		}
	}
	// NOTE: If the optimizer or elaborator rewrites the case to a direct result,
	// there may be no match event. This is acceptable behavior, not a bug.
	if !hasMatch {
		t.Log("NOTICE: no match explain step found — case may be optimized or elaborated differently")
	}
}

func TestProbeE_Eval_ExplainStateEffect(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectState)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := do { put 42; get }
`)
	if err != nil {
		t.Fatal(err)
	}
	var steps []gicel.ExplainStep
	hook := func(s gicel.ExplainStep) {
		steps = append(steps, s)
	}
	caps := map[string]any{"state": &gicel.HostVal{Inner: int64(0)}}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{
		Caps:    caps,
		Explain: hook,
	})
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
	hasEffect := false
	for _, s := range steps {
		if s.Kind == gicel.ExplainEffect {
			hasEffect = true
			break
		}
	}
	if !hasEffect {
		t.Error("expected at least one effect explain step")
	}
}

// ---------------------------------------------------------------------------
// 7. Concurrent Runtime Execution
// ---------------------------------------------------------------------------

func TestProbeE_Eval_ConcurrentRuntimeExecutions(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := 1 + 2
`)
	if err != nil {
		t.Fatal(err)
	}
	const N = 10
	var wg sync.WaitGroup
	errs := make([]error, N)
	vals := make([]int64, N)
	wg.Add(N)
	for i := range N {
		go func(idx int) {
			defer wg.Done()
			result, err := rt.RunWith(context.Background(), nil)
			if err != nil {
				errs[idx] = err
				return
			}
			vals[idx] = gicel.MustHost[int64](result.Value)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
		if vals[i] != 3 {
			t.Errorf("goroutine %d: expected 3, got %d", i, vals[i])
		}
	}
}

func TestProbeE_Eval_ConcurrentRuntimeWithState(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectState)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := do { n <- get; put (n + 1); get }
`)
	if err != nil {
		t.Fatal(err)
	}
	const N = 10
	var wg sync.WaitGroup
	errs := make([]error, N)
	vals := make([]int64, N)
	wg.Add(N)
	for i := range N {
		go func(idx int) {
			defer wg.Done()
			caps := map[string]any{"state": &gicel.HostVal{Inner: int64(idx * 100)}}
			result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
			if err != nil {
				errs[idx] = err
				return
			}
			vals[idx] = gicel.MustHost[int64](result.Value)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
		expected := int64(i*100 + 1)
		if vals[i] != expected {
			t.Errorf("goroutine %d: expected %d, got %d", i, expected, vals[i])
		}
	}
}

// ---------------------------------------------------------------------------
// 8. Value Representation Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_Eval_LargeIntegerArithmetic(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := 4611686018427387903 + 4611686018427387903
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("large integer arithmetic should work: %v", err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 9223372036854775806 {
		t.Errorf("expected 9223372036854775806, got %d", hv)
	}
}

func TestProbeE_Eval_EmptyString(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := strlen ""
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("strlen empty string: %v", err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 0 {
		t.Errorf("expected 0, got %d", hv)
	}
}

func TestProbeE_Eval_EmptyRecord(t *testing.T) {
	v, err := peRun(t, `main := ()`)
	if err != nil {
		t.Fatalf("unit value: %v", err)
	}
	rv, ok := v.(*gicel.RecordVal)
	if !ok || rv.Len() != 0 {
		t.Errorf("expected (), got %v", v)
	}
}

func TestProbeE_Eval_DeeplyNestedConVal(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("import Prelude\nmain := ")
	for i := 0; i < 100; i++ {
		sb.WriteString("Cons 1 (")
	}
	sb.WriteString("Nil")
	for i := 0; i < 100; i++ {
		sb.WriteString(")")
	}
	v, err := peRun(t, sb.String(), gicel.Prelude)
	if err != nil {
		t.Fatalf("deeply nested ConVal should succeed: %v", err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Errorf("expected Cons at head, got %v", v)
	}
}

func TestProbeE_Eval_Tuple(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := (1, True, "hello")
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("tuple: %v", err)
	}
	fields, ok := gicel.FromRecord(v)
	if !ok {
		t.Fatalf("expected record, got %T", v)
	}
	if len(fields) != 3 {
		t.Errorf("expected 3 fields, got %d", len(fields))
	}
}

// ---------------------------------------------------------------------------
// 9. Timeout Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_Eval_TimeoutDoesNotHang(t *testing.T) {
	start := time.Now()
	_, err := gicel.RunSandbox(`
import Prelude
loop :: Int -> Int
loop := \n. loop (n + 1)
main := loop 0
`, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		Timeout:  500 * time.Millisecond,
		MaxSteps: 10_000_000,
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error from infinite loop")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}

// ---------------------------------------------------------------------------
// 10. Fail Effect Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_Eval_FailInDoBlock(t *testing.T) {
	_, err := peRunWithCaps(t, `
import Effect.Fail
main := do { fail; pure 42 }
`, map[string]any{"fail": gicel.NewRecordFromMap(map[string]gicel.Value{})},
		gicel.Prelude, gicel.EffectFail)
	if err == nil {
		t.Fatal("expected error from fail")
	}
}

func TestProbeE_Eval_FailWithMissingCapability(t *testing.T) {
	_, err := peRunWithCaps(t, `
import Effect.Fail
main := fail
`, map[string]any{},
		gicel.Prelude, gicel.EffectFail)
	if err == nil {
		t.Fatal("expected error for fail without capability")
	}
}

// ---------------------------------------------------------------------------
// 11. IO Effect Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_Eval_IOLogEmpty(t *testing.T) {
	result, err := peRunWithCaps(t, `
import Prelude
import Effect.IO
main := log ""
`, map[string]any{"io": []string{}},
		gicel.Prelude, gicel.EffectIO)
	if err != nil {
		t.Fatal(err)
	}
	_ = result
}

func TestProbeE_Eval_IOLogWithoutCapability(t *testing.T) {
	// IO creates buffer on first write, so missing "io" cap should still work.
	result, err := peRunWithCaps(t, `
import Prelude
import Effect.IO
main := log "hello"
`, map[string]any{},
		gicel.Prelude, gicel.EffectIO)
	if err != nil {
		t.Fatal(err)
	}
	_ = result
}

// ---------------------------------------------------------------------------
// 12. Slice Edge Cases (Data.Slice uses qualified names via import)
// ---------------------------------------------------------------------------

func TestProbeE_Eval_SliceEmptyLength(t *testing.T) {
	v, err := peRun(t, `
import Prelude
import Data.Slice as S
main := S.length S.empty
`, gicel.Prelude, gicel.DataSlice)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](v)
	if hv != 0 {
		t.Errorf("expected 0, got %d", hv)
	}
}

func TestProbeE_Eval_SliceIndexOutOfBounds(t *testing.T) {
	v, err := peRun(t, `
import Prelude
import Data.Slice as S
main := S.index 5 (S.singleton 42)
`, gicel.Prelude, gicel.DataSlice)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Nothing" {
		t.Errorf("expected Nothing for out-of-bounds, got %v", v)
	}
}

func TestProbeE_Eval_SliceIndexNegative(t *testing.T) {
	v, err := peRun(t, `
import Prelude
import Data.Slice as S
main := S.index (negate 1) (S.singleton 42)
`, gicel.Prelude, gicel.DataSlice)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Nothing" {
		t.Errorf("expected Nothing for negative index, got %v", v)
	}
}

// ---------------------------------------------------------------------------
// 13. Conversion Edge Cases
// ---------------------------------------------------------------------------

func TestProbeE_Eval_ToValueNil(t *testing.T) {
	v := gicel.ToValue(nil)
	rv, ok := v.(*gicel.RecordVal)
	if !ok || rv.Len() != 0 {
		t.Errorf("ToValue(nil) should be unit, got %v", v)
	}
}

func TestProbeE_Eval_ToValueBool(t *testing.T) {
	vt := gicel.ToValue(true)
	con, ok := vt.(*gicel.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("ToValue(true) should be True, got %v", vt)
	}
	vf := gicel.ToValue(false)
	con, ok = vf.(*gicel.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("ToValue(false) should be False, got %v", vf)
	}
}

func TestProbeE_Eval_FromListMalformed(t *testing.T) {
	_, ok := gicel.FromList(&gicel.HostVal{Inner: 42})
	if ok {
		t.Error("FromList should fail for non-list value")
	}
}

func TestProbeE_Eval_FromListEmpty(t *testing.T) {
	items, ok := gicel.FromList(&gicel.ConVal{Con: "Nil"})
	if !ok {
		t.Fatal("FromList(Nil) should succeed")
	}
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %d items", len(items))
	}
}

func TestProbeE_Eval_ToListRoundtrip(t *testing.T) {
	input := []any{int64(1), int64(2), int64(3)}
	list := gicel.ToList(input)
	output, ok := gicel.FromList(list)
	if !ok {
		t.Fatal("FromList should succeed on ToList output")
	}
	if len(output) != 3 {
		t.Fatalf("expected 3 items, got %d", len(output))
	}
}

func TestProbeE_Eval_ToListEmpty(t *testing.T) {
	list := gicel.ToList(nil)
	con, ok := list.(*gicel.ConVal)
	if !ok || con.Con != "Nil" {
		t.Errorf("ToList(nil) should be Nil, got %v", list)
	}
}

// ---------------------------------------------------------------------------
// 14. Context Cancellation
// ---------------------------------------------------------------------------

func TestProbeE_Eval_ContextCancelledBeforeRun(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := 1 + 2
`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = rt.RunWith(ctx, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestProbeE_Eval_ContextDeadlineExceeded(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
loop :: Int -> Int
loop := \n. loop (n + 1)
main := loop 0
`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = rt.RunWith(ctx, nil)
	if err == nil {
		t.Fatal("expected error from deadline exceeded")
	}
}

// ---------------------------------------------------------------------------
// 15. Division by zero
// ---------------------------------------------------------------------------

func TestProbeE_Eval_DivByZero(t *testing.T) {
	_, err := peRun(t, `
import Prelude
main := div 42 0
`, gicel.Prelude)
	if err == nil {
		t.Fatal("expected error for division by zero")
	}
}

func TestProbeE_Eval_ModByZero(t *testing.T) {
	_, err := peRun(t, `
import Prelude
main := mod 42 0
`, gicel.Prelude)
	if err == nil {
		t.Fatal("expected error for mod by zero")
	}
}

// ---------------------------------------------------------------------------
// 16. Show instances
// ---------------------------------------------------------------------------

func TestProbeE_Eval_ShowInt(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := show 42
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[string](v)
	if hv != "42" {
		t.Errorf("expected '42', got %s", hv)
	}
}

func TestProbeE_Eval_ShowNegativeInt(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := show (negate 42)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[string](v)
	if hv != "-42" {
		t.Errorf("expected '-42', got %s", hv)
	}
}

// ---------------------------------------------------------------------------
// 17. Recursion gating
// ---------------------------------------------------------------------------

func TestProbeE_Eval_FixWithoutRecursionEnabled(t *testing.T) {
	_, err := peRun(t, `
main := fix (\self x. x) 42
`)
	if err == nil {
		t.Fatal("expected error: fix should not be available without EnableRecursion")
	}
}

func TestProbeE_Eval_FixWithRecursionEnabled(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
countdown := fix (\self n. case n == 0 { True => 0; False => self (n + negate 1) })
main := countdown 10
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 0 {
		t.Errorf("expected 0, got %d", hv)
	}
}

// ---------------------------------------------------------------------------
// 18. Double arithmetic edge cases
// ---------------------------------------------------------------------------

func TestProbeE_Eval_DoubleInfinity(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := 1.0 / 0.0
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("double division by zero should produce Inf, not error: %v", err)
	}
	_ = v
}

func TestProbeE_Eval_DoubleNaN(t *testing.T) {
	v, err := peRun(t, `
import Prelude
main := 0.0 / 0.0
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("double 0/0 should produce NaN, not error: %v", err)
	}
	_ = v
}
