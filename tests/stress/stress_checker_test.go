// Type checker stress tests — higher-rank polymorphism, row unification, evidence resolution, kind inference, occurs check, GADT refinement.
// Does NOT cover: stress_parser_test.go, stress_evaluator_test.go, stress_correctness_test.go, stress_crosscutting_test.go, stress_specialized_test.go, stress_limits_test.go.
package stress_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

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
