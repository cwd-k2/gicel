// Computation tests — pure/thunk/force, do blocks, bind chains, case, constructors, effectful deferral.
// Does NOT cover: literals (engine_literal_test.go), host API (engine_host_api_test.go).

package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

func TestIdentity(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
id := \x. x
main := id True
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

func TestPure(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `main := pure ()`)
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

func TestAssumption(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.DeclareAssumption("getUnit", ArrowType(ConType("Bool"), ConType("Bool")))
	eng.RegisterPrim("getUnit", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return &eval.ConVal{Con: "True"}, capEnv, nil
	})
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
getUnit := assumption
main := getUnit True
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

func TestCaseExpression(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
not := \b. case b { True => False; False => True }
main := not True
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

func TestDoBlock(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `main := do { pure () }`)
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

func TestMultiParamLambda(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
konst := \x y. x
main := konst True False
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

func TestNestedCase(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
bothTrue := \x y. case x { True => case y { True => True; False => False }; False => False }
main := bothTrue True True
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

func TestBlockExpression(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	// Block binding: { x := True; x } desugars to (\x. x) True.
	// The body references the block binding variable.
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := { x := True; x }
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

func TestBlockMultipleBindings(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := { x := True; y := False; x }
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

func TestBindChain(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := do { x <- pure True; pure x }
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

func TestThunkForce(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `main := force (thunk (pure ()))`)
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

// Explicit bind syntax: bind comp (\x. body) elaborates to Core.Bind.
func TestExplicitBind(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := bind (pure True) (\x. pure x)
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

// pure/bind/thunk/force used standalone (not applied) must error.
func TestSpecialFormStandalone(t *testing.T) {
	// thunk and force are still special forms.
	forms := []string{"thunk", "force"}
	for _, name := range forms {
		eng := NewEngine()
		_, err := eng.NewRuntime(context.Background(), "main := "+name)
		if err == nil {
			t.Errorf("expected compile error for standalone %s", name)
		}
	}
	// pure and bind are first-class functions — standalone use is valid.
	for _, name := range []string{"pure", "bind"} {
		eng := NewEngine()
		_, err := eng.NewRuntime(context.Background(), "main := "+name)
		if err != nil {
			t.Errorf("standalone %s should compile, got: %v", name, err)
		}
	}
}

// Thunk/force round-trip in do block.
func TestThunkForceInDo(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := do {
  t := thunk (pure True);
  force t
}
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

func TestConstructorArgs(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := Just True
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
	if !ok || inner.Con != "True" {
		t.Errorf("expected True, got %s", con.Args[0])
	}
}

func TestPairConstructor(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := (True, False)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*eval.RecordVal)
	if !ok {
		t.Errorf("expected RecordVal (tuple), got %T: %s", result.Value, result.Value)
	}
	if len(rv.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(rv.Fields))
	}
}

// Empty program (no main) should produce a runtime error, not panic.
func TestNoMainEntry(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
helper := True
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for missing entry point 'main'")
	}
}

// Large case expression: exhaustiveness on 6-constructor ADT.
func TestExhaustiveLargeADT(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data Color := Red | Green | Blue | Yellow | Cyan | Magenta

f := \c. case c {
  Red -> True;
  Green -> False;
  Blue -> True;
  Yellow -> False;
  Cyan -> True;
  Magenta -> False
}

main := f Red
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

// Non-exhaustive on large ADT names missing constructors.
func TestNonExhaustiveLargeADT(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
data Color := Red | Green | Blue | Yellow | Cyan | Magenta

f := \c. case c {
  Red -> True;
  Blue -> True
}

main := f Red
`)
	if err == nil {
		t.Fatal("expected compile error for non-exhaustive case")
	}
	errStr := err.Error()
	for _, missing := range []string{"Green", "Yellow", "Cyan", "Magenta"} {
		if !strings.Contains(errStr, missing) {
			t.Errorf("expected missing constructor %s in error, got: %s", missing, errStr)
		}
	}
}

// _ <- in do blocks must work (was a parser bug: TokUnderscore != TokLower).
func TestDoWildcardBind(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := do {
  _ <- pure True;
  pure False
}
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

// CapEnv threading: mutations propagate through do-block bind chain.
func TestCapEnvThreading(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.RegisterPrim("inc", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		v, _ := capEnv.Get("n")
		n, _ := v.(int)
		return ToValue(nil), capEnv.Set("n", n+1), nil
	})
	eng.RegisterPrim("getN", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		v, _ := capEnv.Get("n")
		n, _ := v.(int)
		return ToValue(n), capEnv, nil
	})
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
inc :: () -> Computation {} {} ()
inc := assumption
getN :: () -> Computation {} {} Int
getN := assumption
main := do { _ <- inc (); _ <- inc (); _ <- inc (); getN () }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: map[string]any{"n": 0}})
	if err != nil {
		t.Fatal(err)
	}
	n := MustHost[int](result.Value)
	if n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
	finalN, _ := result.CapEnv.Get("n")
	if finalN != 3 {
		t.Errorf("expected final capenv n=3, got %v", finalN)
	}
}

func TestEffectfulDeferUntilBind(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	if err := eng.Use(stdlib.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Effect.State
main :: Computation { state: Int | r } { state: Int | r } Int
main := do { s <- get; pure s }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &eval.HostVal{Inner: int64(42)}}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != int64(42) {
		t.Errorf("expected 42, got %v", result.Value)
	}
}

func TestEffectfulTopLevelForce(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	if err := eng.Use(stdlib.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Effect.State
main :: Computation { state: Int | r } { state: Int | r } Int
main := get
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &eval.HostVal{Inner: int64(7)}}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != int64(7) {
		t.Errorf("expected 7, got %v", result.Value)
	}
}

func TestNonEffectfulImmediate(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := 1 + 2
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != int64(3) {
		t.Errorf("expected 3, got %v", result.Value)
	}
}
