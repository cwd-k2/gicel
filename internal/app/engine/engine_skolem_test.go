package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// --- Phase 1: Skolem Infrastructure ---

func TestUnifySkolemRigid(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
not :: Bool -> Bool
not := \x. x
main := not True
`)
	if err != nil {
		t.Fatal("basic check should pass:", err)
	}
}

func TestUnifySkolemSame(t *testing.T) {
	// Indirect test: use a GADT constructor with existential that unifies skolem with itself
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
data SameTest := { MkSame :: \a. a -> a -> SameTest }
useIt :: SameTest -> Bool
useIt := \s. case s { MkSame x y -> True }
`)
	if err != nil {
		t.Fatal("existential pattern match should pass:", err)
	}
}

// --- Phase 1B: Skolem Escape Check ---

func TestSkolemEscapeDetected(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
data Exists := { MkExists :: \a. a -> Exists }
escape :: Exists -> Bool
escape := \e. case e { MkExists x -> x }
`)
	if err == nil {
		t.Fatal("expected escape error, got nil")
	}
	if !strings.Contains(err.Error(), "escape") && !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("expected escape/mismatch error, got: %s", err)
	}
}

func TestSkolemNoEscape(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
data Wrapper := { MkWrapper :: \a. a -> Wrapper }
safe :: Wrapper -> Bool
safe := \w. case w { MkWrapper _ -> True }
`)
	if err != nil {
		t.Fatal("non-escaping existential should pass:", err)
	}
}

func TestSkolemEscapeInMeta(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
data SomeVal := { MkSome :: \a. a -> SomeVal }
leaky := \s. case s { MkSome x -> Just x }
`)
	if err == nil {
		t.Fatal("expected escape error for Just x where x is existential")
	}
}

// --- Phase 2: Existential Types ---

func TestExistentialBasic(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data SomeEq := { MkSomeEq :: \a. Eq a => a -> SomeEq }
useSomeEq :: SomeEq -> Bool
useSomeEq := \s. case s { MkSomeEq x -> eq x x }
main := useSomeEq (MkSomeEq True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestExistentialEscapeError(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
data SomeEq := { MkSomeEq :: \a. Eq a => a -> SomeEq }
escape :: SomeEq -> Bool
escape := \s. case s { MkSomeEq x -> x }
`)
	if err == nil {
		t.Fatal("expected type error for escaping existential")
	}
}

func TestExistentialNoConstraint(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data ExistsF f := { MkExistsF :: \a. f a -> ExistsF f }
useMaybe :: ExistsF Maybe -> Bool
useMaybe := \e. case e { MkExistsF _ -> True }
main := useMaybe (MkExistsF (Just True))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestExistentialMixed(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data Wrapper a := { MkWrapper :: \b. (b -> a) -> b -> Wrapper a }
useWrapper :: Wrapper Bool -> Bool
useWrapper := \w. case w { MkWrapper f x -> f x }
main := useWrapper (MkWrapper (\x. x) True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestExistentialMultiConstraint(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
data ShowOrd := { MkShowOrd :: \a. (Eq a, Ord a) => a -> a -> ShowOrd }
use :: ShowOrd -> Ordering
use := \s. case s { MkShowOrd x y -> compare x y }
`)
	if err != nil {
		t.Fatal(err)
	}
}

// --- Phase 2D: Existential Integration ---

func TestExistentialWithTypeClass(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data SomeEq := { MkSomeEq :: \a. Eq a => a -> SomeEq }
isSame :: SomeEq -> Bool
isSame := \s. case s { MkSomeEq x -> eq x x }
main := isSame (MkSomeEq (Just True))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// eq (Just True) (Just True) = True (same value)
	assertConName(t, result.Value, "True")
}

func TestExistentialWithGADT(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data Typed a := { MkBool :: Bool -> Typed Bool; MkUnit :: Typed () }
data SomeTyped := { MkSome :: \a. Typed a -> SomeTyped }
classify :: SomeTyped -> Bool
classify := \s. case s { MkSome t -> True }
main := classify (MkSome (MkBool True))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestExistentialNestedCase(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data Wrap := { MkWrap :: \a. Eq a => a -> Wrap }
bothSame :: Wrap -> Wrap -> Bool
bothSame := \w1 w2.
  case w1 { MkWrap x -> case w2 { MkWrap y -> True } }
main := bothSame (MkWrap True) (MkWrap False)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

// --- Phase 3: Higher-Rank Polymorphism ---

func TestSubsumptionInstantiate(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
id :: \a. a -> a
id := \x. x
useBool :: (Bool -> Bool) -> Bool
useBool := \f. f True
main := useBool id
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestHigherRankBasic(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
applyToTrue :: (\a. a -> a) -> Bool
applyToTrue := \f. f True
id :: \a. a -> a
id := \x. x
main := applyToTrue id
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestHigherRankAnnotationRequired(t *testing.T) {
	// Rank-2 types should require annotation on the parameter
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
apply :: (\a. a -> a) -> (Bool, ())
apply := \f. (f True, f ())
id :: \a. a -> a
id := \x. x
main := apply id
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*eval.RecordVal)
	if !ok || len(rv.Fields) != 2 {
		t.Errorf("expected tuple (Bool, ()), got %s", result.Value)
	}
}

// --- Phase 3F: Higher-Rank Integration ---

func TestHigherRankPolymorphicArg(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
runId :: (\a. a -> a) -> (Bool, ())
runId := \f. (f True, f ())
id :: \a. a -> a
id := \x. x
main := runId id
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*eval.RecordVal)
	if !ok || len(rv.Fields) != 2 {
		t.Errorf("expected tuple (Bool, ()), got %s", result.Value)
	}
	assertConName(t, rv.Fields["_1"], "True")
	unitField, ok := rv.Fields["_2"].(*eval.RecordVal)
	if !ok || len(unitField.Fields) != 0 {
		t.Errorf("expected () in _2, got %s", rv.Fields["_2"])
	}
}
