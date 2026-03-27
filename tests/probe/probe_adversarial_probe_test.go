//go:build probe

// Adversarial probe tests — precise boundary checks for resource limit
// bypasses, runtime error handling, and compile-time defense gaps.
// Does NOT cover: stress-level throughput tests (tests/stress/stress_adversarial_test.go).
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
// String literal allocation bypass (known vulnerability)
// ===================================================================

// TestProbeE_StringLiteral_AllocEnforced verifies that string literals
// are correctly charged against MaxAlloc.
func TestProbeE_StringLiteral_AllocEnforced(t *testing.T) {
	cases := []struct {
		name     string
		strLen   int
		maxAlloc int64
	}{
		{"10KB_str_vs_1KB_limit", 10_000, 1024},
		{"100KB_str_vs_1KB_limit", 100_000, 1024},
		{"1MB_str_vs_1KB_limit", 1_000_000, 1024},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := `main := "` + strings.Repeat("X", tc.strLen) + `"`
			_, err := probeSandbox(src, &gicel.SandboxConfig{
				MaxAlloc: tc.maxAlloc,
			})
			if err == nil {
				t.Fatal("expected AllocLimitError for string literal exceeding MaxAlloc")
			}
			var ale *gicel.AllocLimitError
			if !errors.As(err, &ale) {
				t.Fatalf("expected AllocLimitError, got: %v", err)
			}
		})
	}
}

// TestProbeE_RuntimeAlloc_StillTracked confirms that runtime-constructed
// values (closures, constructors, records, string concat) ARE tracked.
func TestProbeE_RuntimeAlloc_StillTracked(t *testing.T) {
	t.Run("constructor_chain", func(t *testing.T) {
		src := `
import Prelude
main := Just (Just (Just (Just (Just (Just (Just True))))))
`
		_, err := probeSandbox(src, &gicel.SandboxConfig{
			Packs:    []gicel.Pack{gicel.Prelude},
			MaxAlloc: 64, // very tight
		})
		if err == nil {
			t.Fatal("expected alloc limit for constructor chain with 64-byte limit")
		}
		var ale *gicel.AllocLimitError
		if !errors.As(err, &ale) {
			t.Logf("got non-alloc error (acceptable): %v", err)
		}
	})

	t.Run("string_concat_runtime", func(t *testing.T) {
		src := `
import Prelude
a := "AAAAAAAAAA"
b := a <> a <> a <> a <> a
c := b <> b <> b <> b <> b
d := c <> c <> c <> c <> c
main := d
`
		_, err := probeSandbox(src, &gicel.SandboxConfig{
			Packs:    []gicel.Pack{gicel.Prelude},
			MaxAlloc: 256,
		})
		if err == nil {
			t.Fatal("expected alloc limit for runtime <> with 256-byte limit")
		}
		var ale *gicel.AllocLimitError
		if !errors.As(err, &ale) {
			t.Logf("got non-alloc error (acceptable): %v", err)
		}
	})
}

// ===================================================================
// Fix / recursion boundary probes
// ===================================================================

// TestProbeE_FixSelfApplication confirms self-application is bounded.
func TestProbeE_FixSelfApplication(t *testing.T) {
	src := `
import Prelude
loop := fix (\self x. self x)
main := loop 0
`
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(1000)

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected step limit from fix self-application")
	}
	var sle *gicel.StepLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("expected StepLimitError, got: %v", err)
	}
}

// TestProbeE_FixExponential confirms exponential branching hits step limit.
func TestProbeE_FixExponential(t *testing.T) {
	src := `
import Prelude
bomb := fix (\self n. case n == 0 { True => 0; False => self (n - 1) + self (n - 1) })
main := bomb 30
`
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(50_000)

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected step limit from 2^30 recursion")
	}
	var sle *gicel.StepLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("expected StepLimitError, got: %v", err)
	}
}

// TestProbeE_FixAllocBomb confirms alloc limit catches Cons-accumulation.
func TestProbeE_FixAllocBomb(t *testing.T) {
	src := `
import Prelude
grow := fix (\self acc. self (Cons 1 acc))
main := grow Nil
`
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(1_000_000)
	eng.SetAllocLimit(10_000) // 10 KB

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected resource error from Cons-accumulation bomb")
	}
	// May hit alloc or step first.
	var ale *gicel.AllocLimitError
	var sle *gicel.StepLimitError
	if !errors.As(err, &ale) && !errors.As(err, &sle) {
		t.Fatalf("expected AllocLimitError or StepLimitError, got: %v", err)
	}
}

// ===================================================================
// Type system defenses
// ===================================================================

// TestProbeE_TypeFamilyReductionFuel confirms infinite type family loops
// are caught by the reduction fuel mechanism.
func TestProbeE_TypeFamilyReductionFuel(t *testing.T) {
	src := `
import Prelude
type Loop :: Type := \(a: Type). case a { a => Loop (Maybe a) }
x :: Loop Int
x := 42
main := x
`
	_, err := probeSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err == nil {
		t.Fatal("expected type family reduction limit error")
	}
	if !strings.Contains(err.Error(), "reduction limit") {
		t.Fatalf("expected 'reduction limit', got: %v", err)
	}
}

// TestProbeE_OverlappingInstances confirms overlap detection.
func TestProbeE_OverlappingInstances(t *testing.T) {
	src := `
import Prelude
form C := \a. { m: a -> a }
impl C Int := { m := \x. x }
impl C a := { m := \x. x }
main := m 42
`
	_, err := probeSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err == nil {
		t.Fatal("expected overlapping instance error")
	}
	if !strings.Contains(err.Error(), "overlapping") {
		t.Fatalf("expected 'overlapping', got: %v", err)
	}
}

// TestProbeE_NonExhaustivePatterns confirms compile-time exhaustiveness check.
func TestProbeE_NonExhaustivePatterns(t *testing.T) {
	src := `
import Prelude
form D := A | B | C
f := \x. case x { A => 1; B => 2 }
main := f C
`
	_, err := probeSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err == nil {
		t.Fatal("expected non-exhaustive pattern error")
	}
	if !strings.Contains(err.Error(), "non-exhaustive") {
		t.Fatalf("expected 'non-exhaustive', got: %v", err)
	}
}

// ===================================================================
// Runtime error handling
// ===================================================================

// TestProbeE_DivisionByZero confirms clean runtime error, not panic.
func TestProbeE_DivisionByZero(t *testing.T) {
	_, err := probeSandbox("import Prelude\nmain := 10 / 0", &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err == nil {
		t.Fatal("expected division by zero error")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected 'division by zero', got: %v", err)
	}
}

// TestProbeE_IntegerOverflow documents wrapping behavior.
func TestProbeE_IntegerOverflow(t *testing.T) {
	result, err := probeSandbox("import Prelude\nmain := 9223372036854775807 + 1", &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, -9223372036854775808)
}

// ===================================================================
// Timeout and limit edge cases
// ===================================================================

// TestProbeE_Timeout_CoversCompilation confirms timeout applies to the
// entire pipeline (not just evaluation).
func TestProbeE_Timeout_CoversCompilation(t *testing.T) {
	// Build a large source to stress compilation.
	var sb strings.Builder
	sb.WriteString("import Prelude\n")
	for i := range 5000 {
		sb.WriteString("x" + strings.Repeat("a", 100) + "_" + string(rune('0'+i%10)) + " := 42\n")
	}
	sb.WriteString("main := 42\n")

	start := time.Now()
	_, err := probeSandbox(sb.String(), &gicel.SandboxConfig{
		Packs:   []gicel.Pack{gicel.Prelude},
		Timeout: time.Nanosecond,
	})
	elapsed := time.Since(start)
	// Either succeeds (fast machine) or times out.
	_ = err
	// Must not take more than a few seconds regardless.
	if elapsed > 5*time.Second {
		t.Fatalf("compilation was not bounded by timeout: %v", elapsed)
	}
}

// TestProbeE_NanosecondTimeout_NoRace confirms no data race or panic
// with an extremely short timeout.
func TestProbeE_NanosecondTimeout_NoRace(t *testing.T) {
	for range 10 {
		_, _ = probeSandbox("import Prelude\nmain := 1 + 2", &gicel.SandboxConfig{
			Packs:   []gicel.Pack{gicel.Prelude},
			Timeout: time.Nanosecond,
		})
	}
	// Success: no race detector complaints or panics.
}

// TestProbeE_HighLimits_TimeoutStillFires confirms that even with extreme
// step/depth limits, timeout terminates runaway execution.
func TestProbeE_HighLimits_TimeoutStillFires(t *testing.T) {
	src := `
import Prelude
loop := fix (\self n. self (n + 1))
main := loop 0
`
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(999_999_999)
	eng.SetDepthLimit(999_999_999)

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	_, err = rt.RunWith(ctx, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("timeout didn't fire within expected window: %v", elapsed)
	}
}

// ===================================================================
// Parser resilience probes
// ===================================================================

// TestProbeE_ParserRecursionLimit confirms the parser halts on deep nesting.
func TestProbeE_ParserRecursionLimit(t *testing.T) {
	src := "main := " + strings.Repeat("(", 300) + "42" + strings.Repeat(")", 300)
	_, err := probeSandbox(src, &gicel.SandboxConfig{})
	if err == nil {
		t.Fatal("expected parser error for 300-deep nesting")
	}
	if !strings.Contains(err.Error(), "recursion depth") {
		t.Logf("error was: %v", err)
	}
}

// TestProbeE_ParserStepLimit confirms the parser halts on huge token streams.
func TestProbeE_ParserStepLimit(t *testing.T) {
	// 10000-deep operator chain → >40000 tokens
	var sb strings.Builder
	sb.WriteString("import Prelude\nmain := 0")
	for range 10_000 {
		sb.WriteString(" + 1")
	}
	_, err := probeSandbox(sb.String(), &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	// May succeed (parser step limit = tokens*4) or error; must not hang.
	_ = err
}

// TestProbeE_NullBytes confirms null bytes in source are handled gracefully.
func TestProbeE_NullBytes(t *testing.T) {
	src := "import Prelude\x00\x00\x00\nmain := 42\x00"
	_, err := probeSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	// Must not panic. Error is expected.
	_ = err
}

// TestProbeE_UnterminatedString confirms unterminated string produces
// a clean error, not a hang.
func TestProbeE_UnterminatedString(t *testing.T) {
	_, err := probeSandbox(`main := "hello`, &gicel.SandboxConfig{})
	if err == nil {
		t.Fatal("expected error from unterminated string")
	}
	if !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("expected 'unterminated', got: %v", err)
	}
}

// ===================================================================
// V6: Parser hang on multiline instance method body
// ===================================================================

// TestProbeE_V6_MultilineInstanceMethod confirms that a multiline
// function application in an instance method body does not hang the
// parser. Before the fix, newline + `(` caused parseBody to loop
// infinitely because the stagnation check was shadowed by the
// implicit-separator branch.
func TestProbeE_V6_MultilineInstanceMethod(t *testing.T) {
	src := `
import Prelude

form Comonad := \w. Functor w => {
  extract: \a. w a -> a;
  extend: \a b. (w a -> b) -> w a -> w b;
}

form Z := \a. { MkZ: List a -> a -> List a -> Z a; }

impl Functor Z := {
  fmap := \f z. case z { MkZ ls c rs => MkZ (map f ls) (f c) (map f rs) }
}

impl Comonad Z := {
  extract := \z. case z { MkZ _ c _ => c };
  extend := \f z. MkZ
    (map f Nil)
    (f z)
    (map f Nil)
}

main := 0
`
	start := time.Now()
	_, err := probeSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	elapsed := time.Since(start)

	// Must terminate quickly — the old bug caused an infinite loop.
	if elapsed > 3*time.Second {
		t.Fatalf("parser appears to hang: took %v", elapsed)
	}

	// Currently produces parse errors (multiline continuation not yet
	// supported in instance bodies). An error is acceptable; a hang is not.
	if err == nil {
		t.Log("multiline instance method parsed successfully (continuation support may have been added)")
	}
}
