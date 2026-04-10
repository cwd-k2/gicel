package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// ---------------------------------------------------------------------------
// DK. DataKinds — end-to-end integration
// ---------------------------------------------------------------------------

func TestDataKindsDBState(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
form DBState := { Opened: DBState; Closed: DBState; }
form DB := \s. { MkDB: DB s; }

open :: DB Closed -> DB Opened
open := \_. MkDB

close :: DB Opened -> DB Closed
close := \_. MkDB

main := close (open (MkDB :: DB Closed))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "MkDB" {
		t.Errorf("expected MkDB, got %s", result.Value)
	}
}

func TestDataKindsInRow(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.RegisterPrim("readDB", func(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return ToValue(42), ce, nil
	})
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form DBState := { Opened: DBState; Closed: DBState; }

readDB :: () -> Computation { db: Int } { db: Int } Int
readDB := assumption

main :: Computation { db: Int } { db: Int } Int
main := do { readDB () }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: map[string]any{"db": 0}})
	if err != nil {
		t.Fatal(err)
	}
	v := MustHost[int](result.Value)
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestDataKindsBoolPromotion(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form Proxy := \s. { MkProxy: Proxy s; }
main := (MkProxy :: Proxy True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "MkProxy" {
		t.Errorf("expected MkProxy, got %s", result.Value)
	}
}

// --- GADT integration tests ---

func TestGADTEvalExpr(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form Expr := \a. { LitBool: Bool -> Expr Bool; Not: Expr Bool -> Expr Bool }

eval :: Expr Bool -> Bool
eval := fix (\self e. case e {
  LitBool b => b;
  Not inner => case self inner { True => False; False => True }
})

main := eval (Not (LitBool True))
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

func TestGADTWithDataKinds(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
form DBState := { Opened: DBState; Closed: DBState; }
form DB := \s. { MkDB: DB s; }

form Action := \s. { Open: Action Opened; Close: Action Closed }

describe :: Action Opened -> DB Opened
describe := \a. case a { Open => MkDB }

main := describe Open
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "MkDB" {
		t.Errorf("expected MkDB, got %s", result.Value)
	}
}

func TestGADTNestedPattern(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form Expr := \a. { LitBool: Bool -> Expr Bool; Not: Expr Bool -> Expr Bool }

-- Nested pattern: match on Not (LitBool _)
isDoubleNeg :: Expr Bool -> Bool
isDoubleNeg := \e. case e {
  Not inner => case inner {
    Not _ => True;
    LitBool _ => False
  };
  LitBool _ => False
}

main := isDoubleNeg (Not (Not (LitBool True)))
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

// ---------------------------------------------------------------------------
// Explicit equality constraints in GADT constructors
// ---------------------------------------------------------------------------

// TestGADTExplicitEqualityConstraint verifies that equality constraints
// written as explicit `a ~ b =>` in constructor types are installed as
// given equalities during pattern matching, enabling Refl-style proofs.
func TestGADTExplicitEqualityConstraint(t *testing.T) {
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)

	// castWith: the canonical use — match on MkRefl to coerce a to b.
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

form Refl := \a b. { MkRefl: a ~ b => Refl a b }

castWith :: Refl a b -> a -> b
castWith := \r x. case r { MkRefl => x }

main := castWith (MkRefl :: Refl Int Int) 42
`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := res.Value.(*eval.HostVal); !ok || v.Inner != int64(42) {
		t.Fatalf("castWith: expected 42, got %v", eval.PrettyValue(res.Value))
	}
}

func TestGADTReflSymmetry(t *testing.T) {
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)

	// symm: reverse an equality proof — requires both sides of the
	// equality to be available as givens.
	_, err := eng.NewRuntime(context.Background(), `
import Prelude

form Refl := \a b. { MkRefl: a ~ b => Refl a b }

symm :: Refl a b -> Refl b a
symm := \r. case r { MkRefl => MkRefl }

main := ()
`)
	if err != nil {
		t.Fatalf("symm should type-check: %v", err)
	}
}

func TestGADTEqualityWithClassConstraint(t *testing.T) {
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)

	// Equality constraint combined with a class constraint.
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

form EqWitness := \a b. { MkEqW: (Eq a, a ~ b) => EqWitness a b }

useWitness :: EqWitness a b -> a -> b -> Bool
useWitness := \w x y. case w { MkEqW => x == y }

main := useWitness (MkEqW :: EqWitness Int Int) 42 42
`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := res.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Fatalf("expected True, got %v", eval.PrettyValue(res.Value))
	}
}
