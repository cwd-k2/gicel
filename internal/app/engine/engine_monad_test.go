package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// ---------------------------------------------------------------------------
// GIMonad Class (Group 2B)
// ---------------------------------------------------------------------------

func TestGIMonadClassExists(t *testing.T) {
	// Verify GIMonad class is defined in prelude and usable in type annotations.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
myFn :: \(g: Kind) (m: g -> Row -> Row -> Type -> Type). GIMonad g m => \a (r: Row). a -> m GradeDrop r r a
myFn := gipure
main := True
`)
	if err != nil {
		t.Fatalf("GIMonad class should be usable: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GIMonad Computation (Group 2C)
// ---------------------------------------------------------------------------

func TestGIMonadComputationPure(t *testing.T) {
	// pure resolves to Core.Pure for Computation (fast path).
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.DeclareBinding("n", ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: Computation {} {} Int
main := pure n
`)
	if err != nil {
		t.Fatalf("pure should resolve for Computation: %v", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Bindings: map[string]eval.Value{
		"n": &eval.HostVal{Inner: 42},
	}})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != 42 {
		t.Fatalf("expected 42, got %v", result.Value)
	}
}

func TestGIMonadComputationBind(t *testing.T) {
	// bind resolves to Core.Bind for Computation (fast path).
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.DeclareBinding("n", ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: Computation {} {} Int
main := bind (pure n) (\x. pure x)
`)
	if err != nil {
		t.Fatalf("bind should resolve for Computation: %v", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Bindings: map[string]eval.Value{
		"n": &eval.HostVal{Inner: 99},
	}})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != 99 {
		t.Fatalf("expected 99, got %v", result.Value)
	}
}

func TestGIMonadGenericFunction(t *testing.T) {
	// A generic function using GIMonad constraint with explicit grade should work.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
myReturn :: \a (r: Row). a -> Computation r r a
myReturn := pure
main :: Computation {} {} Bool
main := myReturn True
`)
	if err != nil {
		t.Fatalf("generic monadic function should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Fatalf("expected True, got %v", result.Value)
	}
}

// ---------------------------------------------------------------------------
// Type-Driven do Dispatch (Group 3A)
// ---------------------------------------------------------------------------

func TestDoBlockComputationRegression(t *testing.T) {
	// Existing do blocks with Computation should still use Core.Bind path.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.DeclareBinding("x", ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `
main := do { v <- pure x; pure v }
`)
	if err != nil {
		t.Fatalf("Computation do block regression failed: %v", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Bindings: map[string]eval.Value{
		"x": &eval.HostVal{Inner: 42},
	}})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != 42 {
		t.Fatalf("expected 42, got %v", result.Value)
	}
}

func TestDoBlockComputationCoreBind(t *testing.T) {
	// Verify Computation do blocks elaborate to Core.Bind (not class dispatch).
	// Use Check() to inspect pre-optimization Core IR.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.DeclareBinding("x", ConType("Int"))
	cp, err := eng.Compile(context.Background(), `
main := do { v <- pure x; pure v }
`)
	if err != nil {
		t.Fatal(err)
	}
	// Core.Bind is used when Pretty() shows "bind" nodes rather than "gibind" calls.
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
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterPrim("add", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return ToValue(MustHost[int64](args[0]) + MustHost[int64](args[1])), capEnv, nil
	})
	rt, err := eng.NewRuntime(context.Background(), `
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
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Just" || len(con.Args) != 1 {
		t.Fatalf("expected Just 6, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*eval.HostVal)
	if !ok || hv.Inner != int64(6) {
		t.Fatalf("expected Just 6, got Just %v", con.Args[0])
	}
}

func TestMaybeDoBlockNothing(t *testing.T) {
	// do { x <- Just 5; Nothing; pure x } :: Maybe Int → Nothing
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
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
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Nothing" {
		t.Fatalf("expected Nothing, got %v", result.Value)
	}
}

func TestMaybeDoBlockDirectReturn(t *testing.T) {
	// Last expression not using pure — direct constructor.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
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
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Just" || len(con.Args) != 1 {
		t.Fatalf("expected Just 5, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*eval.HostVal)
	if !ok || hv.Inner != int64(5) {
		t.Fatalf("expected Just 5, got Just %v", con.Args[0])
	}
}

func TestListDoBlockCartesian(t *testing.T) {
	// do { x <- [1,2]; y <- [10,20]; pure (add x y) } :: List Int → [11, 21, 12, 22]
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterPrim("add", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return ToValue(MustHost[int64](args[0]) + MustHost[int64](args[1])), capEnv, nil
	})
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(context.Background(), `
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
	got, ok := FromList(result.Value)
	if !ok || len(got) != 4 {
		t.Fatalf("expected 4 elements, got %v", result.Value)
	}
	expected := []int64{11, 21, 12, 22}
	for i, v := range got {
		hv, ok := v.(*eval.HostVal)
		if !ok || hv.Inner != expected[i] {
			t.Fatalf("element %d: expected %d, got %v", i, expected[i], v)
		}
	}
}

func TestListDoFlatMap(t *testing.T) {
	// flatMap: each element maps to multiple elements.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(context.Background(), `
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
	got, ok := FromList(result.Value)
	if !ok {
		t.Fatalf("expected list, got %v", result.Value)
	}
	expected := []int64{1, 1, 2, 2, 3, 3}
	if len(got) != len(expected) {
		t.Fatalf("expected %d elements, got %d: %v", len(expected), len(got), result.Value)
	}
	for i, v := range got {
		hv, ok := v.(*eval.HostVal)
		if !ok || hv.Inner != expected[i] {
			t.Fatalf("element %d: expected %d, got %v", i, expected[i], v)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration Tests (Group 4A/4B)
// ---------------------------------------------------------------------------

func TestThenCombinator(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.DeclareBinding("x", ConType("Int"))
	eng.DeclareBinding("y", ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := seq (pure x) (pure y)
`)
	if err != nil {
		t.Fatalf("seq combinator should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Bindings: map[string]eval.Value{
		"x": ToValue(1),
		"y": ToValue(2),
	}})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != 2 {
		t.Fatalf("expected 2 (from second computation), got %v", result.Value)
	}
}

func TestGenericMonadicFunction(t *testing.T) {
	// A generic Computation function using pure (builtin) via type annotation.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.DeclareBinding("x", ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
myReturn :: \a (r: Row). a -> Computation r r a
myReturn := pure
main :: Computation {} {} Int
main := myReturn x
`)
	if err != nil {
		t.Fatalf("generic monadic function should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Bindings: map[string]eval.Value{
		"x": ToValue(99),
	}})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != 99 {
		t.Fatalf("expected 99, got %v", result.Value)
	}
}

func TestMaybeDoChain(t *testing.T) {
	// Chain multiple binds in a Maybe do block.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterPrim("add", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return ToValue(MustHost[int64](args[0]) + MustHost[int64](args[1])), capEnv, nil
	})
	rt, err := eng.NewRuntime(context.Background(), `
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
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just 6, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*eval.HostVal)
	if !ok || hv.Inner != int64(6) {
		t.Fatalf("expected Just 6, got Just %v", con.Args[0])
	}
}

func TestMaybeDoEarlyExit(t *testing.T) {
	// Nothing anywhere in the chain short-circuits.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterPrim("add", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return ToValue(MustHost[int64](args[0]) + MustHost[int64](args[1])), capEnv, nil
	})
	rt, err := eng.NewRuntime(context.Background(), `
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
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Nothing" {
		t.Fatalf("expected Nothing, got %v", result.Value)
	}
}

// GIMonad Lift dispatch — verifies Maybe/List do-blocks work via
// GIMonad g (Lift M) instances, not Monad fallback.
// Lift M g r r a ≡ M a (type alias), so dispatch is transparent.

func TestGIMonadLiftMaybeDoBlock(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: Maybe Int
main := do {
  x <- Just 10;
  y <- Just 20;
  pure (x + y)
}
`)
	if err != nil {
		t.Fatalf("GIMonad Lift Maybe should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Just" || len(con.Args) != 1 {
		t.Fatalf("expected Just 30, got %v", result.Value)
	}
	if hv, ok := con.Args[0].(*eval.HostVal); !ok || hv.Inner != int64(30) {
		t.Fatalf("expected Just 30, got Just %v", con.Args[0])
	}
}

func TestGIMonadLiftListDoBlock(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: List Int
main := do {
  x <- [1, 2];
  y <- [10, 20];
  pure (x + y)
}
`)
	if err != nil {
		t.Fatalf("GIMonad Lift List should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// [1+10, 1+20, 2+10, 2+20] = [11, 21, 12, 22]
	// Verify result is a non-empty Cons chain.
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Cons" {
		t.Fatalf("expected Cons chain, got %v", result.Value)
	}
	first, ok := con.Args[0].(*eval.HostVal)
	if !ok || first.Inner != int64(11) {
		t.Fatalf("expected first element 11, got %v", con.Args[0])
	}
}

func TestGIMonadLiftMaybeNothing(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: Maybe Int
main := do {
  x <- Just 5;
  _ <- (Nothing :: Maybe Int);
  pure x
}
`)
	if err != nil {
		t.Fatalf("GIMonad Lift Maybe Nothing should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Nothing" {
		t.Fatalf("expected Nothing, got %v", result.Value)
	}
}

func TestComputationDoRegression(t *testing.T) {
	// Ensure Computation do blocks still use Core.Bind (not class dispatch).
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.DeclareBinding("x", ConType("Int"))
	eng.DeclareBinding("y", ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `
main := do { a <- pure x; b <- pure y; pure a }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Bindings: map[string]eval.Value{
		"x": ToValue(10),
		"y": ToValue(20),
	}})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != 10 {
		t.Fatalf("expected 10, got %v", result.Value)
	}
}

func TestListPipelineEndToEnd(t *testing.T) {
	// fromSlice → fmap → foldr → toSlice full pipeline.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterPrim("add", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return ToValue(MustHost[int64](args[0]) + MustHost[int64](args[1])), capEnv, nil
	})
	eng.EnableRecursion()
	eng.DeclareBinding("xs", AppType(ConType("List"), ConType("Int")))
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
add :: Int -> Int -> Int
add := assumption
main := foldr add 0 (fmap (\x. add x 10) xs)
`)
	if err != nil {
		t.Fatal(err)
	}
	// xs = [1, 2, 3]
	result, err := rt.RunWith(context.Background(), &RunOptions{Bindings: map[string]eval.Value{
		"xs": ToList([]any{int64(1), int64(2), int64(3)}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	// fmap (+10) [1,2,3] = [11,12,13], foldr add 0 = 36
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != int64(36) {
		t.Fatalf("expected 36, got %v", result.Value)
	}
}

func TestEffectComposition(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(stdlib.Fail); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(stdlib.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
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
		"state": &eval.HostVal{Inner: int64(0)},
		"fail":  eval.NewRecordFromMap(map[string]eval.Value{}),
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	if hv := MustHost[int64](result.Value); hv != 15 {
		t.Errorf("expected 15, got %d", hv)
	}
}

// ---------------------------------------------------------------------------
// pure resolution in >>= context (non-do, non-Computation monad)
// ---------------------------------------------------------------------------

func TestMaybePureInBindOperator(t *testing.T) {
	// pure inside >>= continuation should resolve via class dispatch, not Computation.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: Maybe Int
main := Just 5 >>= \x. pure x
`)
	if err != nil {
		t.Fatalf("pure in >>= context should resolve via class dispatch: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Just" || len(con.Args) != 1 {
		t.Fatalf("expected Just 5, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*eval.HostVal)
	if !ok || hv.Inner != int64(5) {
		t.Fatalf("expected Just 5, got Just %v", con.Args[0])
	}
}

func TestMaybePureInBindOperatorWithTransform(t *testing.T) {
	// pure (f x) inside >>= continuation.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterPrim("double", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return ToValue(MustHost[int64](args[0]) * 2), capEnv, nil
	})
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
double :: Int -> Int
double := assumption
main :: Maybe Int
main := Just 3 >>= \x. pure (double x)
`)
	if err != nil {
		t.Fatalf("pure with transform in >>= context should work: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Just" || len(con.Args) != 1 {
		t.Fatalf("expected Just 6, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*eval.HostVal)
	if !ok || hv.Inner != int64(6) {
		t.Fatalf("expected Just 6, got Just %v", con.Args[0])
	}
}
