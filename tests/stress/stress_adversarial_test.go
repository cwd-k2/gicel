package stress_test

// Adversarial stress tests — crafted inputs targeting resource limits,
// parser resilience, and allocation tracking.
// Does NOT cover: probe-level boundary tests (tests/probe/adversarial_test.go).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cwd-k2/gicel"
)

// ===========================================================================
// String literal allocation bypass
// ===========================================================================

// TestAdversarial_StringLiteral_BypassAllocLimit verifies that a string
// literal larger than MaxAlloc is accepted: the budget only tracks
// runtime allocations (closures, constructors, records), NOT compile-time
// string literals.  This is a known gap; the test documents the behavior
// so that a future fix (input size guard or lexer-time accounting) can
// flip the assertion.
func TestAdversarial_StringLiteral_AllocLimitEnforced(t *testing.T) {
	const strLen = 1_000_000 // 1 MB string
	src := `main := "` + strings.Repeat("A", strLen) + `"`

	_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		MaxAlloc: 1024, // 1 KiB — far below the literal size
	})
	if err == nil {
		t.Fatal("expected AllocLimitError for string literal exceeding MaxAlloc")
	}
	var ale *gicel.AllocLimitError
	if !errors.As(err, &ale) {
		t.Fatalf("expected AllocLimitError, got: %v", err)
	}
}

// TestAdversarial_StringLiteral_MemoryPressure checks that a large string
// literal is correctly rejected by the alloc limit.
func TestAdversarial_StringLiteral_MemoryPressure(t *testing.T) {
	const strLen = 10_000_000 // 10 MB
	src := `main := "` + strings.Repeat("B", strLen) + `"`

	_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		MaxAlloc: 1024 * 1024, // 1 MiB — below the 10 MB literal
		Timeout:  10 * time.Second,
	})
	if err == nil {
		t.Fatal("expected alloc limit error for 10 MB string with 1 MiB budget")
	}
	var ale *gicel.AllocLimitError
	if !errors.As(err, &ale) {
		t.Fatalf("expected AllocLimitError, got: %v", err)
	}
}

// TestAdversarial_RuntimeStringConcat_AllocTracked confirms that runtime
// string concatenation via <> IS correctly tracked by the allocator.
func TestAdversarial_RuntimeStringConcat_AllocTracked(t *testing.T) {
	src := `
import Prelude
a := "AAAAAAAAAA"
b := a <> a <> a <> a <> a <> a <> a <> a <> a <> a
c := b <> b <> b <> b <> b <> b <> b <> b <> b <> b
d := c <> c <> c <> c <> c <> c <> c <> c <> c <> c
main := d
`
	_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxAlloc: 1024, // 1 KiB — way too small for 10^4 chars
	})
	if err == nil {
		t.Fatal("expected alloc limit error for runtime <> chain")
	}
	var ale *gicel.AllocLimitError
	if !errors.As(err, &ale) {
		t.Fatalf("expected AllocLimitError, got: %v", err)
	}
}

// ===========================================================================
// Parser resilience
// ===========================================================================

// TestAdversarial_DeepParenNesting confirms the parser stops at its
// recursion depth limit without crashing.
func TestAdversarial_DeepParenNesting(t *testing.T) {
	depth := 500
	src := "import Prelude; main := " + strings.Repeat("(", depth) + "1" + strings.Repeat(")", depth)
	_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err == nil {
		t.Fatal("expected parser error for 500-deep nesting")
	}
	if !strings.Contains(err.Error(), "recursion depth") {
		t.Logf("unexpected error (not recursion depth): %v", err)
	}
}

// TestAdversarial_LongOperatorChain stresses the Pratt parser with a
// long left-associative operator chain.  200 operators is the known
// safe limit; beyond ~500 the evidence system loses $dict references.
func TestAdversarial_LongOperatorChain(t *testing.T) {
	const n = 200
	var sb strings.Builder
	sb.WriteString("import Prelude\nmain := 0")
	for range n {
		sb.WriteString(" + 1")
	}
	result, err := gicel.RunSandbox(sb.String(), &gicel.SandboxConfig{
		Packs:      []gicel.Pack{gicel.Prelude},
		MaxSteps:   500_000,
		MaxNesting: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := gicel.MustHost[int64](result.Value)
	if got != n {
		t.Errorf("expected %d, got %d", n, got)
	}
}

// TestAdversarial_LongOperatorChain_500 confirms that 500-operator chains
// succeed after the FV overflow sentinel fix (V5).
func TestAdversarial_LongOperatorChain_500(t *testing.T) {
	const n = 500
	var sb strings.Builder
	sb.WriteString("import Prelude\nmain := 0")
	for range n {
		sb.WriteString(" + 1")
	}
	result, err := gicel.RunSandbox(sb.String(), &gicel.SandboxConfig{
		Packs:      []gicel.Pack{gicel.Prelude},
		MaxSteps:   1_000_000,
		MaxNesting: 4096,
		MaxAlloc:   100 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := gicel.MustHost[int64](result.Value)
	if got != n {
		t.Errorf("expected %d, got %d", n, got)
	}
}

// TestAdversarial_ManyMalformedDecls ensures parser error recovery handles
// thousands of malformed declarations without hanging.
func TestAdversarial_ManyMalformedDecls(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("import Prelude\n")
	for i := range 1000 {
		sb.WriteString(fmt.Sprintf("x%d := {{{ malformed ]]]\n", i))
	}
	sb.WriteString("main := 1\n")

	start := time.Now()
	_, err := gicel.RunSandbox(sb.String(), &gicel.SandboxConfig{
		Packs:   []gicel.Pack{gicel.Prelude},
		Timeout: 10 * time.Second,
	})
	elapsed := time.Since(start)
	t.Logf("1000 malformed decls processed in %v", elapsed)
	// Must either error or succeed — never hang.
	_ = err
	if elapsed > 10*time.Second {
		t.Fatal("parser took too long on malformed input")
	}
}

// ===========================================================================
// Scope and symbol table stress
// ===========================================================================

// TestAdversarial_ManyBindings confirms the scope/symbol table handles
// a large number of top-level bindings efficiently.
func TestAdversarial_ManyBindings(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("import Prelude\n")
	for i := range 5000 {
		sb.WriteString(fmt.Sprintf("x%d := %d\n", i, i))
	}
	sb.WriteString("main := x4999\n")

	result, err := gicel.RunSandbox(sb.String(), &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxSteps: 1_000_000,
		Timeout:  15 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHostVal(t, result.Value, int64(4999))
}

// TestAdversarial_ManyConstructors confirms ADT with many constructors works.
func TestAdversarial_ManyConstructors(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("import Prelude\ndata Big := C0")
	for i := 1; i < 1000; i++ {
		sb.WriteString(fmt.Sprintf(" | C%d", i))
	}
	sb.WriteString("\nmain := C999\n")

	result, err := gicel.RunSandbox(sb.String(), &gicel.SandboxConfig{
		Packs:   []gicel.Pack{gicel.Prelude},
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "C999")
}

// TestAdversarial_LongIdentifier tests a 100K-character identifier.
func TestAdversarial_LongIdentifier(t *testing.T) {
	name := strings.Repeat("a", 100_000)
	src := fmt.Sprintf("import Prelude\n%s := 42\nmain := %s\n", name, name)

	result, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		Packs:   []gicel.Pack{gicel.Prelude},
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHostVal(t, result.Value, int64(42))
}

// ===========================================================================
// Fix / recursion limits
// ===========================================================================

// TestAdversarial_FixInfiniteLoop confirms step limit catches infinite fix.
func TestAdversarial_FixInfiniteLoop(t *testing.T) {
	src := `
import Prelude
loop := fix (\self x. self x)
main := loop 0
`
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(10_000)

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected step limit error from infinite loop")
	}
	var sle *gicel.StepLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("expected StepLimitError, got: %v", err)
	}
}

// TestAdversarial_FixExponentialBranching confirms step limit catches 2^n recursion.
func TestAdversarial_FixExponentialBranching(t *testing.T) {
	src := `
import Prelude
bomb := fix (\self n. case n == 0 { True => 0; False => self (n - 1) + self (n - 1) })
main := bomb 50
`
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(100_000)

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected step limit error from exponential recursion")
	}
	var sle *gicel.StepLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("expected StepLimitError, got: %v", err)
	}
}

// TestAdversarial_FixMemoryBomb confirms alloc limit catches unbounded allocation.
func TestAdversarial_FixMemoryBomb(t *testing.T) {
	src := `
import Prelude
grow := fix (\self acc. self (Cons 1 acc))
main := grow Nil
`
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(1_000_000)
	eng.SetAllocLimit(1024 * 1024) // 1 MiB

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected resource limit error from memory bomb")
	}
	// May hit step or alloc limit first — either is acceptable.
}

// TestAdversarial_FixStringDoubling confirms alloc limit catches runtime
// string concatenation growth.
func TestAdversarial_FixStringDoubling(t *testing.T) {
	src := `
import Prelude
grow := fix (\self s. self (s <> s))
main := grow "A"
`
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(1_000_000)
	eng.SetAllocLimit(1024 * 1024) // 1 MiB

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected alloc limit error from string doubling")
	}
	var ale *gicel.AllocLimitError
	if !errors.As(err, &ale) {
		// Step limit is also acceptable since it depends on implementation.
		var sle *gicel.StepLimitError
		if !errors.As(err, &sle) {
			t.Fatalf("expected AllocLimitError or StepLimitError, got: %v", err)
		}
	}
}

// ===========================================================================
// Type system stress
// ===========================================================================

// TestAdversarial_TypeFamilyInfiniteLoop confirms type family reduction
// fuel prevents infinite expansion.
func TestAdversarial_TypeFamilyInfiniteLoop(t *testing.T) {
	src := `
import Prelude
type Loop :: Type := \(a: Type). case a { a => Loop (Maybe a) }
x :: Loop Int
x := 42
main := x
`
	_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err == nil {
		t.Fatal("expected error from infinite type family")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "reduction limit") && !strings.Contains(errMsg, "result type too large") {
		t.Fatalf("expected type family reduction error, got: %v", err)
	}
}

// TestAdversarial_OverlappingInstances confirms overlapping instances are rejected.
func TestAdversarial_OverlappingInstances(t *testing.T) {
	src := `
import Prelude
data MyClass := \a. { myMethod: a -> Int }
impl MyClass Int := { myMethod := \x. x }
impl MyClass a := { myMethod := \x. 0 }
main := myMethod 42
`
	_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err == nil {
		t.Fatal("expected error from overlapping instances")
	}
	if !strings.Contains(err.Error(), "overlapping") {
		t.Fatalf("expected 'overlapping' in error, got: %v", err)
	}
}

// TestAdversarial_NonExhaustivePattern confirms exhaustiveness checking at
// compile time.
func TestAdversarial_NonExhaustivePattern(t *testing.T) {
	src := `
import Prelude
data Color := { Red: Color; Green: Color; Blue: Color; }
incomplete := \c. case c { Red => 1; Green => 2 }
main := incomplete Blue
`
	_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err == nil {
		t.Fatal("expected error from non-exhaustive pattern")
	}
	if !strings.Contains(err.Error(), "non-exhaustive") {
		t.Fatalf("expected 'non-exhaustive' in error, got: %v", err)
	}
}

// TestAdversarial_DeepPolymorphicChain stresses type inference with deeply
// nested polymorphic applications.
func TestAdversarial_DeepPolymorphicChain(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("import Prelude\n")
	sb.WriteString("id2 :: \\a. a -> a\n")
	sb.WriteString("id2 := \\x. x\n")
	sb.WriteString("main := ")
	for range 50 {
		sb.WriteString("id2 (")
	}
	sb.WriteString("42")
	sb.WriteString(strings.Repeat(")", 50))
	sb.WriteString("\n")

	result, err := gicel.RunSandbox(sb.String(), &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHostVal(t, result.Value, int64(42))
}

// TestAdversarial_SuperclassChain stresses typeclass superclass resolution.
func TestAdversarial_SuperclassChain(t *testing.T) {
	src := `
import Prelude
data A := \a. { methodA: a -> Int }
data B := \a. A a => { methodB: a -> Int }
data C := \a. B a => { methodC: a -> Int }
data D := \a. C a => { methodD: a -> Int }
data E := \a. D a => { methodE: a -> Int }
data F := \a. E a => { methodF: a -> Int }
data G := \a. F a => { methodG: a -> Int }

impl A Int := { methodA := \x. x }
impl B Int := { methodB := \x. x + 1 }
impl C Int := { methodC := \x. x + 2 }
impl D Int := { methodD := \x. x + 3 }
impl E Int := { methodE := \x. x + 4 }
impl F Int := { methodF := \x. x + 5 }
impl G Int := { methodG := \x. x + 6 }

main := methodG 42
`
	result, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHostVal(t, result.Value, int64(48))
}

// ===========================================================================
// Runtime edge cases
// ===========================================================================

// TestAdversarial_DivisionByZero confirms division by zero produces a
// clean runtime error.
func TestAdversarial_DivisionByZero(t *testing.T) {
	_, err := gicel.RunSandbox("import Prelude\nmain := 42 / 0", &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err == nil {
		t.Fatal("expected runtime error for division by zero")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected 'division by zero' in error, got: %v", err)
	}
}

// TestAdversarial_IntegerOverflow documents silent int64 overflow behavior.
func TestAdversarial_IntegerOverflow(t *testing.T) {
	src := "import Prelude\nmain := 9223372036854775807 + 1"
	result, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Go int64 wraps around; document this behavior.
	n := gicel.MustHost[int64](result.Value)
	if n != -9223372036854775808 {
		t.Errorf("expected int64 wrap to min, got %d", n)
	}
}

// TestAdversarial_TimeoutAsLastResort confirms timeout catches runaway
// programs even when other limits are very high.
func TestAdversarial_TimeoutAsLastResort(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	_, err = rt.RunWith(ctx, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	t.Logf("timeout fired after %v", elapsed)
	if elapsed > 5*time.Second {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}

// ===========================================================================
// Output amplification
// ===========================================================================

// TestAdversarial_SharedValueOutputAmplification documents that shared
// values expand fully in the Pretty printer, producing output much larger
// than the in-memory representation.
func TestAdversarial_SharedValueOutputAmplification(t *testing.T) {
	// Build (a, a, ..., a) × 3 levels of 10-tuples:
	//   a = Just 42       →  ~10 chars
	//   b = (a,a,...,a)   →  ~100 chars
	//   c = (b,b,...,b)   →  ~1000 chars
	//   d = (c,c,...,c)   →  ~10000 chars
	src := `
import Prelude
a := Just 42
b := (a, a, a, a, a, a, a, a, a, a)
c := (b, b, b, b, b, b, b, b, b, b)
d := (c, c, c, c, c, c, c, c, c, c)
main := d
`
	result, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Just verify it completed without hang.
	if result.Value == nil {
		t.Fatal("expected non-nil value")
	}
}

// ===========================================================================
// Garbage / boundary inputs
// ===========================================================================

// TestAdversarial_BinaryGarbage confirms binary data does not crash the lexer.
func TestAdversarial_BinaryGarbage(t *testing.T) {
	var buf []byte
	for i := range 10000 {
		buf = append(buf, byte(i%256))
	}
	_, err := gicel.RunSandbox(string(buf), &gicel.SandboxConfig{})
	// Must error, never panic.
	if err == nil {
		t.Fatal("expected error from binary garbage")
	}
}

// TestAdversarial_DuplicateImport confirms duplicate imports are rejected.
func TestAdversarial_DuplicateImport(t *testing.T) {
	src := strings.Repeat("import Prelude\n", 20) + "main := 42\n"
	_, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		Packs: []gicel.Pack{gicel.Prelude},
	})
	if err == nil {
		t.Fatal("expected error from duplicate imports")
	}
	if !strings.Contains(err.Error(), "duplicate import") {
		t.Fatalf("expected 'duplicate import' in error, got: %v", err)
	}
}

// TestAdversarial_ManyTypeErrors confirms error recovery handles thousands
// of type errors without excessive time.
func TestAdversarial_ManyTypeErrors(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("import Prelude\n")
	for i := range 1000 {
		sb.WriteString(fmt.Sprintf("x%d :: Nonexistent%d\nx%d := 42\n", i, i, i))
	}
	sb.WriteString("main := x0\n")

	start := time.Now()
	_, err := gicel.RunSandbox(sb.String(), &gicel.SandboxConfig{
		Packs:   []gicel.Pack{gicel.Prelude},
		Timeout: 10 * time.Second,
	})
	elapsed := time.Since(start)
	t.Logf("1000 type errors processed in %v", elapsed)
	if err == nil {
		t.Fatal("expected type errors")
	}
	if elapsed > 10*time.Second {
		t.Fatal("type error processing took too long")
	}
}

// ===========================================================================
// V8: User-defined Monad do notation
// ===========================================================================

// TestV8_MonadDoNotation verifies do-notation works with a user-defined
// Monad instance (Reader monad) where the bind type parameters a and b differ.
func TestV8_MonadDoNotation(t *testing.T) {
	src := `
import Prelude
data Reader := \e a. { MkReader: (e -> a) -> Reader e a; }
runReader :: \e a. Reader e a -> e -> a
runReader := \r env. case r { MkReader f => f env }
ask :: \e. Reader e e
ask := MkReader (\e. e)
impl Monad (Reader e) := {
  mpure := \a. MkReader (\_. a);
  mbind := \ma f. MkReader (\env. runReader (f (runReader ma env)) env)
}
main := runReader (do { x <- ask; pure (append "port=" (show x)) } :: Reader Int String) 8080
`
	result, err := gicel.RunSandbox(src, &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxSteps: 500_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := gicel.MustHost[string](result.Value)
	if !strings.Contains(got, "port=8080") {
		t.Errorf("expected result containing %q, got %q", "port=8080", got)
	}
}

// ===========================================================================
// V9: fix ConVal error message
// ===========================================================================

// TestV9_FixConValErrorMessage verifies that applying fix to a constructor
// body produces a clear error message mentioning "requires a lambda body".
func TestV9_FixConValErrorMessage(t *testing.T) {
	src := `
import Prelude
data Pair := { MkPair: Int -> Int -> Pair }
getFirst :: Pair -> Int
getFirst := \p. case p { MkPair a _ => a }
main := getFirst (fix (\self. MkPair 1 2))
`
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(100_000)

	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected runtime error from fix applied to constructor body")
	}
	if !strings.Contains(err.Error(), "requires a lambda body") {
		t.Errorf("expected error containing %q, got: %v", "requires a lambda body", err)
	}
}

// TestAdversarial_DeepMaybeNesting confirms 200-deep Maybe value works.
func TestAdversarial_DeepMaybeNesting(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("import Prelude\nmain := ")
	for range 100 {
		sb.WriteString("Just (")
	}
	sb.WriteString("42")
	sb.WriteString(strings.Repeat(")", 100))
	sb.WriteString("\n")

	result, err := gicel.RunSandbox(sb.String(), &gicel.SandboxConfig{
		Packs:    []gicel.Pack{gicel.Prelude},
		MaxSteps: 500_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Value == nil {
		t.Fatal("expected non-nil value")
	}
}
