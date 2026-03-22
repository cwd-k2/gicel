package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// ---------------------------------------------------------------------------
// TC. Type classes — end-to-end
// ---------------------------------------------------------------------------

func TestTypeClassEqBool(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
data Bool := { True: Bool; False: Bool; }
data Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := {
  eq := \x y. case x {
    True  => case y { True => True;  False => False };
    False => case y { True => False; False => True }
  }
}
main := eq True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("expected False, got %s", result.Value)
	}
}

func TestTypeClassPolymorphic(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
data Bool := { True: Bool; False: Bool; }
data Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := {
  eq := \x y. case x {
    True  => case y { True => True;  False => False };
    False => case y { True => False; False => True }
  }
}
f :: \a. Eq a => a -> a -> Bool
f := \x y. eq x y
main := f True True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestTypeClassSuperclass(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
data Bool := { True: Bool; False: Bool; }
data Eq := \a. { eq: a -> a -> Bool }
data Ord := \a. Eq a => { lt: a -> a -> Bool }
impl Eq Bool := {
  eq := \x y. True
}
impl Ord Bool := {
  lt := \x y. False
}
useOrd :: \a. Ord a => a -> a -> Bool
useOrd := \x y. eq x y
main := useOrd True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestTypeClassFunctor(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
data Bool := { True: Bool; False: Bool; }
data Maybe a := Just a | Nothing
data Functor := \f. { fmap: \a b. (a -> b) -> f a -> f b }
impl Functor Maybe := {
  fmap := \g mx. case mx {
    Just x  => Just (g x);
    Nothing => Nothing
  }
}
not := \b. case b { True => False; False => True }
main := fmap not (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Just" {
		t.Errorf("expected Just, got %s", result.Value)
	}
	if len(con.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(con.Args))
	}
	inner, ok := con.Args[0].(*eval.ConVal)
	if !ok || inner.Con != "False" {
		t.Errorf("expected False inside Just, got %s", con.Args[0])
	}
}

func TestTypeClassMultiParam(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
data Bool := { True: Bool; False: Bool; }
data Coercible := \a b. { coerce: a -> b }
impl Coercible Bool Bool := {
  coerce := \x. x
}
main := coerce True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// --- Stdlib integration tests ---

func TestStdlibEqOrd(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := eq True True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestStdlibFunctor(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
not :: Bool -> Bool
not := \b. case b { True => False; False => True }

main := fmap not (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %s", result.Value)
	}
	inner, ok := con.Args[0].(*eval.ConVal)
	if !ok || inner.Con != "False" {
		t.Errorf("expected Just False, got Just %s", con.Args[0])
	}
}

func TestStdlibFoldable(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := foldr (\x _. x) False (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestStdlibEqPair(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := eq (True, False) (True, False)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// --- Coercible integration tests ---

func TestCoercibleMultiParam(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data Coercible := \a b. { coerce: a -> b }

impl Coercible Bool () := { coerce := \_. () }

main := coerce True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*eval.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

func TestCoercibleUsage(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data Coercible := \a b. { coerce: a -> b }

impl Coercible Bool Bool := { coerce := \x. x }

main: Bool
main := coerce True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// --- Phase 4: Stdlib Expansion ---

func TestClassSemigroup(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := append () ()
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*eval.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

func TestClassMonoid(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := (empty :: Ordering)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "EQ")
}

func TestClassApplicative(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
test := (wrap True :: Maybe Bool)
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClassTraversable(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
test :: \f a b. Applicative f => (a -> f b) -> Maybe a -> f (Maybe b)
test := traverse
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSemigroupUnit(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := append () ()
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*eval.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

func TestSemigroupOrdering(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
test1 := append LT EQ
test2 := append EQ GT
main := (test1, test2)
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
		t.Fatalf("expected tuple, got %s", result.Value)
	}
	assertConName(t, rv.Fields["_1"], "LT") // LT <> EQ = LT
	assertConName(t, rv.Fields["_2"], "GT") // EQ <> GT = GT
}

func TestMonoidUnit(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := (empty :: ())
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*eval.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

func TestApplicativeMaybe(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
wrapped := (wrap True :: Maybe Bool)
main := wrapped
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Just" {
		t.Errorf("expected Just, got %s", result.Value)
	}
}

func TestTraversableMaybe(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
not :: Bool -> Bool
not := \b. case b { True => False; False => True }
main := traverse (\x. Just (not x)) (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// traverse (\x. Just (not x)) (Just True) = Just (Just False)
	outer, ok := result.Value.(*eval.ConVal)
	if !ok || outer.Con != "Just" {
		t.Fatalf("expected Just, got %s", result.Value)
	}
	inner, ok := outer.Args[0].(*eval.ConVal)
	if !ok || inner.Con != "Just" {
		t.Fatalf("expected inner Just, got %s", outer.Args[0])
	}
	assertConName(t, inner.Args[0], "False")
}

func TestOrdBool(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := compare False True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "LT")
}

func TestOrdMaybe(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
test1 := compare (Nothing :: Maybe Bool) (Just True)
test2 := compare (Just False) (Just True)
main := (test1, test2)
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
		t.Fatalf("expected tuple, got %s", result.Value)
	}
	assertConName(t, rv.Fields["_1"], "LT") // Nothing < Just _
	assertConName(t, rv.Fields["_2"], "LT") // Just False < Just True
}

func TestOrdPair(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := compare (False, False) (True, True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// compare (False, False) (True, True) = LT (lexicographic)
	assertConName(t, result.Value, "LT")
}

// --- Phase 5: Cross-Feature Integration ---

func TestExistentialWithStdlib(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data SomeSemigroup := { MkSomeSG: \a. Semigroup a => a -> a -> SomeSemigroup }
combine :: SomeSemigroup -> Bool
combine := \s. case s { MkSomeSG x y -> case append x y { _ => True } }
main := combine (MkSomeSG EQ LT)
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

func TestLetGeneralizationConstrainedEq(t *testing.T) {
	// same := \x y. eq x y  (no annotation)
	// Should generalize to: \ a. Eq a => a -> a -> Bool
	// and work at both Int and Bool.
	eng := NewEngine()
	stdlib.Prelude(eng) // provides Eq Int
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
same := \x y. eq x y
main := (same 1 2, same True True)
`)
	if err != nil {
		t.Fatalf("constrained let-gen should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("constrained let-gen should run: %v", err)
	}
	// same 1 2 = False, same True True = True
	rv, ok := result.Value.(*eval.RecordVal)
	if !ok || len(rv.Fields) != 2 {
		t.Fatalf("expected (False, True), got %v", result.Value)
	}
}

func TestLetGeneralizationConstrainedOrd(t *testing.T) {
	// mymax := \x y. case compare x y { GT => x; _ => y }
	// Should generalize to: \ a. Ord a => a -> a -> a
	eng := NewEngine()
	stdlib.Prelude(eng)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
mymax := \x y. case compare x y { GT => x; _ => y }
main := (mymax 3 7, mymax True False)
`)
	if err != nil {
		t.Fatalf("constrained let-gen Ord should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("constrained let-gen Ord should run: %v", err)
	}
	rv, ok := result.Value.(*eval.RecordVal)
	if !ok || len(rv.Fields) != 2 {
		t.Fatalf("expected (7, True), got %v", result.Value)
	}
}

func TestStdlibClassHierarchy(t *testing.T) {
	// Verify all 8 classes compile and instances work
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
testEq := eq True True
testOrd := compare False True
testSemigroup := append EQ LT
testMonoid := (empty :: Ordering)
testFunctor := fmap (\x. True) (Just False)
testApplicative := (wrap True :: Maybe Bool)
main := (testEq, (testOrd, (testSemigroup, (testMonoid, (testFunctor, testApplicative)))))
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
		t.Fatalf("expected tuple, got %s", result.Value)
	}
	assertConName(t, rv.Fields["_1"], "True") // eq True True = True
}
