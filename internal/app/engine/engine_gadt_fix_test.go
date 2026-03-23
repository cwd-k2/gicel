package engine

// Polymorphic fix tests — GADT polymorphic recursion via fix desugaring.
// Does NOT cover: gadt_test.go (basic GADT, DataKinds).

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
)

func TestGADTPolymorphicFixRenderExpr(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

form Expr := \a. {
  IntLit: Int -> Expr Int;
  BoolLit: Bool -> Expr Bool;
  AddInt: Expr Int -> Expr Int -> Expr Int
}

renderExpr :: \a. Expr a -> String
renderExpr := fix (\self expr. case expr {
  IntLit n     => showInt n;
  BoolLit b    => case b { True => "t"; False => "f" };
  AddInt lhs rhs => self lhs <> "+" <> self rhs
})

main := renderExpr (AddInt (IntLit 1) (IntLit 2))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := MustHost[string](result.Value)
	if got != "1+2" {
		t.Errorf("expected %q, got %q", "1+2", got)
	}
}

func TestMonomorphicFixRegression(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

factorial :: Int -> Int
factorial := fix (\self n. case n == 0 { True => 1; False => n * self (n - 1) })

main := factorial 5
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := MustHost[int64](result.Value)
	if got != 120 {
		t.Errorf("expected 120, got %d", got)
	}
}

func TestFixNonLambdaFallback(t *testing.T) {
	// fix applied to a variable (not a lambda literal) should fallback
	// to the normal infer path and type-check correctly.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableRecursion()
	_, err := eng.NewRuntime(context.Background(), `
import Prelude

id :: \a. a -> a
id := \x. x

val :: Int -> Int
val := fix id

main := 0
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPolymorphicFixInferred(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableRecursion()
	// No type annotation — expected type is a metavar, should work via normal infer path.
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

factorial := fix (\self n. case n == 0 { True => 1; False => n * self (n - 1) })

main := factorial 5
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := MustHost[int64](result.Value)
	if got != 120 {
		t.Errorf("expected 120, got %d", got)
	}
}
