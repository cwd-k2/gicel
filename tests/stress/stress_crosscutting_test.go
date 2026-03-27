// Cross-cutting stress tests — capability threading, type annotations, list literals, type aliases, DataKinds, computation types, then combinator.
// Does NOT cover: stress_parser_test.go, stress_checker_test.go, stress_evaluator_test.go, stress_correctness_test.go, stress_specialized_test.go, stress_limits_test.go.
package stress_test

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel"
)

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
