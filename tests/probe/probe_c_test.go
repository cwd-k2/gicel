//go:build probe

package probe_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe C: Runtime / Evaluator / Stdlib adversarial tests
// ===================================================================

// ---------------------------------------------------------------------------
// helpers (local to probe C)
// ---------------------------------------------------------------------------

// probeRun compiles and executes source with given packs, returning value or error.
func probeRun(t *testing.T, source string, packs ...gicel.Pack) (gicel.Value, error) {
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

// probeSandbox is a convenience wrapper around RunSandbox.
func probeSandbox(source string, cfg *gicel.SandboxConfig) (*gicel.RunResult, error) {
	return gicel.RunSandbox(source, cfg)
}

// probeAssertConVal checks the value is a ConVal with the given constructor name.
func probeAssertConVal(t *testing.T, v gicel.Value, name string) {
	t.Helper()
	con, ok := v.(*gicel.ConVal)
	if !ok {
		t.Fatalf("expected ConVal(%s), got %T: %v", name, v, v)
	}
	if con.Con != name {
		t.Fatalf("expected ConVal(%s), got ConVal(%s)", name, con.Con)
	}
}

// probeAssertHostInt checks the value is a HostVal wrapping the given int64.
func probeAssertHostInt(t *testing.T, v gicel.Value, expected int64) {
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

// probeAssertHostString checks the value is a HostVal wrapping the given string.
func probeAssertHostString(t *testing.T, v gicel.Value, expected string) {
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

// ===================================================================
// 1. Sandbox recovery — panics during compilation must not crash host
// ===================================================================

func TestProbeC_SandboxRecovery_EmptySource(t *testing.T) {
	_, err := probeSandbox("", nil)
	if err == nil {
		t.Fatal("expected error from empty source")
	}
	// Empty source has no main binding — should get a meaningful error.
}

func TestProbeC_SandboxRecovery_OnlyComments(t *testing.T) {
	_, err := probeSandbox("-- just a comment\n-- nothing else", nil)
	if err == nil {
		t.Fatal("expected error from comment-only source")
	}
}

func TestProbeC_SandboxRecovery_OnlyWhitespace(t *testing.T) {
	_, err := probeSandbox("   \n\t\n  ", nil)
	if err == nil {
		t.Fatal("expected error from whitespace-only source")
	}
}

func TestProbeC_SandboxRecovery_MalformedUnicode(t *testing.T) {
	_, err := probeSandbox("main := \xff\xfe", nil)
	if err == nil {
		t.Fatal("expected error from malformed unicode source")
	}
}

func TestProbeC_SandboxRecovery_NilConfig(t *testing.T) {
	// RunSandbox with nil config should not panic.
	_, err := probeSandbox("main := pure ()", nil)
	// Either compiles (unit) or errors — but must not panic.
	_ = err
}

func TestProbeC_SandboxRecovery_DeeplyNestedLambdas(t *testing.T) {
	// Stress: deeply nested lambdas might overflow parser/checker.
	src := "main := "
	for i := 0; i < 200; i++ {
		src += "\\x. "
	}
	src += "x"
	_, err := probeSandbox(src, nil)
	// May compile or error, but must not panic.
	_ = err
}

func TestProbeC_SandboxRecovery_HugeIntLiteral(t *testing.T) {
	// A very large integer literal — tests lexer/parser resilience.
	src := "main := " + strings.Repeat("9", 500)
	_, err := probeSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	// Either parses or errors, but must not panic or OOM.
	_ = err
}

func TestProbeC_SandboxRecovery_HugeStringLiteral(t *testing.T) {
	src := `main := "` + strings.Repeat("a", 10000) + `"`
	result, err := probeSandbox(src, nil)
	if err != nil {
		t.Fatalf("expected success for large string, got: %v", err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", result.Value)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string inner, got %T", hv.Inner)
	}
	if len(s) != 10000 {
		t.Fatalf("expected 10000-char string, got %d", len(s))
	}
}

// ===================================================================
// 2. Resource limits — exact boundary tests
// ===================================================================

func TestProbeC_Limits_StepLimit1_PureLiteral(t *testing.T) {
	// Step limit 1: pure literal "main := 42" should require very few steps.
	// Check if it succeeds or fails, and that the error is a StepLimitError.
	_, err := probeSandbox("main := 42", &gicel.SandboxConfig{
		MaxSteps: 1,
	})
	// With step limit 1, the first evalStep uses 1 step for the literal.
	// But module evaluation (Core) may consume steps first.
	if err != nil {
		var sle *gicel.StepLimitError
		if !errors.As(err, &sle) {
			t.Logf("step limit 1 produced non-StepLimitError: %v", err)
		}
	}
}

func TestProbeC_Limits_StepLimit2(t *testing.T) {
	// With step limit = 2, a minimal program might just barely work or fail.
	_, err := probeSandbox("main := 42", &gicel.SandboxConfig{
		MaxSteps: 2,
	})
	if err != nil {
		var sle *gicel.StepLimitError
		if !errors.As(err, &sle) {
			t.Logf("step limit 2 produced non-StepLimitError: %v", err)
		}
	}
}

func TestProbeC_Limits_DepthLimit1(t *testing.T) {
	// Depth limit 1: any function application should hit it.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.SetDepthLimit(1)
	rt, err := eng.NewRuntime(`
import Prelude
main := id 42
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	// id application increases depth — should hit limit.
	if err != nil {
		var dle *gicel.DepthLimitError
		if !errors.As(err, &dle) {
			t.Logf("depth limit 1 produced non-DepthLimitError: %v", err)
		}
	}
}

func TestProbeC_Limits_AllocLimit0(t *testing.T) {
	// Alloc limit 0 means unlimited (see sandbox.go: if maxAlloc <= 0, use default).
	// So alloc limit 0 is NOT zero-byte budget — it's the default.
	result, err := probeSandbox("main := 42", &gicel.SandboxConfig{
		MaxAlloc: 0, // should use default (10 MiB)
	})
	if err != nil {
		t.Fatalf("alloc limit 0 (default) should work for trivial program: %v", err)
	}
	probeAssertHostInt(t, result.Value, 42)
}

func TestProbeC_Limits_AllocLimit1(t *testing.T) {
	// Alloc limit 1 byte — almost anything should exceed this.
	// But wait: alloc limit <= 0 uses default. So limit=1 is actually 1 byte.
	_, err := probeSandbox(`
import Prelude
main := Just True
`, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxAlloc: 1,
	})
	if err == nil {
		t.Fatal("expected alloc limit error with 1-byte budget")
	}
	var ale *gicel.AllocLimitError
	if !errors.As(err, &ale) {
		t.Logf("alloc limit 1 produced non-AllocLimitError: %v", err)
	}
}

func TestProbeC_Limits_NegativeStepLimit(t *testing.T) {
	// Negative step limit: sandbox.go checks <= 0 → use default.
	result, err := probeSandbox("main := 42", &gicel.SandboxConfig{
		MaxSteps: -1,
	})
	if err != nil {
		t.Fatalf("negative step limit should use default, got: %v", err)
	}
	probeAssertHostInt(t, result.Value, 42)
}

func TestProbeC_Limits_NegativeDepthLimit(t *testing.T) {
	result, err := probeSandbox("main := 42", &gicel.SandboxConfig{
		MaxDepth: -1,
	})
	if err != nil {
		t.Fatalf("negative depth limit should use default, got: %v", err)
	}
	probeAssertHostInt(t, result.Value, 42)
}

func TestProbeC_Limits_NegativeAllocLimit(t *testing.T) {
	result, err := probeSandbox("main := 42", &gicel.SandboxConfig{
		MaxAlloc: -1,
	})
	if err != nil {
		t.Fatalf("negative alloc limit should use default, got: %v", err)
	}
	probeAssertHostInt(t, result.Value, 42)
}

func TestProbeC_Limits_TimeoutVeryShort(t *testing.T) {
	// 1 nanosecond timeout — should expire before anything completes.
	_, err := probeSandbox("import Prelude\nmain := 1 + 2", &gicel.SandboxConfig{
		Packs:   []gicel.Pack{gicel.Prelude},
		Timeout: time.Nanosecond,
	})
	if err == nil {
		// Might succeed on very fast machines, but usually should fail.
		t.Log("surprisingly succeeded with 1ns timeout")
	}
}

func TestProbeC_Limits_StepLimitStatsAccurate(t *testing.T) {
	// Verify stats report correct step count.
	result, err := probeSandbox("main := 42", &gicel.SandboxConfig{
		MaxSteps: 100000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stats.Steps == 0 {
		t.Error("expected non-zero steps in stats")
	}
}

// ===================================================================
// 3. Pattern matching edge cases
// ===================================================================

func TestProbeC_Pattern_FirstMatchWins(t *testing.T) {
	// Overlapping patterns: checker rejects redundant patterns.
	// Verify compile-time rejection (not a runtime issue).
	_, err := probeRun(t, `
import Prelude
f := \x. case x { True -> 1; True -> 2; _ -> 3 }
main := f True
`, gicel.Prelude)
	if err == nil {
		t.Fatal("expected compile error for redundant patterns")
	}
	if !strings.Contains(err.Error(), "redundant") {
		t.Logf("expected 'redundant' in error, got: %v", err)
	}
}

func TestProbeC_Pattern_WildcardCatchAll(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
f := \x. case x { True -> 1; _ -> 2 }
main := f False
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 2)
}

func TestProbeC_Pattern_LiteralIntMatch(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
f := \x. case x { 0 -> "zero"; 1 -> "one"; _ -> "other" }
main := f 0
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "zero")
}

func TestProbeC_Pattern_LiteralIntMatchOther(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
f := \x. case x { 0 -> "zero"; 1 -> "one"; _ -> "other" }
main := f 99
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "other")
}

func TestProbeC_Pattern_LiteralStringMatch(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
f := \x. case x { "hello" -> True; _ -> False }
main := f "hello"
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "True")
}

func TestProbeC_Pattern_LiteralStringMismatch(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
f := \x. case x { "hello" -> True; _ -> False }
main := f "world"
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "False")
}

func TestProbeC_Pattern_NestedConstructor(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
f := \x. case x { Just (Just y) -> y; _ -> False }
main := f (Just (Just True))
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "True")
}

func TestProbeC_Pattern_NestedConstructorFallthrough(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
f := \x. case x { Just (Just y) -> y; _ -> False }
main := f (Just Nothing)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "False")
}

func TestProbeC_Pattern_NonExhaustive_CompileError(t *testing.T) {
	// Non-exhaustive pattern is caught at compile time by the checker.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(`
import Prelude
f := \x. case x { True -> 1 }
main := f False
`)
	if err == nil {
		t.Fatal("expected non-exhaustive pattern compile error")
	}
	if !strings.Contains(err.Error(), "non-exhaustive") {
		t.Fatalf("expected 'non-exhaustive' in error, got: %v", err)
	}
}

func TestProbeC_Pattern_NonExhaustive_CaughtAtCompile(t *testing.T) {
	// The checker enforces exhaustiveness even for Int literal patterns.
	// Verify that case on Int without a wildcard is rejected at compile time.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(`
import Prelude
f := \x. case x { 0 -> "zero"; 1 -> "one" }
main := f 99
`)
	if err == nil {
		t.Fatal("expected non-exhaustive pattern compile error")
	}
	if !strings.Contains(err.Error(), "non-exhaustive") {
		t.Fatalf("expected 'non-exhaustive' in error, got: %v", err)
	}
}

func TestProbeC_Pattern_CaseOnLiteral(t *testing.T) {
	// Case directly on an integer literal.
	v, err := probeRun(t, `
import Prelude
main := case 42 { 42 -> True; _ -> False }
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "True")
}

func TestProbeC_Pattern_CaseOnEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
f := \xs. case xs { Nil -> "empty"; Cons _ _ -> "nonempty" }
main := f Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "empty")
}

func TestProbeC_Pattern_CaseOnSingletonList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
f := \xs. case xs { Nil -> 0; Cons x Nil -> 1; Cons _ _ -> 2 }
main := f (Cons True Nil)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 1)
}

// ===================================================================
// 4. Capability environment edge cases
// ===================================================================

func TestProbeC_CapEnv_EmptyCapsPureProgram(t *testing.T) {
	// No caps, no effects — should work fine.
	result, err := probeSandbox("main := 42", nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 42)
}

func TestProbeC_CapEnv_StateWithoutCap(t *testing.T) {
	// Using state without providing the "state" capability should fail.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectState)
	rt, err := eng.NewRuntime(`
import Prelude
import Effect.State
main := do { get }
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when using state without providing state cap")
	}
}

func TestProbeC_CapEnv_FailWithoutCap(t *testing.T) {
	// Using fail without providing the "fail" capability should fail.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectFail)
	rt, err := eng.NewRuntime(`
import Effect.Fail
main := do { fail }
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when using fail without providing fail cap")
	}
}

func TestProbeC_CapEnv_StatePutGet(t *testing.T) {
	// Basic put/get to verify cap env threading.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectState)
	rt, err := eng.NewRuntime(`
import Prelude
import Effect.State
main := do { put 99; get }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &gicel.HostVal{Inner: int64(0)}}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 99)
}

func TestProbeC_CapEnv_CapEnvIsolation(t *testing.T) {
	// Multiple executions of the same runtime should not share cap state.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectState)
	rt, err := eng.NewRuntime(`
import Prelude
import Effect.State
main := do { n <- get; put (n + 1); get }
`)
	if err != nil {
		t.Fatal(err)
	}

	caps1 := map[string]any{"state": &gicel.HostVal{Inner: int64(0)}}
	result1, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps1})
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result1.Value, 1)

	// Second run with fresh caps — should start from 0 again.
	caps2 := map[string]any{"state": &gicel.HostVal{Inner: int64(0)}}
	result2, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps2})
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result2.Value, 1)
}

// ===================================================================
// 5. Stdlib correctness — list, map, set, show edge cases
// ===================================================================

func TestProbeC_Stdlib_HeadEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := head Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nothing")
}

func TestProbeC_Stdlib_TailEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := tail Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nothing")
}

func TestProbeC_Stdlib_NullEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := null Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "True")
}

func TestProbeC_Stdlib_NullSingletonList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := null (Cons 1 Nil)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "False")
}

func TestProbeC_Stdlib_MapEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := map (\x. x + 1) Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nil")
}

func TestProbeC_Stdlib_FilterEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := filter (\x. True) Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nil")
}

func TestProbeC_Stdlib_FoldlEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := foldl (\acc x. acc + x) 0 Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

func TestProbeC_Stdlib_FoldrEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := foldr (\x acc. x + acc) 0 Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

func TestProbeC_Stdlib_ReverseEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := reverse Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nil")
}

func TestProbeC_Stdlib_ReverseSingletonList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := reverse (Cons 42 Nil)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %v", v)
	}
	if len(con.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(con.Args))
	}
	hv, ok := con.Args[0].(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", con.Args[0])
	}
	if hv.Inner != int64(42) {
		t.Fatalf("expected 42, got %v", hv.Inner)
	}
}

func TestProbeC_Stdlib_LengthEmptyList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := length Nil
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

func TestProbeC_Stdlib_ShowInt(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := show 0
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "0")
}

func TestProbeC_Stdlib_ShowNegativeInt(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := showInt (negate 42)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "-42")
}

func TestProbeC_Stdlib_ShowBoolTrue(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := show True
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "True")
}

func TestProbeC_Stdlib_ShowUnit(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := show ()
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "()")
}

func TestProbeC_Stdlib_ShowMaybeJust(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := show (Just 42)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %v", v, v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T", hv.Inner)
	}
	if !strings.Contains(s, "Just") || !strings.Contains(s, "42") {
		t.Fatalf("expected show (Just 42) to contain 'Just' and '42', got %q", s)
	}
}

func TestProbeC_Stdlib_ShowNothing(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := show (Nothing :: Maybe Int)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %v", v, v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T", hv.Inner)
	}
	if !strings.Contains(s, "Nothing") {
		t.Fatalf("expected 'Nothing' in show output, got %q", s)
	}
}

func TestProbeC_Stdlib_ShowListEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := show (Nil :: List Int)
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %v", v, v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T", hv.Inner)
	}
	// Should represent an empty list somehow (e.g. "[]" or "Nil").
	if s == "" {
		t.Fatal("show of empty list produced empty string")
	}
}

func TestProbeC_Stdlib_DivisionByZero(t *testing.T) {
	// Division by zero should produce an error, not a panic.
	_, err := probeRun(t, `
import Prelude
main := div 1 0
`, gicel.Prelude)
	if err == nil {
		t.Fatal("expected error from division by zero")
	}
	// Verify no panic propagation — the error itself is sufficient.
}

func TestProbeC_Stdlib_ModByZero(t *testing.T) {
	_, err := probeRun(t, `
import Prelude
main := mod 1 0
`, gicel.Prelude)
	if err == nil {
		t.Fatal("expected error from mod by zero")
	}
}

func TestProbeC_Stdlib_StringAppendEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := append "" ""
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "")
}

func TestProbeC_Stdlib_StrLenEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
main := strlen ""
`, gicel.Prelude)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

// ===================================================================
// 5b. Data.Slice edge cases
// ===================================================================

func TestProbeC_Stdlib_SliceEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Slice
main := length (empty :: Slice Int)
`, gicel.Prelude, gicel.DataSlice)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

func TestProbeC_Stdlib_SliceSingleton(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Slice
main := length (singleton 42)
`, gicel.Prelude, gicel.DataSlice)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 1)
}

// ===================================================================
// 5c. Data.Map edge cases
// ===================================================================

func TestProbeC_Stdlib_MapEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Map
main := size (empty :: Map Int Int)
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

func TestProbeC_Stdlib_MapInsertLookup(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Map
main := lookup 1 (insert 1 42 (empty :: Map Int Int))
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", v)
	}
}

func TestProbeC_Stdlib_MapLookupMissing(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Map
main := lookup 99 (empty :: Map Int Int)
`, gicel.Prelude, gicel.DataMap)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "Nothing")
}

// ===================================================================
// 5d. Data.Set edge cases
// ===================================================================

func TestProbeC_Stdlib_SetEmpty(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.Set
main := size (empty :: Set Int)
`, gicel.Prelude, gicel.DataSet)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 0)
}

// ===================================================================
// 6. Module interaction
// ===================================================================

func TestProbeC_Module_QualifiedConstructor(t *testing.T) {
	eng := gicel.NewEngine()
	err := eng.RegisterModule("Lib", `
data Color := Red | Blue | Green
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Lib
main := Red
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, result.Value, "Red")
}

func TestProbeC_Module_QualifiedFunction(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	err := eng.RegisterModule("Lib", `
import Prelude
double :: Int -> Int
double := \x. x + x
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import Lib
main := double 21
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 42)
}

func TestProbeC_Module_PrivateNotExported(t *testing.T) {
	// _prefixed names should not be exported.
	eng := gicel.NewEngine()
	err := eng.RegisterModule("Lib", `
data Bool := True | False
_internal := True
public := _internal
`)
	if err != nil {
		t.Fatal(err)
	}
	// Accessing public should work.
	rt, err := eng.NewRuntime(`
import Lib
main := public
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, result.Value, "True")
}

func TestProbeC_Module_PrivateDirectAccess(t *testing.T) {
	// Attempting to access _internal directly should fail at compile time.
	eng := gicel.NewEngine()
	err := eng.RegisterModule("Lib", `
data Bool := True | False
_secret := True
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.NewRuntime(`
import Lib
main := _secret
`)
	if err == nil {
		t.Fatal("expected error accessing private binding from another module")
	}
}

func TestProbeC_Module_EmptyModule(t *testing.T) {
	// Register a module with only data declarations (no value bindings).
	eng := gicel.NewEngine()
	err := eng.RegisterModule("Lib", `
data Unit := Unit
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Lib
main := Unit
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, result.Value, "Unit")
}

func TestProbeC_Module_MultiModule_ChainedImport(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	err := eng.RegisterModule("A", `
import Prelude
valA :: Int
valA := 10
`)
	if err != nil {
		t.Fatal(err)
	}
	err = eng.RegisterModule("B", `
import Prelude
import A
valB :: Int
valB := valA + 20
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import B
main := valB
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 30)
}

func TestProbeC_Module_RegisterModuleSyntaxError(t *testing.T) {
	eng := gicel.NewEngine()
	err := eng.RegisterModule("Bad", `this is not valid @@@@`)
	if err == nil {
		t.Fatal("expected error for module with syntax errors")
	}
}

func TestProbeC_Module_DuplicateRegistration(t *testing.T) {
	eng := gicel.NewEngine()
	err := eng.RegisterModule("Lib", `data U := U`)
	if err != nil {
		t.Fatal(err)
	}
	err = eng.RegisterModule("Lib", `data V := V`)
	if err == nil {
		t.Fatal("expected error for duplicate module registration")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected 'already registered' in error, got: %v", err)
	}
}

func TestProbeC_Module_EmptyModuleName(t *testing.T) {
	eng := gicel.NewEngine()
	err := eng.RegisterModule("", `data U := U`)
	if err == nil {
		t.Fatal("expected error for empty module name")
	}
}

// ===================================================================
// 7. Edge cases — thunks, entry points, empty programs
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
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`x := 42`)
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
	rt, err := eng.NewRuntime(`main := 42`)
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
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`main := hostVal`)
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
	rt, err := eng.NewRuntime(`
import Prelude
import Effect.State
inner := do { put 10; get }
main := do { put 0; inner }
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
// 8. Error message quality
// ===================================================================

func TestProbeC_Error_CompileErrorHasPosition(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.NewRuntime(`main := undefined_variable`)
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
	_, err := eng.NewRuntime(`main := @@@`)
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
	_, err := eng.NewRuntime(`
import Prelude
f := \x. case x { 0 -> "zero"; 1 -> "one" }
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
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`
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
	_, err := eng.NewRuntime(`
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
// 9. Goroutine safety — concurrent executions
// ===================================================================

func TestProbeC_Concurrency_MultipleRuns(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(`
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
// 10. Regression / adversarial — edge-case compositions
// ===================================================================

func TestProbeC_Regression_CaseOnTuple(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
f := \p. case p { (x, y) -> x + y }
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
	rt, err := eng.NewRuntime(`
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
