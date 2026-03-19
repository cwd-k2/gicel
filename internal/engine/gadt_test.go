package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/stdlib"
)

// ---------------------------------------------------------------------------
// DK. DataKinds — end-to-end integration
// ---------------------------------------------------------------------------

func TestDataKindsDBState(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
data DBState := Opened | Closed
data DB s := MkDB

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
data DBState := Opened | Closed

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
data Proxy s := MkProxy
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
data Expr a := { LitBool :: Bool -> Expr Bool; Not :: Expr Bool -> Expr Bool }

eval :: Expr Bool -> Bool
eval := fix (\self e. case e {
  LitBool b -> b;
  Not inner -> case self inner { True -> False; False -> True }
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
data DBState := Opened | Closed
data DB s := MkDB

data Action s := { Open :: Action Opened; Close :: Action Closed }

describe :: Action Opened -> DB Opened
describe := \a. case a { Open -> MkDB }

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
data Expr a := { LitBool :: Bool -> Expr Bool; Not :: Expr Bool -> Expr Bool }

-- Nested pattern: match on Not (LitBool _)
isDoubleNeg :: Expr Bool -> Bool
isDoubleNeg := \e. case e {
  Not inner -> case inner {
    Not _ -> True;
    LitBool _ -> False
  };
  LitBool _ -> False
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
