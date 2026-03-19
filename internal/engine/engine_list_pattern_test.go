package engine

// List pattern tests — [p1, p2, ...] pattern matching and list pretty printing.
// Does NOT cover: engine_literal_test.go (list literal expressions).

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/stdlib"
)

func TestListPatternExact(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

f := \xs. case xs { [x, y] -> x + y; _ -> 0 }

main := f [3, 4]
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := MustHost[int64](result.Value)
	if got != 7 {
		t.Errorf("expected 7, got %d", got)
	}
}

func TestListPatternEmpty(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

f := \xs. case xs { [] -> "empty"; _ -> "nonempty" }

main := f ([] :: List Int)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := MustHost[string](result.Value)
	if got != "empty" {
		t.Errorf("expected %q, got %q", "empty", got)
	}
}

func TestListPatternNested(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

head := \xs. case xs { Cons x _ -> Just x; Nil -> Nothing }

main := head [42, 99]
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
		t.Fatalf("expected Just, got %v", result.Value)
	}
}

func TestListPrettyPrint(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

main := [1, 2, 3]
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := eval.PrettyValue(result.Value)
	if got != "[1, 2, 3]" {
		t.Errorf("expected [1, 2, 3], got %s", got)
	}
}

func TestListPrettyPrintEmpty(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

main := ([] :: List Int)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := eval.PrettyValue(result.Value)
	if got != "[]" {
		t.Errorf("expected [], got %s", got)
	}
}

func TestListPatternInAtom(t *testing.T) {
	// List pattern used as a constructor argument position (atom).
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

f := \pair. case pair { (xs, n) -> case xs { [x] -> x + n; _ -> n } }

main := f ([10], 5)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := MustHost[int64](result.Value)
	if got != 15 {
		t.Errorf("expected 15, got %d", got)
	}
}
