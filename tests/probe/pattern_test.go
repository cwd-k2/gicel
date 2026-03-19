//go:build probe

// Pattern matching edge-case tests.
package probe_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe C: Pattern matching edge cases
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
	_, err := eng.NewRuntime(context.Background(), `
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
	_, err := eng.NewRuntime(context.Background(), `
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
