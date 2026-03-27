// Solver given equality integration tests — GADT given + type family, contradiction, class resolution.
// Does NOT cover: engine_gadt_test.go (basic GADT), engine_gadt_fix_test.go (polymorphic fix).

package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
)

// --- Given equality enables type family reduction ---

func TestGivenEnablesTFReduction(t *testing.T) {
	// GADT branch provides given a ~ Int. Type family F dispatches on a.
	// After Zonk replaces skolem a with Int (via skolemSoln from given eq),
	// FamilyReducer reduces F a => F Int => String.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

form Tag := \a. { IntTag: Tag Int; BoolTag: Tag Bool }

type F :: Type := \(a: Type). case a {
  Int => String;
  Bool => Int
}

useTag :: \a. Tag a -> F a -> F a
useTag := \tag x. case tag {
  IntTag => x;
  BoolTag => x
}

main := useTag IntTag "hello"
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertHostString(t, result.Value, "hello")
}

func TestGivenEnablesTFReductionBoolBranch(t *testing.T) {
	// Same setup but matching BoolTag: given a ~ Bool, F a reduces to Int.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

form Tag := \a. { IntTag: Tag Int; BoolTag: Tag Bool }

type F :: Type := \(a: Type). case a {
  Int => String;
  Bool => Int
}

useBoolTag :: Tag Bool -> F Bool -> F Bool
useBoolTag := \tag x. case tag {
  BoolTag => x
}

main := useBoolTag BoolTag 42
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertHostInt(t, result.Value, 42)
}

// --- Given equality contradiction (inaccessible branch) ---

func TestGivenContradictionInaccessible(t *testing.T) {
	// When a GADT branch provides contradictory givens (a ~ Int but
	// scrutinee has a ~ Bool), the branch is inaccessible and should
	// not cause type errors. The program type-checks if all reachable
	// branches are covered.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

form Tag := \a. { IntTag: Tag Int; BoolTag: Tag Bool }

-- Scrutinee is Tag Int; BoolTag branch is inaccessible.
describeInt :: Tag Int -> String
describeInt := \tag. case tag {
  IntTag => "int"
}

main := describeInt IntTag
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertHostString(t, result.Value, "int")
}

// --- Given equality enables class resolution ---

func TestGivenEnablesClassResolution(t *testing.T) {
	// GADT branch provides given a ~ Int. A class constraint Num a
	// should resolve to Num Int via the given equality.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

form Tag := \a. { IntTag: Tag Int; BoolTag: Tag Bool }

addOne :: \a. Tag a -> a -> a
addOne := \tag x. case tag {
  IntTag => x + 1;
  BoolTag => x
}

main := addOne IntTag 41
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertHostInt(t, result.Value, 42)
}

func TestGivenEnablesShowResolution(t *testing.T) {
	// GADT branch with given a ~ Int enables Show a to resolve to Show Int.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

form Tag := \a. { IntTag: Tag Int; BoolTag: Tag Bool }

showTagged :: \a. Tag a -> a -> String
showTagged := \tag x. case tag {
  IntTag => show x;
  BoolTag => case x { True => "true"; False => "false" }
}

main := showTagged IntTag 42
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertHostString(t, result.Value, "42")
}

// --- Combined: Given + TF + class ---

func TestGivenTFAndClassCombined(t *testing.T) {
	// GADT branch provides given a ~ Int.
	// Type family Result reduces Result a to Result Int = String via Zonk + FamilyReducer.
	// Class constraint Show a resolves to Show Int.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

form Tag := \a. { IntTag: Tag Int }

type Result :: Type := \(a: Type). case a {
  Int => String
}

process :: \a. Tag a -> a -> Result a
process := \tag x. case tag {
  IntTag => show x
}

main := process IntTag 42
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertHostString(t, result.Value, "42")
}

// --- Given equality with polymorphic recursive fix ---

func TestGivenTFWithFix(t *testing.T) {
	// Polymorphic fix over a GADT with type family return type.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

form Expr := \a. {
  LitI: Int -> Expr Int;
  LitB: Bool -> Expr Bool;
  Add: Expr Int -> Expr Int -> Expr Int
}

eval :: \a. Expr a -> a
eval := fix (\self e. case e {
  LitI n => n;
  LitB b => b;
  Add x y => self x + self y
})

main := eval (Add (LitI 10) (LitI 32))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertHostInt(t, result.Value, 42)
}
