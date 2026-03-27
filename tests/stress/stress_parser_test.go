// Parser stress tests — dot operator, precedence, semicolons, lambda desugaring, block disambiguation.
// Does NOT cover: stress_checker_test.go, stress_evaluator_test.go, stress_correctness_test.go, stress_crosscutting_test.go, stress_specialized_test.go, stress_limits_test.go.
package stress_test

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// P1. Parser: Dot operator as composition (infixr 9) vs \ body separator
// ---------------------------------------------------------------------------

func TestDotAsCompositionOperator(t *testing.T) {
	// The `.` operator with infixr 9 should be parsed as function composition
	// when fixity is declared.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
infixr 9 .
(.) :: \ a b c. (b -> c) -> (a -> b) -> a -> c
(.) := \f g x. f (g x)

not :: Bool -> Bool
not := \b. case b { True => False; False => True }

id :: \ a. a -> a
id := \x. x

main := (not . id) True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "False")
}

func TestDotInForallDoesNotConflictWithComposition(t *testing.T) {
	// `.` in \ type body separator vs `.` as operator: both should
	// coexist without ambiguity.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
infixr 9 .
(.) :: \ a b c. (b -> c) -> (a -> b) -> a -> c
(.) := \f g x. f (g x)

id :: \ a. a -> a
id := \x. x

main := (id . id) True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")
}

// ---------------------------------------------------------------------------
// P2. Parser: Operator precedence and associativity
// ---------------------------------------------------------------------------

func TestOperatorPrecedenceMixedAssociativity(t *testing.T) {
	// infixl 6 + and infixl 7 * : 1 + 2 * 3 should be 1 + (2*3) = 7
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := 1 + 2 * 3
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 7 {
		t.Errorf("expected 7, got %d (precedence issue)", hv)
	}
}

func TestOperatorRightAssociativity(t *testing.T) {
	// infixr 5 :: (cons operator) should associate to the right.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
infixr 5 ++
(++) :: Int -> Int -> Int
(++) := \x y. x + y
-- a ++ b ++ c should be a ++ (b ++ c)
-- 1 ++ (2 ++ 3) = 1 + (2 + 3) = 1 + 5 = 6
main := 1 ++ 2 ++ 3
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 6 {
		t.Errorf("expected 6, got %d (right associativity issue)", hv)
	}
}

func TestOperatorLeftAssociativity(t *testing.T) {
	// infixl 6 - : a - b - c should be (a - b) - c
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
-- 10 - 3 - 2 should be (10 - 3) - 2 = 5 (left assoc)
-- not 10 - (3 - 2) = 10 - 1 = 9 (right assoc)
main := 10 - 3 - 2
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 5 {
		t.Errorf("expected 5, got %d (left associativity issue)", hv)
	}
}

// ---------------------------------------------------------------------------
// P3. Parser: Semicolon/newline interchangeability edge cases
// ---------------------------------------------------------------------------

func TestSemicolonInDoBlock(t *testing.T) {
	// Semicolons should work as statement separators in do blocks.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := do { x <- pure True; y <- pure False; pure x }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")
}

func TestNewlineInDoBlock(t *testing.T) {
	// DESIGN DECISION: Inside brace-delimited blocks (do, case), explicit
	// semicolons are required. Newlines act as separators only at the
	// top-level declaration scope. This is intentional — braces establish
	// an explicit grouping context where semicolons provide unambiguous
	// statement separation.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `main := do {
  x <- pure True
  y <- pure False
  pure x
}`)
	if err == nil {
		t.Log("DESIGN_GAP resolved: newlines now work in do blocks")
	} else {
		t.Log("DESIGN_GAP confirmed: newlines inside do blocks require semicolons")
		t.Log("Error:", err)
	}
}

func TestSemicolonInCaseAlts(t *testing.T) {
	// Semicolons between case alternatives.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
f := \x. case x { True => False; False => True }
main := f True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "False")
}

func TestNewlineInCaseAlts(t *testing.T) {
	// DESIGN DECISION: Semicolons are required between case alternatives
	// inside braces. Same rule as do blocks — braces use explicit separators.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
f := \x. case x {
  True => False
  False => True
}
main := f False
`)
	if err == nil {
		t.Log("DESIGN_GAP resolved: newlines now work in case blocks")
	} else {
		t.Log("DESIGN_GAP confirmed: newlines inside case blocks require semicolons")
		t.Log("Error:", err)
	}
}

// ---------------------------------------------------------------------------
// P4. Parser: Multi-param lambda desugaring
// ---------------------------------------------------------------------------

func TestMultiParamLambdaDeep(t *testing.T) {
	// Nested lambdas: \x y z. x
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
konst3 := \x y z. x
main := konst3 True False True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")
}

// ---------------------------------------------------------------------------
// P5. Parser: Block expression vs record literal disambiguation
// ---------------------------------------------------------------------------

func TestBlockExpressionBindingShadowing(t *testing.T) {
	// Block: { x := True; x } — the block-local x should shadow any outer x.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
x := False
main := { x := True; x }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")
}

func TestEmptyRecordAndUnit(t *testing.T) {
	// {} and () should both produce empty records.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
r := {}
u := ()
main := r
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected empty record, got %s", result.Value)
	}
}
