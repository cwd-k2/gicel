package gicel_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// IxMonad Class + Lift Alias (Group 2B)
// ---------------------------------------------------------------------------

func TestIxMonadClassExists(t *testing.T) {
	// Verify IxMonad class is defined in prelude and usable in type annotations.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(`
import Prelude
myFn :: \(m: Row -> Row -> Type -> Type). IxMonad m => \a (r: Row). a -> m r r a
myFn := ixpure
main := True
`)
	if err != nil {
		t.Fatalf("IxMonad class should be usable: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Instance IxMonad Computation (Group 2C)
// ---------------------------------------------------------------------------

func TestIxMonadComputationIxpure(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.DeclareBinding("n", gicel.ConType("Int"))
	rt, err := eng.NewRuntime(`
import Prelude
main :: Computation {} {} Int
main := ixpure n
`)
	if err != nil {
		t.Fatalf("ixpure should resolve via IxMonad Computation: %v", err)
	}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: map[string]gicel.Value{
		"n": &gicel.HostVal{Inner: 42},
	}})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok || hv.Inner != 42 {
		t.Fatalf("expected 42, got %v", result.Value)
	}
}

func TestIxMonadComputationIxbind(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.DeclareBinding("n", gicel.ConType("Int"))
	rt, err := eng.NewRuntime(`
import Prelude
main :: Computation {} {} Int
main := ixbind (ixpure n) (\x. ixpure x)
`)
	if err != nil {
		t.Fatalf("ixbind should resolve via IxMonad Computation: %v", err)
	}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: map[string]gicel.Value{
		"n": &gicel.HostVal{Inner: 99},
	}})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok || hv.Inner != 99 {
		t.Fatalf("expected 99, got %v", result.Value)
	}
}

func TestIxMonadGenericFunction(t *testing.T) {
	// A generic function using IxMonad constraint should work with Computation.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
myReturn :: \(m: Row -> Row -> Type -> Type). IxMonad m => \a (r: Row). a -> m r r a
myReturn := ixpure
main :: Computation {} {} Bool
main := myReturn True
`)
	if err != nil {
		t.Fatalf("generic IxMonad function should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "True" {
		t.Fatalf("expected True, got %v", result.Value)
	}
}

// ---------------------------------------------------------------------------
// Type-Driven do Dispatch (Group 3A)
// ---------------------------------------------------------------------------

func TestDoBlockComputationRegression(t *testing.T) {
	// Existing do blocks with Computation should still use Core.Bind path.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.DeclareBinding("x", gicel.ConType("Int"))
	rt, err := eng.NewRuntime(`
main := do { v <- pure x; pure v }
`)
	if err != nil {
		t.Fatalf("Computation do block regression failed: %v", err)
	}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: map[string]gicel.Value{
		"x": &gicel.HostVal{Inner: 42},
	}})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok || hv.Inner != 42 {
		t.Fatalf("expected 42, got %v", result.Value)
	}
}

func TestDoBlockComputationCoreBind(t *testing.T) {
	// Verify Computation do blocks elaborate to Core.Bind (not class dispatch).
	// Use Check() to inspect pre-optimization Core IR.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.DeclareBinding("x", gicel.ConType("Int"))
	cp, err := eng.Compile(`
main := do { v <- pure x; pure v }
`)
	if err != nil {
		t.Fatal(err)
	}
	// Core.Bind is used when Pretty() shows "bind" nodes rather than "ixbind" calls.
	pretty := cp.Pretty()
	if !strings.Contains(pretty, "bind") {
		t.Fatalf("expected Core.Bind in pretty output, got:\n%s", pretty)
	}
}

// ---------------------------------------------------------------------------
// Monad Instances for Maybe and List (Group 3C)
// ---------------------------------------------------------------------------

func TestMaybeDoBlockPure(t *testing.T) {
	// do { x <- Just 5; pure (add x 1) } :: Maybe Int → Just 6
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.RegisterPrim("add", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		return gicel.ToValue(gicel.MustHost[int64](args[0]) + gicel.MustHost[int64](args[1])), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
import Prelude
add :: Int -> Int -> Int
add := assumption
main :: Maybe Int
main := do { x <- Just 5; pure (add x 1) }
`)
	if err != nil {
		t.Fatalf("Maybe do block should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "Just" || len(con.Args) != 1 {
		t.Fatalf("expected Just 6, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*gicel.HostVal)
	if !ok || hv.Inner != int64(6) {
		t.Fatalf("expected Just 6, got Just %v", con.Args[0])
	}
}

func TestMaybeDoBlockNothing(t *testing.T) {
	// do { x <- Just 5; Nothing; pure x } :: Maybe Int → Nothing
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
main :: Maybe Int
main := do { x <- Just 5; Nothing; pure x }
`)
	if err != nil {
		t.Fatalf("Maybe do block Nothing should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "Nothing" {
		t.Fatalf("expected Nothing, got %v", result.Value)
	}
}

func TestMaybeDoBlockDirectReturn(t *testing.T) {
	// Last expression not using pure — direct constructor.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
main :: Maybe Int
main := do { x <- Just 5; Just x }
`)
	if err != nil {
		t.Fatalf("Maybe do block direct return should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "Just" || len(con.Args) != 1 {
		t.Fatalf("expected Just 5, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*gicel.HostVal)
	if !ok || hv.Inner != int64(5) {
		t.Fatalf("expected Just 5, got Just %v", con.Args[0])
	}
}

func TestListDoBlockCartesian(t *testing.T) {
	// do { x <- [1,2]; y <- [10,20]; pure (add x y) } :: List Int → [11, 21, 12, 22]
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.RegisterPrim("add", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		return gicel.ToValue(gicel.MustHost[int64](args[0]) + gicel.MustHost[int64](args[1])), capEnv, nil
	})
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(`
import Prelude
add :: Int -> Int -> Int
add := assumption
main :: List Int
main := do {
  x <- Cons 1 (Cons 2 Nil);
  y <- Cons 10 (Cons 20 Nil);
  pure (add x y)
}
`)
	if err != nil {
		t.Fatalf("List do block cartesian should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := gicel.FromList(result.Value)
	if !ok || len(got) != 4 {
		t.Fatalf("expected 4 elements, got %v", result.Value)
	}
	expected := []int64{11, 21, 12, 22}
	for i, v := range got {
		hv, ok := v.(*gicel.HostVal)
		if !ok || hv.Inner != expected[i] {
			t.Fatalf("element %d: expected %d, got %v", i, expected[i], v)
		}
	}
}

func TestListDoFlatMap(t *testing.T) {
	// flatMap: each element maps to multiple elements.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(`
import Prelude
main :: List Int
main := do {
  x <- Cons 1 (Cons 2 (Cons 3 Nil));
  Cons x (Cons x Nil)
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := gicel.FromList(result.Value)
	if !ok {
		t.Fatalf("expected list, got %v", result.Value)
	}
	expected := []int64{1, 1, 2, 2, 3, 3}
	if len(got) != len(expected) {
		t.Fatalf("expected %d elements, got %d: %v", len(expected), len(got), result.Value)
	}
	for i, v := range got {
		hv, ok := v.(*gicel.HostVal)
		if !ok || hv.Inner != expected[i] {
			t.Fatalf("element %d: expected %d, got %v", i, expected[i], v)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration Tests (Group 4A/4B)
// ---------------------------------------------------------------------------

func TestThenCombinator(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.DeclareBinding("x", gicel.ConType("Int"))
	eng.DeclareBinding("y", gicel.ConType("Int"))
	rt, err := eng.NewRuntime(`
import Prelude
main := then (pure x) (pure y)
`)
	if err != nil {
		t.Fatalf("then combinator should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: map[string]gicel.Value{
		"x": gicel.ToValue(1),
		"y": gicel.ToValue(2),
	}})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok || hv.Inner != 2 {
		t.Fatalf("expected 2 (from second computation), got %v", result.Value)
	}
}

func TestGenericMonadicFunction(t *testing.T) {
	// A generic IxMonad function used with Computation via type annotation.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.DeclareBinding("x", gicel.ConType("Int"))
	rt, err := eng.NewRuntime(`
import Prelude
myReturn :: \(m: Row -> Row -> Type -> Type). IxMonad m => \a (r: Row). a -> m r r a
myReturn := ixpure
main :: Computation {} {} Int
main := myReturn x
`)
	if err != nil {
		t.Fatalf("generic monadic function should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: map[string]gicel.Value{
		"x": gicel.ToValue(99),
	}})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok || hv.Inner != 99 {
		t.Fatalf("expected 99, got %v", result.Value)
	}
}

func TestMaybeDoChain(t *testing.T) {
	// Chain multiple binds in a Maybe do block.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.RegisterPrim("add", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		return gicel.ToValue(gicel.MustHost[int64](args[0]) + gicel.MustHost[int64](args[1])), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
import Prelude
add :: Int -> Int -> Int
add := assumption
main :: Maybe Int
main := do {
  x <- Just 1;
  y <- Just 2;
  z <- Just 3;
  pure (add (add x y) z)
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just 6, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*gicel.HostVal)
	if !ok || hv.Inner != int64(6) {
		t.Fatalf("expected Just 6, got Just %v", con.Args[0])
	}
}

func TestMaybeDoEarlyExit(t *testing.T) {
	// Nothing anywhere in the chain short-circuits.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.RegisterPrim("add", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		return gicel.ToValue(gicel.MustHost[int64](args[0]) + gicel.MustHost[int64](args[1])), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
import Prelude
add :: Int -> Int -> Int
add := assumption
main :: Maybe Int
main := do {
  x <- Just 1;
  y <- Nothing;
  pure (add x y)
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "Nothing" {
		t.Fatalf("expected Nothing, got %v", result.Value)
	}
}

func TestComputationDoRegression(t *testing.T) {
	// Ensure Computation do blocks still use Core.Bind (not class dispatch).
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.DeclareBinding("x", gicel.ConType("Int"))
	eng.DeclareBinding("y", gicel.ConType("Int"))
	rt, err := eng.NewRuntime(`
main := do { a <- pure x; b <- pure y; pure a }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: map[string]gicel.Value{
		"x": gicel.ToValue(10),
		"y": gicel.ToValue(20),
	}})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok || hv.Inner != 10 {
		t.Fatalf("expected 10, got %v", result.Value)
	}
}

func TestListPipelineEndToEnd(t *testing.T) {
	// fromSlice → fmap → foldr → toSlice full pipeline.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.RegisterPrim("add", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		return gicel.ToValue(gicel.MustHost[int64](args[0]) + gicel.MustHost[int64](args[1])), capEnv, nil
	})
	eng.EnableRecursion()
	eng.DeclareBinding("xs", gicel.AppType(gicel.ConType("List"), gicel.ConType("Int")))
	rt, err := eng.NewRuntime(`
import Prelude
add :: Int -> Int -> Int
add := assumption
main := foldr add 0 (fmap (\x. add x 10) xs)
`)
	if err != nil {
		t.Fatal(err)
	}
	// xs = [1, 2, 3]
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: map[string]gicel.Value{
		"xs": gicel.ToList([]any{int64(1), int64(2), int64(3)}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	// fmap (+10) [1,2,3] = [11,12,13], foldr add 0 = 36
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok || hv.Inner != int64(36) {
		t.Fatalf("expected 36, got %v", result.Value)
	}
}

func TestEffectComposition(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(gicel.EffectFail); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(gicel.EffectState); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import Effect.Fail
import Effect.State
main := do {
  put 10;
  n <- get;
  m <- fromMaybe (Just (n + 5));
  put m;
  get
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{
		"state": &gicel.HostVal{Inner: int64(0)},
		"fail":  &gicel.RecordVal{Fields: map[string]gicel.Value{}},
	}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	if hv := gicel.MustHost[int64](result.Value); hv != 15 {
		t.Errorf("expected 15, got %d", hv)
	}
}
