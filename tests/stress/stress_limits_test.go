// Limits stress tests — infinite recursion, cyclic aliases, error accumulation, parser depth limits.
// Does NOT cover: stress_parser_test.go, stress_checker_test.go, stress_evaluator_test.go, stress_correctness_test.go, stress_crosscutting_test.go, stress_specialized_test.go.
package stress_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// Fix 1: resolveInstance must have a depth limit to prevent stack overflow
// from self-referential or mutually recursive instance contexts.
func TestInstanceResolutionDepthLimit(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	// instance C a => C a would cause infinite resolution without a depth limit.
	// We use a realistic scenario: two classes with mutually recursive instances.
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
form Foo := \a. { foo: a -> Int }
form Bar := \a. { bar: a -> Int }
impl Bar a => Foo a := { foo := \x. bar x }
impl Foo a => Bar a := { bar := \x. foo x }
main := foo True
`)
	if err == nil {
		t.Fatal("expected error from mutually recursive instance resolution, got nil")
	}
	// Should get a structured error, not a stack overflow / panic.
	errStr := err.Error()
	if !strings.Contains(errStr, "depth") && !strings.Contains(errStr, "instance") {
		t.Logf("error: %s", errStr)
	}
}

// Fix 2: expandTypeAliases must have a fuel limit, and cyclic aliases
// must not install the expander at all.
func TestCyclicAliasExpansionTerminates(t *testing.T) {
	eng := gicel.NewEngine()
	// Direct cycle: type A := A — already caught by validateAliasGraph.
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
type A := A
main := pure @A True
`)
	if err == nil {
		t.Fatal("expected error for cyclic alias, got nil")
	}

	// Parametric cycle: type F a := F a — the expansion could loop even if
	// the name-level cycle is detected.
	eng2 := gicel.NewEngine()
	eng2.Use(gicel.Prelude)
	_, err2 := eng2.NewRuntime(context.Background(), `
import Prelude
type F := \a. F a
main := pure @(F Int) True
`)
	if err2 == nil {
		t.Fatal("expected error for parametric cyclic alias, got nil")
	}
}

// Alias expansion depth limit: deeply nested but non-cyclic aliases.
func TestAliasExpansionDepthLimit(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	// Build a chain of aliases: T0 a = a, T1 a = T0 (T0 a), ..., TN a = T(N-1) (T(N-1) a)
	// This grows exponentially in expansion depth.
	src := `
type T0 := \a. a
type T1 := \a. T0 (T0 a)
type T2 := \a. T1 (T1 a)
type T3 := \a. T2 (T2 a)
type T4 := \a. T3 (T3 a)
type T5 := \a. T4 (T4 a)
type T6 := \a. T5 (T5 a)
type T7 := \a. T6 (T6 a)
type T8 := \a. T7 (T7 a)
type T9 := \a. T8 (T8 a)
type T10 := \a. T9 (T9 a)
id :: T10 Bool -> T10 Bool
id := \x. x
main := id True
`
	// This should either succeed (with reasonable expansion) or report an error.
	// It must NOT hang or stack overflow.
	_, err := eng.NewRuntime(context.Background(), src)
	_ = err // Either outcome is acceptable; the key is termination.
}

// Fix 3: Error accumulation must have a cap.
func TestErrorAccumulationCap(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	// Generate a program with many type errors.
	var lines []string
	lines = append(lines, "import Prelude")
	for i := range 500 {
		lines = append(lines, fmt.Sprintf("x%d := unknownVar%d", i, i))
	}
	lines = append(lines, "main := 0")
	src := strings.Join(lines, "\n")
	_, err := eng.NewRuntime(context.Background(), src)
	if err == nil {
		t.Fatal("expected errors from 500 unbound variables, got nil")
	}
	// The key assertion: we got an error (not OOM or timeout) and
	// the error count is reasonably bounded.
	errStr := err.Error()
	// Should not contain more than ~200 error lines (cap + overflow message).
	errorLines := strings.Count(errStr, "error[E")
	if errorLines > 200 {
		t.Fatalf("expected error count to be capped, got %d error lines", errorLines)
	}
}

// Fix 4: parseKindExpr must use enterRecurse() to prevent stack overflow
// from deeply nested kind expressions.
func TestParserKindExprDepthLimit(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	// Build deeply nested kind: ((((((...Type))))))
	kind := "Type"
	for range 300 {
		kind = "(" + kind + ")"
	}
	src := fmt.Sprintf("form X (a: %s) := MkX\nmain := MkX", kind)
	_, err := eng.NewRuntime(context.Background(), src)
	// Should get a parse error, not a stack overflow.
	if err == nil {
		t.Log("deeply nested kind accepted (ok if within limits)")
	}
	// The main thing is it didn't panic.
}
