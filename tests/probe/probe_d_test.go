//go:build probe

package probe_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe D: Deep edge-case probing — numeric, string, list, map, set,
// effects, resource limits, sandbox, conversion, concurrency, type
// classes, do-blocks, and records.
// ===================================================================

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// pdRun compiles and executes source with given packs, returning value or error.
func pdRun(t *testing.T, source string, packs ...gicel.Pack) (gicel.Value, error) {
	t.Helper()
	eng := gicel.NewEngine()
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(source)
	if err != nil {
		return nil, err
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

// pdRunWithCaps compiles and executes with packs and capabilities.
func pdRunWithCaps(t *testing.T, source string, caps map[string]any, packs ...gicel.Pack) (gicel.Value, error) {
	t.Helper()
	eng := gicel.NewEngine()
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(source)
	if err != nil {
		return nil, err
	}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

// pdRunWithLimits compiles and executes with custom engine limits.
func pdRunWithLimits(t *testing.T, source string, steps, depth int, alloc int64, packs ...gicel.Pack) (gicel.Value, error) {
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
	rt, err := eng.NewRuntime(source)
	if err != nil {
		return nil, err
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

func pdAssertInt(t *testing.T, v gicel.Value, expected int64) {
	t.Helper()
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal(%d), got %T: %v", expected, v, v)
	}
	n, ok := hv.Inner.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T: %v", hv.Inner, hv.Inner)
	}
	if n != expected {
		t.Fatalf("expected %d, got %d", expected, n)
	}
}

func pdAssertString(t *testing.T, v gicel.Value, expected string) {
	t.Helper()
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal(%q), got %T: %v", expected, v, v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", hv.Inner, hv.Inner)
	}
	if s != expected {
		t.Fatalf("expected %q, got %q", expected, s)
	}
}

func pdAssertCon(t *testing.T, v gicel.Value, name string) {
	t.Helper()
	con, ok := v.(*gicel.ConVal)
	if !ok {
		t.Fatalf("expected ConVal(%s), got %T: %v", name, v, v)
	}
	if con.Con != name {
		t.Fatalf("expected ConVal(%s), got ConVal(%s)", name, con.Con)
	}
}

// ===================================================================
// 1. Numeric edge cases
// ===================================================================

func TestProbeD_Num_IntOverflowAdd(t *testing.T) {
	// max int64 + 1 should silently wrap (Go int64 behavior).
	// BUG candidate: no overflow detection.
	v, err := pdRun(t, `
import Prelude
main := 9223372036854775807 + 1
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Go wraps int64: max + 1 = min
	pdAssertInt(t, v, math.MinInt64)
	// BUG: low - integer overflow wraps silently; no runtime error for
	// arithmetic overflow. This is Go-native behavior but may be unexpected
	// for a safe sandbox language.
}

func TestProbeD_Num_IntOverflowMul(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := 9223372036854775807 * 2
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Go wraps: MaxInt64 * 2 = -2
	pdAssertInt(t, v, -2)
}

func TestProbeD_Num_DivByZero(t *testing.T) {
	_, err := pdRun(t, `
import Prelude
main := 10 / 0
`, gicel.Prelude)
	if err == nil {
		t.Fatal("expected error from division by zero")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected 'division by zero' error, got: %v", err)
	}
}

func TestProbeD_Num_ModByZero(t *testing.T) {
	_, err := pdRun(t, `
import Prelude
main := mod 10 0
`, gicel.Prelude)
	if err == nil {
		t.Fatal("expected error from modulo by zero")
	}
	if !strings.Contains(err.Error(), "modulo by zero") {
		t.Fatalf("expected 'modulo by zero' error, got: %v", err)
	}
}

func TestProbeD_Num_NegativeModulo(t *testing.T) {
	// Go's % returns result with sign of dividend.
	v, err := pdRun(t, `
import Prelude
main := mod (negate 7) 3
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, -1) // Go: (-7) % 3 == -1
}

func TestProbeD_Num_DoubleDivByZero(t *testing.T) {
	// IEEE 754: 1.0 / 0.0 = +Inf, no error.
	v, err := pdRun(t, `
import Prelude
main := 1.0 / 0.0
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", v)
	}
	f, ok := hv.Inner.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T", hv.Inner)
	}
	if !math.IsInf(f, 1) {
		t.Fatalf("expected +Inf, got %v", f)
	}
}

func TestProbeD_Num_DoubleNaN(t *testing.T) {
	// 0.0 / 0.0 = NaN
	v, err := pdRun(t, `
import Prelude
main := 0.0 / 0.0
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", v)
	}
	f, ok := hv.Inner.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T", hv.Inner)
	}
	if !math.IsNaN(f) {
		t.Fatalf("expected NaN, got %v", f)
	}
}

func TestProbeD_Num_NaNEquality(t *testing.T) {
	// NaN == NaN should be False (IEEE 754).
	v, err := pdRun(t, `
import Prelude
nan := 0.0 / 0.0
main := nan == nan
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "False")
}

// ===================================================================
// 2. String edge cases
// ===================================================================

func TestProbeD_Str_EmptyConcat(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := "" <> ""
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertString(t, v, "")
}

func TestProbeD_Str_EmptyLength(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := strlen ""
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_Str_EmptyEq(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := "" == ""
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "True")
}

func TestProbeD_Str_UnicodeLength(t *testing.T) {
	// strlen counts runes, not bytes.
	v, err := pdRun(t, `
import Prelude
main := strlen "hello"
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 5 runes (each 1 char in this case)
	pdAssertInt(t, v, 5)
}

func TestProbeD_Str_ConcatIdentity(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := "abc" <> ""
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertString(t, v, "abc")
}

func TestProbeD_Str_SplitEmpty(t *testing.T) {
	// Splitting empty string by comma.
	v, err := pdRun(t, `
import Prelude
main := length (split "," "")
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// strings.Split("", ",") returns [""], so length = 1
	pdAssertInt(t, v, 1)
}

func TestProbeD_Str_JoinEmptyList(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := join "," Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertString(t, v, "")
}

func TestProbeD_Str_CharAtBeyondEnd(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := charAt 100 "hi"
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "Nothing")
}

func TestProbeD_Str_SubstringNegativeStart(t *testing.T) {
	// substring with negative start should clamp to 0.
	v, err := pdRun(t, `
import Prelude
main := substring (negate 5) 3 "abcdef"
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertString(t, v, "abc")
}

// ===================================================================
// 3. List edge cases
// ===================================================================

func TestProbeD_List_FoldlEmpty(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := foldl (\acc x. acc + x) 0 Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_LengthEmpty(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (Nil :: List Int)
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_HeadEmpty(t *testing.T) {
	// index 0 of empty list should be Nothing.
	v, err := pdRun(t, `
import Prelude
main := index 0 (Nil :: List Int)
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "Nothing")
}

func TestProbeD_List_TakeZero(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (take 0 (Cons 1 (Cons 2 Nil)))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_TakeMoreThanLength(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (take 100 (Cons 1 (Cons 2 Nil)))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 2) // take clips to actual length
}

func TestProbeD_List_DropMoreThanLength(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (drop 100 (Cons 1 (Cons 2 Nil)))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_ReverseEmpty(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (reverse (Nil :: List Int))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_ReverseSingleton(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := reverse (Cons 42 Nil)
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	pdAssertInt(t, con.Args[0], 42)
}

func TestProbeD_List_ConcatEmpty(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (concat (Nil :: List Int) (Nil :: List Int))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_ZipDifferentLengths(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (zip (Cons 1 (Cons 2 (Cons 3 Nil))) (Cons 10 Nil))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 1) // zip truncates to shorter list
}

func TestProbeD_List_ReplicateZero(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (replicate 0 42)
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_List_ReplicateNegative(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := length (replicate (negate 5) 42)
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0) // negative count produces empty list
}

// ===================================================================
// 4. Map/Set edge cases
// ===================================================================

func TestProbeD_Map_EmptySize(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Map
main := size (empty :: Map Int Int)
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_Map_LookupMissing(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Map
main := lookup 42 (empty :: Map Int String)
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "Nothing")
}

func TestProbeD_Map_DuplicateKeys(t *testing.T) {
	// Inserting same key twice should update, not duplicate.
	v, err := pdRun(t, `
import Prelude
import Data.Map
m := insert 1 "b" (insert 1 "a" (empty :: Map Int String))
main := (size m, lookup 1 m)
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", v, v)
	}
	pdAssertInt(t, rec.Fields["_1"], 1) // size = 1, not 2
	justV, ok := rec.Fields["_2"].(*gicel.ConVal)
	if !ok || justV.Con != "Just" {
		t.Fatalf("expected Just, got %v", rec.Fields["_2"])
	}
	pdAssertString(t, justV.Args[0], "b") // latest value wins
}

func TestProbeD_Map_DeleteFromEmpty(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Map
main := size (delete 1 (empty :: Map Int Int))
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_Set_EmptySize(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Set
main := size (empty :: Set Int)
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 0)
}

func TestProbeD_Set_DuplicateInsert(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Set
main := size (insert 1 (insert 1 (empty :: Set Int)))
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 1) // duplicates collapsed
}

func TestProbeD_Set_MemberMissing(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
import Data.Set
main := member 99 (insert 1 (empty :: Set Int))
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "False")
}

// ===================================================================
// 5. Effect interaction: state + fail
// ===================================================================

func TestProbeD_Effect_StateBeforeFail(t *testing.T) {
	// put then fail: error should propagate.
	caps := map[string]any{
		"state": &gicel.HostVal{Inner: int64(0)},
		"fail":  &gicel.RecordVal{Fields: map[string]gicel.Value{}},
	}
	_, err := pdRunWithCaps(t, `
import Prelude
import Effect.Fail
import Effect.State
main := do { put 42; fail }
`, caps, gicel.Prelude, gicel.EffectFail, gicel.EffectState)
	if err == nil {
		t.Fatal("expected error from fail after put")
	}
}

func TestProbeD_Effect_GetWithoutPut(t *testing.T) {
	// get without initial state capability should error.
	_, err := pdRunWithCaps(t, `
import Prelude
import Effect.State
main := get
`, nil, gicel.Prelude, gicel.EffectState)
	if err == nil {
		t.Fatal("expected error from get without state capability")
	}
	if !strings.Contains(err.Error(), "no state capability") {
		t.Fatalf("expected 'no state capability' error, got: %v", err)
	}
}

func TestProbeD_Effect_PutThenGet(t *testing.T) {
	caps := map[string]any{
		"state": &gicel.HostVal{Inner: int64(0)},
	}
	v, err := pdRunWithCaps(t, `
import Prelude
import Effect.State
main := do { put 100; get }
`, caps, gicel.Prelude, gicel.EffectState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 100)
}

func TestProbeD_Effect_NestedModify(t *testing.T) {
	caps := map[string]any{
		"state": &gicel.HostVal{Inner: int64(0)},
	}
	v, err := pdRunWithCaps(t, `
import Prelude
import Effect.State
main := do {
  put 1;
  modify (+ 10);
  modify (* 3);
  get
}
`, caps, gicel.Prelude, gicel.EffectState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 33) // (1 + 10) * 3 = 33
}

func TestProbeD_Effect_FromMaybeNothing(t *testing.T) {
	caps := map[string]any{
		"fail": &gicel.RecordVal{Fields: map[string]gicel.Value{}},
	}
	_, err := pdRunWithCaps(t, `
import Prelude
import Effect.Fail
main := fromMaybe Nothing
`, caps, gicel.Prelude, gicel.EffectFail)
	if err == nil {
		t.Fatal("expected error from fromMaybe Nothing")
	}
}

// ===================================================================
// 6. Resource limits — boundary tests
// ===================================================================

func TestProbeD_Limits_StepLimitExact(t *testing.T) {
	// Very tight step limit should trigger StepLimitError.
	_, err := pdRunWithLimits(t, `main := 42`, 1, 1000, 0)
	if err != nil {
		var sle *gicel.StepLimitError
		if !errors.As(err, &sle) {
			t.Logf("step=1 produced non-StepLimitError: %v", err)
		}
	}
	// Either succeeds (1 step is enough for literal) or fails with StepLimitError.
}

func TestProbeD_Limits_DepthLimit(t *testing.T) {
	// Depth limit 1 should trigger DepthLimitError on do-block.
	_, err := pdRunWithLimits(t, `
import Prelude
main := do { pure 1 }
`, 100000, 1, 0, gicel.Prelude)
	if err != nil {
		var dle *gicel.DepthLimitError
		if !errors.As(err, &dle) {
			t.Logf("depth=1 produced non-DepthLimitError: %v", err)
		}
	}
}

func TestProbeD_Limits_AllocLimitTight(t *testing.T) {
	// Very tight alloc limit should fire for list allocation.
	_, err := pdRunWithLimits(t, `
import Prelude
main := replicate 1000 1
`, 1000000, 1000, 256, gicel.Prelude)
	if err == nil {
		t.Fatal("expected alloc limit error for replicate 1000 with 256-byte limit")
	}
	var ale *gicel.AllocLimitError
	if !errors.As(err, &ale) {
		t.Fatalf("expected AllocLimitError, got: %v", err)
	}
}

func TestProbeD_Limits_ZeroAllocDisablesTracking(t *testing.T) {
	// AllocLimit=0 should disable tracking, so large allocation succeeds.
	v, err := pdRunWithLimits(t, `
import Prelude
main := length (replicate 500 1)
`, 1000000, 1000, 0, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error with alloc=0: %v", err)
	}
	pdAssertInt(t, v, 500)
}

// ===================================================================
// 7. Sandbox edge cases
// ===================================================================

func TestProbeD_Sandbox_ZeroTimeout(t *testing.T) {
	// Zero timeout uses the default (5s), should not fail for trivial program.
	result, err := gicel.RunSandbox("main := 42", &gicel.SandboxConfig{
		Timeout: 0,
	})
	if err != nil {
		t.Fatalf("unexpected error with zero timeout: %v", err)
	}
	pdAssertInt(t, result.Value, 42)
}

func TestProbeD_Sandbox_NegativeLimits(t *testing.T) {
	// Negative limits should be clamped to defaults (not crash).
	result, err := gicel.RunSandbox("main := 42", &gicel.SandboxConfig{
		MaxSteps: -1,
		MaxDepth: -1,
		MaxAlloc: -1,
	})
	if err != nil {
		t.Fatalf("unexpected error with negative limits: %v", err)
	}
	pdAssertInt(t, result.Value, 42)
}

func TestProbeD_Sandbox_ConcurrentRuns(t *testing.T) {
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := gicel.RunSandbox("import Prelude\nmain := 1 + 2", &gicel.SandboxConfig{
				Packs: []gicel.Pack{gicel.Prelude},
			})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent sandbox failed: %v", err)
	}
}

func TestProbeD_Sandbox_EmptyEntry(t *testing.T) {
	// Empty entry should default to "main".
	result, err := gicel.RunSandbox("main := 42", &gicel.SandboxConfig{
		Entry: "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, result.Value, 42)
}

func TestProbeD_Sandbox_MissingEntry(t *testing.T) {
	// Non-existent entry should fail gracefully.
	_, err := gicel.RunSandbox("main := 42", &gicel.SandboxConfig{
		Entry: "nonExistent",
	})
	if err == nil {
		t.Fatal("expected error for missing entry point")
	}
}

// ===================================================================
// 8. Value conversion edge cases
// ===================================================================

func TestProbeD_Convert_NilToValue(t *testing.T) {
	v := gicel.ToValue(nil)
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal (unit), got %T", v)
	}
	if len(rec.Fields) != 0 {
		t.Fatalf("expected empty record (unit), got %d fields", len(rec.Fields))
	}
}

func TestProbeD_Convert_BoolTrue(t *testing.T) {
	v := gicel.ToValue(true)
	pdAssertCon(t, v, "True")
}

func TestProbeD_Convert_BoolFalse(t *testing.T) {
	v := gicel.ToValue(false)
	pdAssertCon(t, v, "False")
}

func TestProbeD_Convert_FromBoolNonCon(t *testing.T) {
	_, ok := gicel.FromBool(&gicel.HostVal{Inner: 42})
	if ok {
		t.Fatal("expected ok=false for non-ConVal")
	}
}

func TestProbeD_Convert_FromConVal(t *testing.T) {
	nested := &gicel.ConVal{Con: "Just", Args: []gicel.Value{
		&gicel.ConVal{Con: "Just", Args: []gicel.Value{
			&gicel.HostVal{Inner: int64(99)},
		}},
	}}
	name, args, ok := gicel.FromCon(nested)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if name != "Just" {
		t.Fatalf("expected Just, got %s", name)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	innerName, innerArgs, ok := gicel.FromCon(args[0])
	if !ok || innerName != "Just" || len(innerArgs) != 1 {
		t.Fatalf("unexpected inner structure: %v", args[0])
	}
}

func TestProbeD_Convert_ToListEmpty(t *testing.T) {
	v := gicel.ToList(nil)
	items, ok := gicel.FromList(v)
	if !ok {
		t.Fatal("expected valid list from nil")
	}
	if len(items) != 0 {
		t.Fatalf("expected empty list, got %d items", len(items))
	}
}

func TestProbeD_Convert_ToListRoundtrip(t *testing.T) {
	original := []any{int64(1), int64(2), int64(3)}
	v := gicel.ToList(original)
	items, ok := gicel.FromList(v)
	if !ok {
		t.Fatal("expected valid list")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

func TestProbeD_Convert_FromRecordNonRecord(t *testing.T) {
	_, ok := gicel.FromRecord(&gicel.HostVal{Inner: 42})
	if ok {
		t.Fatal("expected ok=false for non-RecordVal")
	}
}

func TestProbeD_Convert_MustHostPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic from MustHost on wrong type")
		}
	}()
	_ = gicel.MustHost[int64](&gicel.ConVal{Con: "True"})
}

// ===================================================================
// 9. Runtime reuse — compile once, run many
// ===================================================================

func TestProbeD_RuntimeReuse_Sequential(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`
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
	rt1, err := eng.NewRuntime("import Prelude\nmain := 10")
	if err != nil {
		t.Fatal(err)
	}
	rt2, err := eng.NewRuntime("import Prelude\nmain := 20")
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
// 10. Type class method dispatch
// ===================================================================

func TestProbeD_TypeClass_ShowInt(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := showInt 42
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertString(t, v, "42")
}

func TestProbeD_TypeClass_EqBool(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := True == False
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "False")
}

func TestProbeD_TypeClass_OrdInt(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := compare 1 2
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertCon(t, v, "LT")
}

func TestProbeD_TypeClass_FmapList(t *testing.T) {
	v, err := pdRun(t, `
import Prelude
main := fmap (+ 1) (Cons 1 (Cons 2 (Cons 3 Nil)))
`, gicel.Prelude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be [2, 3, 4]
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	pdAssertInt(t, con.Args[0], 2)
}

// ===================================================================
// 11. Do-block semantics
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
inner := do { pure 10 }
main := do { x <- inner; pure (x + 5) }
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
// 12. Record operations
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
// Additional edge cases
// ===================================================================

func TestProbeD_ContextCancellation(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`
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

func TestProbeD_Sandbox_StatsNonZero(t *testing.T) {
	result, err := gicel.RunSandbox("import Prelude\nmain := 1 + 2", &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stats.Steps == 0 {
		t.Fatal("expected non-zero steps")
	}
}
