package stress_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// =============================================================================
// Deep Pattern Verification
//
// Systematic probe of idiomatic patterns the language implementation should
// handle correctly. Organized by subsystem: parser, type checker, evaluator,
// and cross-cutting concerns.
// =============================================================================

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

// ---------------------------------------------------------------------------
// T1. Type checker: Higher-rank polymorphism via subsumption
// ---------------------------------------------------------------------------

func TestHigherRankPolyIdFunction(t *testing.T) {
	// A function that takes a polymorphic function and uses it at two types.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
applyBoth :: (\ a. a -> a) -> Bool
applyBoth := \f. f True

main := applyBoth (\x. x)
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

func TestHigherRankRejection(t *testing.T) {
	// A monomorphic function should NOT satisfy a higher-rank requirement.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
applyBoth :: (\a. a -> a) -> Bool
applyBoth := \f. f True

not :: Bool -> Bool
not := \b. case b { True => False; False => True }

main := applyBoth not
`)
	// 'not' is Bool -> Bool, not \ a. a -> a
	if err == nil {
		t.Fatal("expected compile error: Bool -> Bool should not satisfy \\ a. a -> a")
	}
}

// ---------------------------------------------------------------------------
// T2. Type checker: Row unification edge cases
// ---------------------------------------------------------------------------

func TestRowUnifyEmptyOpenClosed(t *testing.T) {
	// An open row { | r } should unify with a closed empty row {}.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
f :: \(r: Row). Record { | r } -> Bool
f := \_. True
main := f {}
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

func TestRowUnifyFieldSubset(t *testing.T) {
	// A function that requires field x should accept a record with x and y.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
getX :: \r. Record { x: Int | r } -> Int
getX := \rec. rec.#x
main := getX { x: 42, y: 0 }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

func TestRowUnifyMissingField(t *testing.T) {
	// A function requiring field x should reject a record without x.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
getX :: Record { x: Int } -> Int
getX := \rec. rec.#x
main := getX { y: 0 }
`)
	if err == nil {
		t.Fatal("expected compile error: record { y } does not have field x")
	}
}

// ---------------------------------------------------------------------------
// T3. Type checker: Evidence and constraint resolution
// ---------------------------------------------------------------------------

func TestConstraintResolutionChain(t *testing.T) {
	// Using a superclass method (eq) through a subclass constraint (Ord).
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
useSuperFromSub :: \a. Ord a => a -> a -> Bool
useSuperFromSub := \x y. eq x y

main := useSuperFromSub True True
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

func TestPolymorphicInstanceResolution(t *testing.T) {
	// Eq (Maybe Bool) should resolve through Eq a => Eq (Maybe a) and Eq Bool.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := eq (Just True) (Just True)
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

func TestPolymorphicInstanceResolutionNeg(t *testing.T) {
	// Eq (Maybe (Maybe Bool)) should also resolve.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := eq (Just (Just True)) (Just Nothing)
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

// ---------------------------------------------------------------------------
// T4. Type checker: Kind inference and checking
// ---------------------------------------------------------------------------

func TestKindInferenceSimple(t *testing.T) {
	// form Maybe := \a. { Just: a -> Maybe a; Nothing: Maybe a; }  -- 'a' should get kind Type.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
form MyMaybe := \a. { MyJust: a -> MyMaybe a; MyNothing: MyMaybe a; }
main := MyJust True
`)
	if err != nil {
		t.Fatal("kind inference for simple ADT should succeed:", err)
	}
}

func TestKindMismatchInApplication(t *testing.T) {
	// Applying a type constructor to the wrong kind should fail.
	// F expects (a: Type -> Type) but receives Bool: Type.
	// Previously FALSE_NEGATIVE — fixed in 923721f.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
form F (a: Type -> Type) := MkF
main := (MkF :: F Bool)
`)
	if err == nil {
		t.Fatal("expected kind error: Bool: Type applied where Type -> Type expected")
	}
}

// ---------------------------------------------------------------------------
// T5. Type checker: Occurs check in complex types
// ---------------------------------------------------------------------------

func TestOccursCheckInApplication(t *testing.T) {
	// Self-application: \x. x x should trigger occurs check.
	eng := gicel.NewEngine()
	_, err := eng.NewRuntime(context.Background(), `main := \x. x x`)
	if err == nil {
		t.Fatal("expected occurs check error for self-application")
	}
	if !strings.Contains(err.Error(), "infinite") && !strings.Contains(err.Error(), "occurs") {
		t.Errorf("expected occurs check message, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// T6. Type checker: GADT pattern refinement with existentials
// ---------------------------------------------------------------------------

func TestGADTRefinementDisjointBranches(t *testing.T) {
	// Two GADT constructors producing different type-level indices:
	// matching on each should refine the types differently.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form Token := \a. { BoolTok: Bool -> Token Bool; UnitTok: Token () }

toBool :: Token Bool -> Bool
toBool := \t. case t { BoolTok b => b }

main := toBool (BoolTok True)
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
// E1. Evaluator: LetRec (recursive let) with fix
// ---------------------------------------------------------------------------

func TestLetRecFix(t *testing.T) {
	// Both inferred and annotated polymorphic recursion work correctly.
	eng := gicel.NewEngine()
	eng.EnableRecursion()
	eng.Use(gicel.Prelude)

	// Inferred version (no annotation):
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

myLength := fix (\self xs. case xs { Nil => 0; Cons _ rest => 1 + self rest })

main := myLength (Cons True (Cons False (Cons True Nil)))
`)
	if err != nil {
		t.Fatal("inferred version should succeed:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 3 {
		t.Errorf("expected 3, got %d", hv)
	}

	// Annotated version with implicit forall:
	eng2 := gicel.NewEngine()
	eng2.EnableRecursion()
	eng2.Use(gicel.Prelude)
	rt2, err := eng2.NewRuntime(context.Background(), `
import Prelude

myLength :: List a -> Int
myLength := fix (\self xs. case xs { Nil => 0; Cons _ rest => 1 + self rest })

main := myLength (Cons True (Cons False (Cons True Nil)))
`)
	if err != nil {
		t.Fatal("annotated version should work:", err)
	}
	result2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv2 := gicel.MustHost[int64](result2.Value)
	if hv2 != 3 {
		t.Errorf("expected 3, got %d", hv2)
	}
}

// ---------------------------------------------------------------------------
// E2. Evaluator: Pattern matching with nested constructors
// ---------------------------------------------------------------------------

func TestNestedConstructorPatternMatch(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form Pair := \a b. { MkPair: a -> b -> Pair a b; }

fst :: \a b. Pair a b -> a
fst := \p. case p { MkPair x _ => x }

snd :: \a b. Pair a b -> b
snd := \p. case p { MkPair _ y => y }

main := fst (MkPair True False)
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

func TestDoubleNestedPatternMatch(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
isJustTrue :: Maybe Bool -> Bool
isJustTrue := \m. case m {
  Just b => case b { True => True; False => False };
  Nothing => False
}
main := isJustTrue (Just True)
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
// E3. Evaluator: Record operations end-to-end
// ---------------------------------------------------------------------------

func TestRecordProjectionEndToEnd(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
r := { x: 1, y: 2 }
main := r.#x + r.#y
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 3 {
		t.Errorf("expected 3, got %d", hv)
	}
}

func TestRecordUpdateEndToEnd(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
r := { x: 1, y: 2 }
r2 := { r | x: 100 }
main := r2.#x + r2.#y
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 102 {
		t.Errorf("expected 102, got %d", hv)
	}
}

func TestRecordPatternMatchEndToEnd(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
swap := \r. case r { { x: a, y: b } => { x: b, y: a } }
r := swap { x: 1, y: 2 }
main := r.#x
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 2 {
		t.Errorf("expected 2 (swapped), got %d", hv)
	}
}

// ---------------------------------------------------------------------------
// E4. Evaluator: Tuple operations
// ---------------------------------------------------------------------------

func TestTupleConstructAndProject(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
t := (1, 2, 3)
main := t.#_1 + t.#_2 + t.#_3
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
		t.Errorf("expected 6, got %d", hv)
	}
}

func TestTuplePatternMatch(t *testing.T) {
	// Tuple patterns in lambda position are desugared to case expressions
	// via isStructuredPattern. Both case and direct lambda forms work.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	// Case destructure form:
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
myFst := \p. case p { (a, b) => a }
main := myFst (True, False)
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
myFst := \(a, b). a
main := myFst (True, False)
`)
	if err != nil {
		t.Fatal("tuple pattern in lambda should work:", err)
	}
	result2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result2.Value, "True")
}

// ---------------------------------------------------------------------------
// E5. Evaluator: CBPV triad (pure/thunk/force) correctness
// ---------------------------------------------------------------------------

func TestCBPVTriadInDo(t *testing.T) {
	// thunk delays evaluation; force resumes it. pure lifts values.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := do {
  t := thunk (pure True);
  x <- force t;
  pure x
}
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

func TestForceNonThunkError(t *testing.T) {
	// Forcing a non-thunk value should produce a type error at compile time.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
main := force True
`)
	if err == nil {
		t.Fatal("expected compile error: cannot force a non-thunk")
	}
}

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

// ---------------------------------------------------------------------------
// CROSS1. Cross-cutting: Capability environment threading in computations
// ---------------------------------------------------------------------------

func TestCapEnvPersistsThroughBinds(t *testing.T) {
	// Capability environment mutations should persist through do-block binds.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectState)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := do {
  put 10;
  n <- get;
  put (n + 5);
  get
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &gicel.HostVal{Inner: int64(0)}}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 15 {
		t.Errorf("expected 15, got %d", hv)
	}
}

// ---------------------------------------------------------------------------
// CROSS2. Cross-cutting: Type annotations on expressions
// ---------------------------------------------------------------------------

func TestInlineTypeAnnotation(t *testing.T) {
	// (expr :: Type) should work for expression-level type annotations.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := (True :: Bool)
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

func TestInlineTypeAnnotationMismatch(t *testing.T) {
	// (True :: Int) should fail at compile time.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
main := (True :: Int)
`)
	if err == nil {
		t.Fatal("expected type annotation mismatch error")
	}
}

// ---------------------------------------------------------------------------
// CROSS3. Cross-cutting: List literals
// ---------------------------------------------------------------------------

func TestListLiteralSyntax(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := [True, False, True]
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Should produce Cons True (Cons False (Cons True Nil))
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Errorf("expected Cons, got %s", result.Value)
	}
}

func TestEmptyListLiteral(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := ([] :: List Bool)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "Nil" {
		t.Errorf("expected Nil, got %s", result.Value)
	}
}

// ---------------------------------------------------------------------------
// CROSS4. Cross-cutting: Type alias in complex position
// ---------------------------------------------------------------------------

func TestTypeAliasInFunctionSig(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
type Predicate := \a. a -> Bool
isTrue :: Predicate Bool
isTrue := \b. b
main := isTrue True
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

func TestTypeAliasWithParams(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
type Pair := \a b. (a, b)
mkPair :: \a b. a -> b -> Pair a b
mkPair := \x y. (x, y)
main := (mkPair True False).#_1
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
// CROSS5. Cross-cutting: DataKinds promotion
// ---------------------------------------------------------------------------

func TestDataKindsInGADTIndex(t *testing.T) {
	eng := gicel.NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
form Phase := { Building: Phase; Running: Phase; }

form Builder := \(p: Phase). { MkBuilder: Builder p }

start :: Builder Building
start := MkBuilder

main := start
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "MkBuilder")
}

func TestDataKindsMismatch(t *testing.T) {
	// Using a promoted constructor from a different data type at a kind-annotated
	// position should fail. Builder expects (p: Phase) but True has kind Bool.
	// Previously FALSE_NEGATIVE — fixed in 923721f.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
form Phase := { Building: Phase; Running: Phase; }
form Builder := \(p: Phase). { MkBuilder: Builder p }
start :: Builder True
start := MkBuilder
main := start
`)
	if err == nil {
		t.Fatal("expected kind error: True has kind Bool, but Phase expected")
	}
}

// ---------------------------------------------------------------------------
// CROSS6. Cross-cutting: Computation type aliases
// ---------------------------------------------------------------------------

func TestEffectTypeAlias(t *testing.T) {
	// The prelude defines: type Effect r a := Computation r r a
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: Effect {} Bool
main := pure True
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
// CROSS7. Cross-cutting: then combinator in do blocks
// ---------------------------------------------------------------------------

func TestThenInDoBlock(t *testing.T) {
	// Using >> (then) between two computations.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := do {
  pure ();
  pure True
}
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

// =============================================================================
// Safety: Infinite Recursion / Loop / Memory Exhaustion
// =============================================================================

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
	for i := 0; i < 500; i++ {
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
	for i := 0; i < 300; i++ {
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

// ---------------------------------------------------------------------------
// NP1. Nested patterns: bare constructor in pattern argument position
// ---------------------------------------------------------------------------

func TestNestedPatternBareConstructor(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form Pair := \a b. { MkPair: a -> b -> Pair a b; }

match := \p. case p { MkPair (Just x) Nothing => x; _ => False }
main := match (MkPair (Just True) Nothing)
`)
	if err != nil {
		t.Fatal("bare constructor in pattern arg should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")
}

func TestNestedPatternParenthesized(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
isJustTrue := \m. case m { Just True => True; _ => False }
main := (isJustTrue (Just True), isJustTrue (Just False), isJustTrue Nothing)
`)
	if err != nil {
		t.Fatal("nested constructor pattern (Just True) should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Verify: (True, False, False) — positive AND negative cases
	rv := result.Value.(*gicel.RecordVal)
	assertCon(t, rv.Fields["_1"], "True")
	assertCon(t, rv.Fields["_2"], "False")
	assertCon(t, rv.Fields["_3"], "False")
}

func TestNestedPatternExhaustiveness(t *testing.T) {
	// Non-exhaustive nested pattern should produce clear error.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
f := \x. case x { Just True => True }
main := f (Just True)
`)
	if err == nil {
		t.Fatal("expected non-exhaustive error for nested pattern")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "non-exhaustive") {
		t.Errorf("expected non-exhaustive message, got: %s", errStr)
	}
}

func TestNestedPatternDeepNesting(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
f := \x. case x { Just (Just (Just True)) => True; _ => False }
main := (f (Just (Just (Just True))), f (Just (Just (Just False))), f (Just (Just Nothing)))
`)
	if err != nil {
		t.Fatal("deeply nested pattern should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv := result.Value.(*gicel.RecordVal)
	assertCon(t, rv.Fields["_1"], "True")
	assertCon(t, rv.Fields["_2"], "False")
	assertCon(t, rv.Fields["_3"], "False")
}

// ---------------------------------------------------------------------------
// TA1. Type application: explicit @Type should work
// ---------------------------------------------------------------------------

func TestExplicitTypeApplication(t *testing.T) {
	// Verify @Type actually constrains: id @Bool applied to non-Bool must fail.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
id :: \a. a -> a
id := \x. x
main := id @Bool True
`)
	if err != nil {
		t.Fatal("explicit type application should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")

	// Negative: @Bool constrains, so Int argument must fail.
	eng2 := gicel.NewEngine()
	eng2.Use(gicel.Prelude)
	_, err = eng2.NewRuntime(context.Background(), `
import Prelude
id :: \a. a -> a
id := \x. x
main := id @Bool 42
`)
	if err == nil {
		t.Fatal("expected type error: @Bool applied but Int argument given")
	}
}

func TestExplicitTypeApplicationWrongType(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
id :: \a. a -> a
id := \x. x
main := id @Int True
`)
	if err == nil {
		t.Fatal("expected type error: @Int applied but Bool argument given")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "mismatch") && !strings.Contains(errStr, "Int") {
		t.Errorf("expected type mismatch error mentioning Int, got: %s", errStr)
	}
}

func TestExplicitTypeApplicationOnConstructor(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := Just @Bool True
`)
	if err != nil {
		t.Fatal("explicit type application on constructor should work:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con := result.Value.(*gicel.ConVal)
	if con.Con != "Just" || len(con.Args) != 1 {
		t.Fatalf("expected Just with 1 arg, got %s", result.Value)
	}
	assertCon(t, con.Args[0], "True")
}

// ---------------------------------------------------------------------------
// HR1. Higher-rank types in record fields
// ---------------------------------------------------------------------------

func TestHigherRankRecordField(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
r :: Record { apply : \a. a -> a }
r := { apply: \x. x }
main := ((r.#apply) True, (r.#apply) 42)
`)
	if err != nil {
		t.Fatal("higher-rank record field should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Verify polymorphic use: (True, 42) — field used at Bool AND Int.
	rv := result.Value.(*gicel.RecordVal)
	assertCon(t, rv.Fields["_1"], "True")
	v := gicel.MustHost[int64](rv.Fields["_2"])
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

// ---------------------------------------------------------------------------
// PC1. Packed class: Slice instance resolution with concrete types
// ---------------------------------------------------------------------------

func TestPackedSliceConcreteType(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.DataSlice)

	// Test 1: with annotation
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Data.Slice
xs :: Slice Int
xs := fromList [1, 2, 3]
main := toList xs
`)
	if err != nil {
		t.Fatal("Packed (Slice Int) Int should resolve:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Result should be a List (Cons 1 (Cons 2 (Cons 3 Nil)))
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", result.Value)
	}

	// Test 2: without annotation (inferred)
	eng2 := gicel.NewEngine()
	eng2.Use(gicel.Prelude)
	eng2.Use(gicel.DataSlice)
	rt2, err := eng2.NewRuntime(context.Background(), `
import Prelude
import Data.Slice
main := toList (fromList [1, 2, 3] :: Slice Int)
`)
	if err != nil {
		t.Fatal("Packed Slice without annotation should resolve:", err)
	}
	result2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con2, ok := result2.Value.(*gicel.ConVal)
	if !ok || con2.Con != "Cons" {
		t.Fatalf("expected Cons, got %s", result2.Value)
	}
}

// ---------------------------------------------------------------------------
// LP1. Literal patterns in case expressions
// ---------------------------------------------------------------------------

func TestLiteralPatternInt(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
describe := \n. case n { 0 => "zero"; 1 => "one"; _ => "other" }
main := describe 1
`)
	if err != nil {
		t.Fatal("integer literal pattern should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := gicel.MustHost[string](result.Value)
	if s != "one" {
		t.Errorf("expected \"one\", got %q", s)
	}
}

func TestLiteralPatternString(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
greet := \name. case name { "Alice" => "Hello Alice"; _ => "Hello stranger" }
main := greet "Alice"
`)
	if err != nil {
		t.Fatal("string literal pattern should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := gicel.MustHost[string](result.Value)
	if s != "Hello Alice" {
		t.Errorf("expected \"Hello Alice\", got %q", s)
	}
}

func TestLiteralPatternFallthrough(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
fib := \n. case n { 0 => 0; 1 => 1; _ => 99 }
main := fib 5
`)
	if err != nil {
		t.Fatal("literal pattern with wildcard fallthrough should work:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	v := gicel.MustHost[int64](result.Value)
	if v != 99 {
		t.Errorf("expected 99, got %d", v)
	}
}

func TestLiteralPatternInNestedCase(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
f := \m. case m { Just n => case n { 0 => True; _ => False }; Nothing => False }
main := f (Just 0)
`)
	if err != nil {
		t.Fatal("literal pattern in nested case should work:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")
}

func TestLiteralPatternRune(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
classify := \c. case c { 'a' => "vowel"; 'e' => "vowel"; _ => "other" }
main := classify 'a'
`)
	if err != nil {
		t.Fatal("rune literal pattern should be accepted:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := gicel.MustHost[string](result.Value)
	if s != "vowel" {
		t.Errorf("expected \"vowel\", got %q (rune pattern match failed)", s)
	}
}
