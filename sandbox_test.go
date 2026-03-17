package gicel_test

import (
	"strings"
	"testing"
	"time"

	"github.com/cwd-k2/gicel"
)

func TestSandboxRun(t *testing.T) {
	result, err := gicel.RunSandbox("import Prelude\nmain := True", &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertConVal(t, result.Value, "True")
}

func TestSandboxRunWithPacks(t *testing.T) {
	result, err := gicel.RunSandbox("import Prelude\nmain := 1 + 2", &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHostInt(t, result.Value, 3)
}

func TestSandboxRunTimeout(t *testing.T) {
	// This should not hang — step limit will catch infinite recursion.
	_, err := gicel.RunSandbox(`
import Prelude
loop :: Int -> Int
loop := \n. loop (n + 1)
main := loop 0
`, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxSteps: 1000,
	})
	if err == nil {
		t.Fatal("expected error from infinite loop")
	}
}

func TestSandboxRunCompileError(t *testing.T) {
	_, err := gicel.RunSandbox(`main := x`, nil)
	if err == nil {
		t.Fatal("expected compile error")
	}
}

func TestSandboxRunJSON(t *testing.T) {
	result, err := gicel.RunSandbox("import Prelude\nmain := True", &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stats.Steps == 0 {
		t.Fatal("expected non-zero steps")
	}
}

func TestSandboxDeepNesting(t *testing.T) {
	// Deeply nested expression — parser depth limit should catch this.
	src := "main := "
	for range 500 {
		src += "("
	}
	src += "True"
	for range 500 {
		src += ")"
	}
	_, err := gicel.RunSandbox(src, nil)
	if err == nil {
		t.Fatal("expected error from deeply nested expression")
	}
}

func TestSandboxMalformedDo(t *testing.T) {
	// Malformed do-block that previously caused infinite loop.
	_, err := gicel.RunSandbox("main := do { let x = 42; pure x }", nil)
	if err == nil {
		t.Fatal("expected error from invalid do-block")
	}
}

func TestSandboxDeepRecursion(t *testing.T) {
	// Deep recursion — depth limit should catch.
	_, err := gicel.RunSandbox(`
import Prelude
f :: Int -> Int
f := \n. f (n + 1)
main := f 0
`, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxSteps: 500,
		MaxDepth: 50,
	})
	if err == nil {
		t.Fatal("expected error from deep recursion")
	}
}

// Regression: timeout clock must start before compilation so that
// compile + execute total is bounded by the configured timeout.
func TestSandboxTimeoutCoversCompilation(t *testing.T) {
	start := time.Now()
	_, err := gicel.RunSandbox("import Prelude\nmain := 1 + 2", &gicel.SandboxConfig{
		Packs:   []gicel.Pack{gicel.Prelude},
		Timeout: time.Nanosecond,
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error: nanosecond timeout should expire")
	}
	// The total call should not exceed a generous bound.
	if elapsed > 5*time.Second {
		t.Fatalf("RunSandbox took %v, expected bounded by timeout + compile", elapsed)
	}
}

// Regression: stdlib allocations (list reverse, slice ops) must be charged
// against MaxAlloc so that the alloc limit fires for stdlib-heavy programs.
func TestSandboxAllocLimitFiresForStdlib(t *testing.T) {
	// Build a large list and reverse it — stdlib allocates proportionally.
	// With a very tight alloc limit, this must fail.
	_, err := gicel.RunSandbox(`
import Prelude
main := reverse (replicate 500 1)
`, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxAlloc: 1024, // 1 KiB — far too small for 500-element reverse
	})
	if err == nil {
		t.Fatal("expected alloc limit error for stdlib-heavy program")
	}
	if !strings.Contains(err.Error(), "allocation limit exceeded") {
		t.Fatalf("expected allocation limit error, got: %v", err)
	}
}

func TestSandboxRunAllPacks(t *testing.T) {
	result, err := gicel.RunSandbox("import Prelude\nmain := showInt (foldl (\\acc x. acc + x) 0 (Cons 1 (Cons 2 (Cons 3 Nil))))", &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHostString(t, result.Value, "6")
}
