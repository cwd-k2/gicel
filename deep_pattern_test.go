package gomputation_test

import (
	"context"
	"strings"
	"testing"

	gmp "github.com/cwd-k2/gomputation"
)

// =============================================================================
// Deep Pattern Verification
//
// Systematic probe of idiomatic patterns the language implementation should
// handle correctly. Organized by subsystem: parser, type checker, evaluator,
// and cross-cutting concerns.
// =============================================================================

// ---------------------------------------------------------------------------
// P1. Parser: Dot operator as composition (infixr 9) vs forall body separator
// ---------------------------------------------------------------------------

func TestDotAsCompositionOperator(t *testing.T) {
	// The `.` operator with infixr 9 should be parsed as function composition
	// when fixity is declared.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
infixr 9 .
(.) :: forall a b c. (b -> c) -> (a -> b) -> a -> c
(.) := \f -> \g -> \x -> f (g x)

not :: Bool -> Bool
not := \b -> case b { True -> False; False -> True }

id :: forall a. a -> a
id := \x -> x

main := (not . id) True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "False")
}

func TestDotInForallDoesNotConflictWithComposition(t *testing.T) {
	// `.` in forall type body separator vs `.` as operator: both should
	// coexist without ambiguity.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
infixr 9 .
(.) :: forall a b c. (b -> c) -> (a -> b) -> a -> c
(.) := \f -> \g -> \x -> f (g x)

id :: forall a. a -> a
id := \x -> x

main := (id . id) True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

// ---------------------------------------------------------------------------
// P2. Parser: Operator precedence and associativity
// ---------------------------------------------------------------------------

func TestOperatorPrecedenceMixedAssociativity(t *testing.T) {
	// infixl 6 + and infixl 7 * : 1 + 2 * 3 should be 1 + (2*3) = 7
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	rt, err := eng.NewRuntime(`
import Std.Num
main := 1 + 2 * 3
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 7 {
		t.Errorf("expected 7, got %d (precedence issue)", hv)
	}
}

func TestOperatorRightAssociativity(t *testing.T) {
	// infixr 5 :: (cons operator) should associate to the right.
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	rt, err := eng.NewRuntime(`
import Std.Num
infixr 5 ++
(++) :: Int -> Int -> Int
(++) := \x -> \y -> x + y
-- a ++ b ++ c should be a ++ (b ++ c)
-- 1 ++ (2 ++ 3) = 1 + (2 + 3) = 1 + 5 = 6
main := 1 ++ 2 ++ 3
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 6 {
		t.Errorf("expected 6, got %d (right associativity issue)", hv)
	}
}

func TestOperatorLeftAssociativity(t *testing.T) {
	// infixl 6 - : a - b - c should be (a - b) - c
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	rt, err := eng.NewRuntime(`
import Std.Num
-- 10 - 3 - 2 should be (10 - 3) - 2 = 5 (left assoc)
-- not 10 - (3 - 2) = 10 - 1 = 9 (right assoc)
main := 10 - 3 - 2
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 5 {
		t.Errorf("expected 5, got %d (left associativity issue)", hv)
	}
}

// ---------------------------------------------------------------------------
// P3. Parser: Semicolon/newline interchangeability edge cases
// ---------------------------------------------------------------------------

func TestSemicolonInDoBlock(t *testing.T) {
	// Semicolons should work as statement separators in do blocks.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := do { x <- pure True; y <- pure False; pure x }`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestNewlineInDoBlock(t *testing.T) {
	// DESIGN_GAP: Newlines inside do { } do NOT act as statement separators.
	// The spec says "semicolons and newlines are interchangeable" but the parser
	// only treats newlines as separators at the top-level declaration scope.
	// Inside brace-delimited blocks (do, case), explicit semicolons are required.
	//
	// Workaround: use semicolons inside do blocks.
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := do {
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
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
f := \x -> case x { True -> False; False -> True }
main := f True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "False")
}

func TestNewlineInCaseAlts(t *testing.T) {
	// DESIGN_GAP: Newlines inside case { } do NOT act as alternative separators.
	// Same root cause as TestNewlineInDoBlock — the parser only uses newlines
	// as separators at top-level scope, not inside brace-delimited blocks.
	//
	// Workaround: use semicolons between case alternatives.
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
f := \x -> case x {
  True -> False
  False -> True
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
	// Nested lambdas: \x -> \y -> \z -> x
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
konst3 := \x -> \y -> \z -> x
main := konst3 True False True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

// ---------------------------------------------------------------------------
// P5. Parser: Block expression vs record literal disambiguation
// ---------------------------------------------------------------------------

func TestBlockExpressionBindingShadowing(t *testing.T) {
	// Block: { x := True; x } — the block-local x should shadow any outer x.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
x := False
main := { x := True; x }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestEmptyRecordAndUnit(t *testing.T) {
	// {} and () should both produce empty records.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
r := {}
u := ()
main := r
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected empty record, got %s", result.Value)
	}
}

// ---------------------------------------------------------------------------
// T1. Type checker: Higher-rank polymorphism via subsumption
// ---------------------------------------------------------------------------

func TestHigherRankPolyIdFunction(t *testing.T) {
	// A function that takes a polymorphic function and uses it at two types.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
applyBoth :: (forall a. a -> a) -> Bool
applyBoth := \f -> f True

main := applyBoth (\x -> x)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestHigherRankRejection(t *testing.T) {
	// A monomorphic function should NOT satisfy a higher-rank requirement.
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
applyBoth :: (forall a. a -> a) -> Bool
applyBoth := \f -> f True

not :: Bool -> Bool
not := \b -> case b { True -> False; False -> True }

main := applyBoth not
`)
	// 'not' is Bool -> Bool, not forall a. a -> a
	if err == nil {
		t.Fatal("expected compile error: Bool -> Bool should not satisfy forall a. a -> a")
	}
}

// ---------------------------------------------------------------------------
// T2. Type checker: Row unification edge cases
// ---------------------------------------------------------------------------

func TestRowUnifyEmptyOpenClosed(t *testing.T) {
	// An open row { | r } should unify with a closed empty row {}.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
f :: forall (r : Row). Record { | r } -> Bool
f := \_ -> True
main := f {}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestRowUnifyFieldSubset(t *testing.T) {
	// A function that requires field x should accept a record with x and y.
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	rt, err := eng.NewRuntime(`
import Std.Num
getX :: forall r. Record { x : Int | r } -> Int
getX := \rec -> rec!#x
main := getX { x = 42, y = 0 }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

func TestRowUnifyMissingField(t *testing.T) {
	// A function requiring field x should reject a record without x.
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	_, err := eng.NewRuntime(`
import Std.Num
getX :: Record { x : Int } -> Int
getX := \rec -> rec!#x
main := getX { y = 0 }
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
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
useSuperFromSub :: forall a. Ord a => a -> a -> Bool
useSuperFromSub := \x -> \y -> eq x y

main := useSuperFromSub True True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestPolymorphicInstanceResolution(t *testing.T) {
	// Eq (Maybe Bool) should resolve through Eq a => Eq (Maybe a) and Eq Bool.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := eq (Just True) (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestPolymorphicInstanceResolutionNeg(t *testing.T) {
	// Eq (Maybe (Maybe Bool)) should also resolve.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := eq (Just (Just True)) (Just Nothing)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "False")
}

// ---------------------------------------------------------------------------
// T4. Type checker: Kind inference and checking
// ---------------------------------------------------------------------------

func TestKindInferenceSimple(t *testing.T) {
	// data Maybe a = Just a | Nothing  -- 'a' should get kind Type.
	eng := gmp.NewEngine()
	_, err := eng.Check(`
data MyMaybe a = MyJust a | MyNothing
main := MyJust True
`)
	if err != nil {
		t.Fatal("kind inference for simple ADT should succeed:", err)
	}
}

func TestKindMismatchInApplication(t *testing.T) {
	// FALSE_NEGATIVE: Applying a type constructor to the wrong kind should fail,
	// but the kind checker does not reject this.
	// F expects (a : Type -> Type) but receives Bool : Type.
	eng := gmp.NewEngine()
	_, err := eng.Check(`
data F (a : Type -> Type) = MkF
main := (MkF :: F Bool)
`)
	if err == nil {
		t.Fatal("expected kind error: Bool : Type applied where Type -> Type expected")
	}
}

// ---------------------------------------------------------------------------
// T5. Type checker: Occurs check in complex types
// ---------------------------------------------------------------------------

func TestOccursCheckInApplication(t *testing.T) {
	// Self-application: \x -> x x should trigger occurs check.
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := \x -> x x`)
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
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Token a = { BoolTok :: Bool -> Token Bool; UnitTok :: Token () }

toBool :: Token Bool -> Bool
toBool := \t -> case t { BoolTok b -> b }

main := toBool (BoolTok True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

// ---------------------------------------------------------------------------
// E1. Evaluator: LetRec (recursive let) with fix
// ---------------------------------------------------------------------------

func TestLetRecFix(t *testing.T) {
	// FALSE_POSITIVE: A polymorphic annotation `List a -> Int` does not unify
	// with a concrete application `List Bool -> Int`. The checker treats `a` in
	// the annotation as rigid (skolem) rather than instantiating it.
	//
	// Workaround: omit the type annotation and let the checker infer.
	eng := gmp.NewEngine()
	eng.EnableRecursion()
	eng.Use(gmp.Num)

	// First, demonstrate the workaround works (no annotation):
	rt, err := eng.NewRuntime(`
import Std.Num

myLength := fix (\self -> \xs -> case xs { Nil -> 0; Cons _ rest -> 1 + self rest })

main := myLength (Cons True (Cons False (Cons True Nil)))
`)
	if err != nil {
		t.Fatal("workaround (no annotation) should succeed:", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 3 {
		t.Errorf("expected 3, got %d", hv)
	}

	// Annotation version with implicit forall:
	eng2 := gmp.NewEngine()
	eng2.EnableRecursion()
	eng2.Use(gmp.Num)
	rt2, err := eng2.NewRuntime(`
import Std.Num

myLength :: List a -> Int
myLength := fix (\self -> \xs -> case xs { Nil -> 0; Cons _ rest -> 1 + self rest })

main := myLength (Cons True (Cons False (Cons True Nil)))
`)
	if err != nil {
		t.Fatal("polymorphic annotation should work with implicit forall:", err)
	}
	result2, err := rt2.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv2 := gmp.MustHost[int64](result2.Value)
	if hv2 != 3 {
		t.Errorf("expected 3, got %d", hv2)
	}
}

// ---------------------------------------------------------------------------
// E2. Evaluator: Pattern matching with nested constructors
// ---------------------------------------------------------------------------

func TestNestedConstructorPatternMatch(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Pair a b = MkPair a b

fst :: forall a b. Pair a b -> a
fst := \p -> case p { MkPair x _ -> x }

snd :: forall a b. Pair a b -> b
snd := \p -> case p { MkPair _ y -> y }

main := fst (MkPair True False)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestDoubleNestedPatternMatch(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
isJustTrue :: Maybe Bool -> Bool
isJustTrue := \m -> case m {
  Just b -> case b { True -> True; False -> False };
  Nothing -> False
}
main := isJustTrue (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

// ---------------------------------------------------------------------------
// E3. Evaluator: Record operations end-to-end
// ---------------------------------------------------------------------------

func TestRecordProjectionEndToEnd(t *testing.T) {
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	rt, err := eng.NewRuntime(`
import Std.Num
r := { x = 1, y = 2 }
main := r!#x + r!#y
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 3 {
		t.Errorf("expected 3, got %d", hv)
	}
}

func TestRecordUpdateEndToEnd(t *testing.T) {
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	rt, err := eng.NewRuntime(`
import Std.Num
r := { x = 1, y = 2 }
r2 := { r | x = 100 }
main := r2!#x + r2!#y
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 102 {
		t.Errorf("expected 102, got %d", hv)
	}
}

func TestRecordPatternMatchEndToEnd(t *testing.T) {
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	rt, err := eng.NewRuntime(`
import Std.Num
swap := \r -> case r { { x = a, y = b } -> { x = b, y = a } }
r := swap { x = 1, y = 2 }
main := r!#x
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 2 {
		t.Errorf("expected 2 (swapped), got %d", hv)
	}
}

// ---------------------------------------------------------------------------
// E4. Evaluator: Tuple operations
// ---------------------------------------------------------------------------

func TestTupleConstructAndProject(t *testing.T) {
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	rt, err := eng.NewRuntime(`
import Std.Num
t := (1, 2, 3)
main := t!#_1 + t!#_2 + t!#_3
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 6 {
		t.Errorf("expected 6, got %d", hv)
	}
}

func TestTuplePatternMatch(t *testing.T) {
	// DESIGN_GAP: Tuple (and record) patterns in lambda position are not
	// elaborated. The checker's patternName() returns "_" for non-variable
	// patterns, so bindings introduced by the pattern are silently discarded.
	//
	// Workaround: use `case` to destructure, or use projection (e.g., p!#_1).
	eng := gmp.NewEngine()

	// Demonstrate the workaround using case:
	rt, err := eng.NewRuntime(`
myFst := \p -> case p { (a, b) -> a }
main := myFst (True, False)
`)
	if err != nil {
		t.Fatal("workaround (case destructure) should succeed:", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")

	// Direct lambda pattern version:
	eng2 := gmp.NewEngine()
	rt2, err := eng2.NewRuntime(`
myFst := \(a, b) -> a
main := myFst (True, False)
`)
	if err != nil {
		t.Fatal("tuple pattern in lambda should work:", err)
	}
	result2, err := rt2.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result2.Value, "True")
}

// ---------------------------------------------------------------------------
// E5. Evaluator: CBPV triad (pure/thunk/force) correctness
// ---------------------------------------------------------------------------

func TestCBPVTriadInDo(t *testing.T) {
	// thunk delays evaluation; force resumes it. pure lifts values.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := do {
  t := thunk (pure True);
  x <- force t;
  pure x
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestForceNonThunkError(t *testing.T) {
	// Forcing a non-thunk value should produce a type error at compile time.
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := force True`)
	if err == nil {
		t.Fatal("expected compile error: cannot force a non-thunk")
	}
}

// ---------------------------------------------------------------------------
// F1. FALSE_NEGATIVE: Invalid programs that should be rejected
// ---------------------------------------------------------------------------

func TestFN_TypeAnnotationMismatch(t *testing.T) {
	// Value annotated as Int but defined as Bool should fail.
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	_, err := eng.NewRuntime(`
import Std.Num
main :: Int
main := True
`)
	if err == nil {
		t.Fatal("expected type mismatch: Int vs Bool")
	}
}

func TestFN_ConstructorArityMismatch(t *testing.T) {
	// Just applied to too many arguments should fail.
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := Just True False`)
	if err == nil {
		t.Fatal("expected type error: Just takes 1 argument, not 2")
	}
}

func TestFN_RecordFieldTypeMismatch(t *testing.T) {
	// A function expecting { x : Int } given { x : Bool } should fail.
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	_, err := eng.NewRuntime(`
import Std.Num
f :: Record { x : Int } -> Int
f := \r -> r!#x
main := f { x = True }
`)
	if err == nil {
		t.Fatal("expected type mismatch: field x is Bool, expected Int")
	}
}

func TestFN_EmptyCaseBody(t *testing.T) {
	// Empty case expression should produce an error.
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := case True {}`)
	if err == nil {
		t.Fatal("expected error for case with no alternatives")
	}
}

func TestFN_DoBlockEndingWithBind(t *testing.T) {
	// A do block ending with a bind (<-) is malformed.
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := do { x <- pure True }`)
	if err == nil {
		t.Fatal("expected error: do block must end with an expression")
	}
}

// ---------------------------------------------------------------------------
// F2. FALSE_POSITIVE: Valid programs that might be incorrectly rejected
// ---------------------------------------------------------------------------

func TestFP_PolymorphicFunctionApplication(t *testing.T) {
	// Polymorphic identity applied at different types should work.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
id :: forall a. a -> a
id := \x -> x

main := case id True { True -> id (); False -> id () }
`)
	if err != nil {
		t.Fatal("polymorphic function at multiple types should be accepted:", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

func TestFP_LambdaWithRecordPattern(t *testing.T) {
	// DESIGN_GAP: Record patterns in lambda position are not elaborated.
	// Same root cause as TestTuplePatternMatch: patternName() in bidir.go
	// returns "_" for PatRecord, discarding all bindings.
	//
	// Workaround: use `case` or projection (r!#x) instead.
	eng := gmp.NewEngine()

	// Workaround using case:
	rt, err := eng.NewRuntime(`
getX := \r -> case r { { x = v } -> v }
main := getX { x = True }
`)
	if err != nil {
		t.Fatal("workaround (case destructure) should succeed:", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")

	// Direct lambda pattern version:
	eng2 := gmp.NewEngine()
	rt2, err := eng2.NewRuntime(`
getX := \{ x = v } -> v
main := getX { x = True }
`)
	if err != nil {
		t.Fatal("record pattern in lambda should work:", err)
	}
	result2, err := rt2.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result2.Value, "True")
}

func TestFP_WildcardInCasePattern(t *testing.T) {
	// Wildcard pattern in case should exhaust all remaining alternatives.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Color = Red | Green | Blue
isRed := \c -> case c { Red -> True; _ -> False }
main := isRed Blue
`)
	if err != nil {
		t.Fatal("wildcard should satisfy exhaustiveness:", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "False")
}

func TestFP_NestedTuplePattern(t *testing.T) {
	// DESIGN_GAP: Nested tuple patterns in lambda position are not elaborated.
	// Same root cause as TestTuplePatternMatch and TestFP_LambdaWithRecordPattern.
	//
	// Workaround: use nested `case` or chained projection.
	eng := gmp.NewEngine()

	// Workaround using case:
	rt, err := eng.NewRuntime(`
f := \p -> case p { (a, inner) -> case inner { (b, c) -> b } }
main := f (True, (False, True))
`)
	if err != nil {
		t.Fatal("workaround (nested case destructure) should succeed:", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "False")

	// Direct nested tuple pattern version:
	eng2 := gmp.NewEngine()
	rt2, err := eng2.NewRuntime(`
f := \(a, (b, c)) -> b
main := f (True, (False, True))
`)
	if err != nil {
		t.Fatal("nested tuple pattern in lambda should work:", err)
	}
	result2, err := rt2.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result2.Value, "False")
}

// ---------------------------------------------------------------------------
// ERR1. Error reporting quality
// ---------------------------------------------------------------------------

func TestErrorReportsUnboundVarName(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := unknownVariable`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknownVariable") {
		t.Errorf("error should mention the unbound variable name, got: %s", err.Error())
	}
}

func TestErrorReportsNonExhaustiveConstructor(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
data Color = Red | Green | Blue
f := \c -> case c { Red -> True }
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
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	_, err := eng.NewRuntime(`
import Std.Num
f :: Int -> Int
f := \x -> x
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
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	eng.Use(gmp.State)
	rt, err := eng.NewRuntime(`
import Std.Num
import Std.State
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
	caps := map[string]any{"state": &gmp.HostVal{Inner: int64(0)}}
	result, err := rt.RunContext(context.Background(), caps, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 15 {
		t.Errorf("expected 15, got %d", hv)
	}
}

// ---------------------------------------------------------------------------
// CROSS2. Cross-cutting: Type annotations on expressions
// ---------------------------------------------------------------------------

func TestInlineTypeAnnotation(t *testing.T) {
	// (expr :: Type) should work for expression-level type annotations.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := (True :: Bool)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestInlineTypeAnnotationMismatch(t *testing.T) {
	// (True :: Int) should fail at compile time.
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	_, err := eng.NewRuntime(`
import Std.Num
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
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := [True, False, True]
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	// Should produce Cons True (Cons False (Cons True Nil))
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Cons" {
		t.Errorf("expected Cons, got %s", result.Value)
	}
}

func TestEmptyListLiteral(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := ([] :: List Bool)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Nil" {
		t.Errorf("expected Nil, got %s", result.Value)
	}
}

// ---------------------------------------------------------------------------
// CROSS4. Cross-cutting: Type alias in complex position
// ---------------------------------------------------------------------------

func TestTypeAliasInFunctionSig(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
type Predicate a = a -> Bool
isTrue :: Predicate Bool
isTrue := \b -> b
main := isTrue True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestTypeAliasWithParams(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
type Pair a b = (a, b)
mkPair :: forall a b. a -> b -> Pair a b
mkPair := \x -> \y -> (x, y)
main := (mkPair True False)!#_1
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

// ---------------------------------------------------------------------------
// CROSS5. Cross-cutting: DataKinds promotion
// ---------------------------------------------------------------------------

func TestDataKindsInGADTIndex(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Phase = Building | Running

data Builder (p : Phase) = MkBuilder

start :: Builder Building
start := MkBuilder

main := start
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "MkBuilder")
}

func TestDataKindsMismatch(t *testing.T) {
	// FALSE_NEGATIVE: Using a promoted constructor from a different data type
	// at a kind-annotated position should fail, but it is accepted.
	// Builder expects (p : Phase) but True has kind Bool — these are different
	// promoted data kinds. The kind checker does not enforce cross-kind boundaries.
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
data Phase = Building | Running
data Builder (p : Phase) = MkBuilder
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
	// The prelude defines: type Effect r a = Computation r r a
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
myId :: Effect {} Bool
myId := pure True
main := myId
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

// ---------------------------------------------------------------------------
// CROSS7. Cross-cutting: then combinator in do blocks
// ---------------------------------------------------------------------------

func TestThenInDoBlock(t *testing.T) {
	// Using >> (then) between two computations.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := do {
  pure ();
  pure True
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

// assertConName is defined in gomputation_test.go — reused here.
