//go:build probe

// Sandbox recovery, resource limits, and sandbox edge-case tests.
package probe_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cwd-k2/gicel"
	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/host/stdlib"
)

// ===================================================================
// Probe C: Sandbox recovery — panics during compilation must not crash host
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
// Probe C: Resource limits — exact boundary tests
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
	rt, err := eng.NewRuntime(context.Background(), `
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
	// Verify stats report correct step count. The source must contain
	// at least one CBPV reduction (App/Force/Bind/Case/PrimOp/Fix) so
	// the assertion is well-defined: bare literals like `main := 42`
	// emit no steps in the bytecode (Lit is a value form per
	// compileExpr's emitStep gate). Importing Prelude and using `+`
	// produces a saturated PrimOp call that DOES emit a step.
	result, err := probeSandbox("import Prelude\nmain := 1 + 2", &gicel.SandboxConfig{
		Packs:    []registry.Pack{stdlib.Prelude},
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
// Probe D: Resource limits — boundary tests
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
// Probe D: Sandbox edge cases
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
