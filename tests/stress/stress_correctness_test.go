// Correctness stress tests — false negatives, false positives, error reporting quality.
// Does NOT cover: stress_parser_test.go, stress_checker_test.go, stress_evaluator_test.go, stress_crosscutting_test.go, stress_specialized_test.go, stress_limits_test.go.
package stress_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// F1. FALSE_NEGATIVE: Invalid programs that should be rejected
// ---------------------------------------------------------------------------

func TestFN_TypeAnnotationMismatch(t *testing.T) {
	// Value annotated as Int but defined as Bool should fail.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: Int
main := True
`)
	if err == nil {
		t.Fatal("expected type mismatch: Int vs Bool")
	}
}

func TestFN_ConstructorArityMismatch(t *testing.T) {
	// Just applied to too many arguments should fail.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
main := Just True False
`)
	if err == nil {
		t.Fatal("expected type error: Just takes 1 argument, not 2")
	}
}

func TestFN_RecordFieldTypeMismatch(t *testing.T) {
	// A function expecting { x: Int } given { x: Bool } should fail.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
f :: Record { x: Int } -> Int
f := \r. r.#x
main := f { x: True }
`)
	if err == nil {
		t.Fatal("expected type mismatch: field x is Bool, expected Int")
	}
}

func TestFN_EmptyCaseBody(t *testing.T) {
	// Empty case expression should produce an error.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
main := case True {}
`)
	if err == nil {
		t.Fatal("expected error for case with no alternatives")
	}
}

func TestFN_DoBlockEndingWithBind(t *testing.T) {
	// A do block ending with a bind (<-) is malformed.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
main := do { x <- pure True }
`)
	if err == nil {
		t.Fatal("expected error: do block must end with an expression")
	}
}

// ---------------------------------------------------------------------------
// F2. FALSE_POSITIVE: Valid programs that might be incorrectly rejected
// ---------------------------------------------------------------------------

func TestFP_PolymorphicFunctionApplication(t *testing.T) {
	// Polymorphic identity applied at different types should work.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
id :: \a. a -> a
id := \x. x

main := case id True { True => id (); False => id () }
`)
	if err != nil {
		t.Fatal("polymorphic function at multiple types should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

func TestFP_LambdaWithRecordPattern(t *testing.T) {
	// Record patterns in lambda position are desugared to case expressions.
	// Both case and direct lambda forms work.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	// Case destructure form:
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
getX := \r. case r { { x: v } => v }
main := getX { x: True }
`)
	if err != nil {
		t.Fatal("case destructure should succeed:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")

	// Direct lambda pattern form:
	eng2 := gicel.NewEngine()
	eng2.Use(gicel.Prelude)
	rt2, err := eng2.NewRuntime(context.Background(), `
import Prelude
getX := \{ x: v }. v
main := getX { x: True }
`)
	if err != nil {
		t.Fatal("record pattern in lambda should work:", err)
	}
	result2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result2.Value, "True")
}

func TestFP_WildcardInCasePattern(t *testing.T) {
	// Wildcard pattern in case should exhaust all remaining alternatives.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form Color := { Red: Color; Green: Color; Blue: Color; }
isRed := \c. case c { Red => True; _ => False }
main := isRed Blue
`)
	if err != nil {
		t.Fatal("wildcard should satisfy exhaustiveness:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "False")
}

func TestFP_NestedTuplePattern(t *testing.T) {
	// Nested tuple patterns in lambda position are desugared to case expressions.
	// Both case and direct lambda forms work.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	// Case destructure form:
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
f := \p. case p { (a, inner) => case inner { (b, c) => b } }
main := f (True, (False, True))
`)
	if err != nil {
		t.Fatal("case destructure should succeed:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "False")

	// Direct nested tuple pattern form:
	eng2 := gicel.NewEngine()
	eng2.Use(gicel.Prelude)
	rt2, err := eng2.NewRuntime(context.Background(), `
import Prelude
f := \(a, (b, c)). b
main := f (True, (False, True))
`)
	if err != nil {
		t.Fatal("nested tuple pattern in lambda should work:", err)
	}
	result2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result2.Value, "False")
}

// ---------------------------------------------------------------------------
// ERR1. Error reporting quality
// ---------------------------------------------------------------------------

func TestErrorReportsUnboundVarName(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.NewRuntime(context.Background(), `main := unknownVariable`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknownVariable") {
		t.Errorf("error should mention the unbound variable name, got: %s", err.Error())
	}
}

func TestErrorReportsNonExhaustiveConstructor(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
form Color := { Red: Color; Green: Color; Blue: Color; }
f := \c. case c { Red => True }
main := f Red
`)
	if err == nil {
		t.Fatal("expected non-exhaustive error")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "Green") || !strings.Contains(errStr, "Blue") {
		t.Errorf("error should mention missing constructors Green and Blue, got: %s", errStr)
	}
}

func TestErrorReportsTypeMismatchTypes(t *testing.T) {
	// The error message should include the expected and actual types.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
f :: Int -> Int
f := \x. x
main := f True
`)
	if err == nil {
		t.Fatal("expected type mismatch error")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "mismatch") && !strings.Contains(errStr, "Bool") && !strings.Contains(errStr, "Int") {
		// At least one indicator of type mismatch should be present
		if !strings.Contains(errStr, "type") {
			t.Errorf("error should indicate type mismatch, got: %s", errStr)
		}
	}
}
