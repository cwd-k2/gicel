// Integration tests — full pipeline, e2e features, stdlib effects, list operations, type families.
// Does NOT cover: literals (literal_test.go), host API (host_api_test.go), sandbox (sandbox_test.go).

package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/stdlib"
)

func TestFailAbort(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	if err := eng.Use(stdlib.Fail); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Effect.Fail
main := do { fail; pure True }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"fail": &eval.RecordVal{Fields: map[string]eval.Value{}}}
	_, err = rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err == nil {
		t.Fatal("expected error from fail")
	}
}

func TestFromMaybe(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	if err := eng.Use(stdlib.Fail); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Effect.Fail
main := fromMaybe (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"fail": &eval.RecordVal{Fields: map[string]eval.Value{}}}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	b, ok := FromBool(result.Value)
	if !ok || !b {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestStateGetPut(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(stdlib.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import Effect.State
main := do { put 42; get }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &eval.HostVal{Inner: int64(0)}}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	hv := MustHost[int64](result.Value)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

func TestStateThread(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(stdlib.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import Effect.State
main := do { n <- get; put (n + 1); get }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &eval.HostVal{Inner: int64(0)}}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	hv := MustHost[int64](result.Value)
	if hv != 1 {
		t.Errorf("expected 1, got %d", hv)
	}
}

func TestFromResult(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	if err := eng.Use(stdlib.Fail); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Effect.Fail
main := fromResult (Ok True)
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"fail": &eval.RecordVal{Fields: map[string]eval.Value{}}}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	b, ok := FromBool(result.Value)
	if !ok || !b {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestFailWithState(t *testing.T) {
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
	rt, err := eng.NewRuntime(`
import Prelude
import Effect.Fail
import Effect.State
main := do { put 42; fail }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{
		"state": &eval.HostVal{Inner: int64(0)},
		"fail":  &eval.RecordVal{Fields: map[string]eval.Value{}},
	}
	_, err = rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err == nil {
		t.Fatal("expected error from fail")
	}
}

// :: type annotation in source — the spec-canonical way to type assumptions.
func TestInSourceTypeAnnotation(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.RegisterPrim("getVal", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return ToValue(99), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
import Prelude
getVal :: () -> Computation {} {} Int
getVal := assumption
main := do { x <- getVal (); pure x }
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	v := MustHost[int](result.Value)
	if v != 99 {
		t.Errorf("expected 99, got %d", v)
	}
}

// Forall with kinded binder: \ (r: Row). T
func TestKindedForallBinder(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.RegisterPrim("getVal", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return ToValue(7), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
import Prelude
getVal :: \(r: Row). () -> Computation r r Int
getVal := assumption
main := do { getVal () }
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	v := MustHost[int](result.Value)
	if v != 7 {
		t.Errorf("expected 7, got %d", v)
	}
}

// Result type has 2 parameters: Result e a = Ok a | Err e
func TestResult2Params(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("String", KindType())
	rt, err := eng.NewRuntime(`
import Prelude
main := Ok True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Ok" {
		t.Errorf("expected Ok, got %s", result.Value)
	}
	if len(con.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(con.Args))
	}
	inner, ok := con.Args[0].(*eval.ConVal)
	if !ok || inner.Con != "True" {
		t.Errorf("expected True inside Ok, got %s", con.Args[0])
	}
}

// Type alias: type Effect r a := Computation r r a
func TestTypeAlias(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
type Effect r a := Computation r r a

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
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// Prelude's Effect alias should work without user-defined alias.
func TestPreludeEffectAlias(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
main :: Effect {} Bool
main := pure True
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// Env flat map: deeply nested binds don't retain parent chains.
func TestDeepBindEnvDoesNotLeak(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableRecursion()
	eng.RegisterType("Int", KindType())
	eng.RegisterPrim("mkInt", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return ToValue(0), capEnv, nil
	})
	// A chain of 100 nested binds. With linked-list Env, each
	// closure would retain the entire chain. With flat Env, each
	// only holds its own bindings map.
	rt, err := eng.NewRuntime(`
import Prelude
mkInt :: () -> Computation {} {} Int
mkInt := assumption
main := do {
  x0 <- mkInt ();
  x1 <- mkInt ();
  x2 <- mkInt ();
  x3 <- mkInt ();
  x4 <- mkInt ();
  x5 <- mkInt ();
  x6 <- mkInt ();
  x7 <- mkInt ();
  x8 <- mkInt ();
  x9 <- mkInt ();
  pure x9
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	v := MustHost[int](result.Value)
	if v != 0 {
		t.Errorf("expected 0, got %d", v)
	}
}

func TestFullPipeline(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
data SomeEq := { MkSomeEq :: \a. Eq a => a -> SomeEq }
isSelf :: SomeEq -> Bool
isSelf := \s. case s { MkSomeEq x -> eq x x }
applyId :: (\a. a -> a) -> Bool
applyId := \f. f True
id :: \a. a -> a
id := \x. x
main := case isSelf (MkSomeEq True) { True -> applyId id; False -> False }
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

func TestBackslashForallE2E(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
id :: \a. a -> a
id := \x. x
applyId :: (\a. a -> a) -> Bool
applyId := \f. f True
main := applyId id
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

func TestConstraintTupleE2E(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
eqAndOrd :: \a. (Eq a, Ord a) => a -> a -> Bool
eqAndOrd := \x y. case eq x y { True -> True; False -> case compare x y { EQ -> True; _ -> False } }
main := eqAndOrd True True
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

func TestListFmap(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := fmap (\x. add x 1) (Cons 1 (Cons 2 (Cons 3 Nil)))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Expected: Cons 2 (Cons 3 (Cons 4 Nil))
	assertList(t, result.Value, []int64{2, 3, 4})
}

func TestListFoldr(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := foldr add 0 (Cons 1 (Cons 2 (Cons 3 Nil)))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", result.Value)
	}
	if hv.Inner != int64(6) {
		t.Fatalf("expected 6, got %v", hv.Inner)
	}
}

func TestListAppend(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := append (Cons 1 (Cons 2 Nil)) (Cons 3 Nil)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertList(t, result.Value, []int64{1, 2, 3})
}

func TestListEq(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := eq (Cons 1 (Cons 2 Nil)) (Cons 1 (Cons 2 Nil))
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
		t.Fatalf("expected True, got %v", result.Value)
	}
}

func TestListEqDifferent(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := eq (Cons 1 Nil) (Cons 2 Nil)
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
		t.Fatalf("expected False, got %v", result.Value)
	}
}

func TestListMonoidEmpty(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
main := (empty :: List Bool)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Nil" {
		t.Fatalf("expected Nil, got %v", result.Value)
	}
}

func TestLiftAliasExpansion(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.NewRuntime(`
import Prelude
test :: Lift Maybe {} {} Bool
test := Just True
main := True
`)
	if err != nil {
		t.Fatalf("Lift alias should expand to Maybe Bool: %v", err)
	}
}

func TestKindedClassParam(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.NewRuntime(`
import Prelude
class MyClass (m: Row -> Row -> Type -> Type) {
  myPure :: \a (r: Row). a -> m r r a
}
main := True
`)
	if err != nil {
		t.Fatalf("kinded class param should compile: %v", err)
	}
}

func TestKindedClassParamWithInstance(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
class Wrap (f: Type -> Type) {
  wrap :: \a. a -> f a
}
instance Wrap Maybe {
  wrap := \x. Just x
}
main := wrap True
`)
	if err != nil {
		t.Fatalf("kinded class with instance should compile: %v", err)
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

func TestToListFromList(t *testing.T) {
	list := ToList([]any{
		&eval.HostVal{Inner: int64(1)},
		&eval.HostVal{Inner: int64(2)},
		&eval.HostVal{Inner: int64(3)},
	})
	items, ok := FromList(list)
	if !ok {
		t.Fatal("expected valid list")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	for i, want := range []int64{1, 2, 3} {
		hv := items[i].(*eval.HostVal)
		if hv.Inner != want {
			t.Fatalf("element %d: expected %d, got %v", i, want, hv.Inner)
		}
	}
}

func TestToListEmpty(t *testing.T) {
	list := ToList(nil)
	con, ok := list.(*eval.ConVal)
	if !ok || con.Con != "Nil" {
		t.Fatalf("expected Nil, got %v", list)
	}
}

func TestFromListInvalid(t *testing.T) {
	_, ok := FromList(&eval.HostVal{Inner: 42})
	if ok {
		t.Fatal("expected false for non-list")
	}
}

func TestToListWithBinding(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	eng.DeclareBinding("xs", AppType(ConType("List"), ConType("Int")))
	rt, err := eng.NewRuntime(`
import Prelude
main := foldr add 0 xs
`)
	if err != nil {
		t.Fatal(err)
	}
	bindings := map[string]eval.Value{
		"xs": ToList([]any{
			&eval.HostVal{Inner: int64(1)},
			&eval.HostVal{Inner: int64(2)},
			&eval.HostVal{Inner: int64(3)},
		}),
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Bindings: bindings})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != int64(6) {
		t.Fatalf("expected 6, got %v", result.Value)
	}
}

func TestListStdlibLength(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := length (Cons True (Cons False Nil))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != int64(2) {
		t.Fatalf("expected 2, got %v", result.Value)
	}
}

func TestTypeFamilyIsolationAcrossCompilations(t *testing.T) {
	eng := NewEngine()
	// Register a module with a class + associated type.
	err := eng.RegisterModule("TFLib", `
data Bool := True | False
data Nat := Zero | Succ Nat

class Convert a {
  type Target a :: Type;
  convert :: a -> Target a
}
`)
	if err != nil {
		t.Fatal(err)
	}
	// First compilation: instance Convert Bool.
	rt1, err := eng.NewRuntime(`
import TFLib
instance Convert Bool {
  type Target Bool =: Nat;
  convert := \b. case b { True -> Succ Zero; False -> Zero }
}
main := convert True
`)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := rt1.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con1, ok := r1.Value.(*eval.ConVal)
	if !ok || con1.Con != "Succ" {
		t.Fatalf("first compilation: expected Succ, got %s", r1.Value)
	}
	// Second compilation: same import, different instance.
	// If isolation is broken, the first compilation's equations
	// would have leaked into the module metadata.
	rt2, err := eng.NewRuntime(`
import TFLib
instance Convert Nat {
  type Target Nat =: Bool;
  convert := \n. case n { Zero -> False; Succ _ -> True }
}
main := convert Zero
`)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con2, ok := r2.Value.(*eval.ConVal)
	if !ok || con2.Con != "False" {
		t.Fatalf("second compilation: expected False, got %s", r2.Value)
	}
}
