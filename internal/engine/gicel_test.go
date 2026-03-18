package engine

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/reg"
	"github.com/cwd-k2/gicel/internal/stdlib"
)

func TestIdentity(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`main := pure ()`)
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

func TestHostBinding(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.DeclareBinding("x", ConType("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}
	bindings := map[string]eval.Value{
		"x": &eval.HostVal{Inner: 42},
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Bindings: bindings})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != 42 {
		t.Errorf("expected HostVal(42), got %s", result.Value)
	}
}

func TestAssumption(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.DeclareAssumption("getUnit", ArrowType(ConType("Bool"), ConType("Bool")))
	eng.RegisterPrim("getUnit", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return &eval.ConVal{Con: "True"}, capEnv, nil
	})
	rt, err := eng.NewRuntime(`
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

func TestEvalIntLit(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`main := 42`)
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
	if hv.Inner != int64(42) {
		t.Errorf("expected 42, got %v", hv.Inner)
	}
}

func TestEvalStrLit(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(`main := "hello"`)
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
	if hv.Inner != "hello" {
		t.Errorf("expected 'hello', got %v", hv.Inner)
	}
}

func TestEngineUse(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	called := false
	pack := reg.Pack(func(r reg.Registrar) error {
		called = true
		return nil
	})
	if err := eng.Use(pack); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("pack was not called")
	}
}

func TestNumAdd(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := add 1 2
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := MustHost[int64](result.Value)
	if hv != 3 {
		t.Errorf("expected 3, got %d", hv)
	}
}

func TestNumOperators(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := 1 + 2 * 3
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := MustHost[int64](result.Value)
	if hv != 7 {
		t.Errorf("expected 7, got %d", hv)
	}
}

func TestNumNegate(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := negate 42
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := MustHost[int64](result.Value)
	if hv != -42 {
		t.Errorf("expected -42, got %d", hv)
	}
}

func TestNumEqInt(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := eq 1 1
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, ok := FromBool(result.Value)
	if !ok || !b {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestNumOrdInt(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := compare 1 2
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "LT" {
		t.Errorf("expected LT, got %s", result.Value)
	}
}

func TestNumDivMod(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := div 7 3
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := MustHost[int64](result.Value)
	if hv != 2 {
		t.Errorf("expected 2, got %d", hv)
	}
}

func TestStrConcat(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := append "hello" " world"
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := MustHost[string](result.Value)
	if hv != "hello world" {
		t.Errorf("expected 'hello world', got %s", hv)
	}
}

func TestStrEq(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := eq "abc" "abc"
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, ok := FromBool(result.Value)
	if !ok || !b {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestStrOrd(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := compare "a" "b"
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "LT" {
		t.Errorf("expected LT, got %s", result.Value)
	}
}

func TestStrLength(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := strlen "hello"
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := MustHost[int64](result.Value)
	if hv != 5 {
		t.Errorf("expected 5, got %d", hv)
	}
}

func TestRuneEq(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := eq 'a' 'a'
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, ok := FromBool(result.Value)
	if !ok || !b {
		t.Errorf("expected True, got %s", result.Value)
	}
}

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

func TestCaseExpression(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
not := \b. case b { True -> False; False -> True }
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
	rt, err := eng.NewRuntime(`main := do { pure () }`)
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

func TestNoPrelude(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(`
data MyBool := MyTrue | MyFalse
main := MyTrue
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "MyTrue" {
		t.Errorf("expected MyTrue, got %s", result.Value)
	}
}

func TestCompileError(t *testing.T) {
	eng := NewEngine()
	_, err := eng.NewRuntime(`main := undefined_thing`)
	if err == nil {
		t.Error("expected compile error")
	}
	_, ok := err.(*CompileError)
	if !ok {
		t.Errorf("expected CompileError, got %T", err)
	}
}

func TestMissingEntry(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
x := True
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Error("expected error for missing entry point")
	}
}

func TestConstructorArgs(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`
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

func TestStepLimit(t *testing.T) {
	eng := NewEngine()
	eng.SetStepLimit(100)
	eng.EnableRecursion()
	// No recursion in source, just a program that completes in few steps.
	rt, err := eng.NewRuntime(`
data Unit := Unit
main := Unit
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stats.Steps == 0 {
		t.Error("expected non-zero step count")
	}
}

func TestGoroutineSafety(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
main := True
`)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 10)
	for range 10 {
		go func() {
			_, err := rt.RunWith(context.Background(), nil)
			errs <- err
		}()
	}
	for range 10 {
		if err := <-errs; err != nil {
			t.Errorf("concurrent execution failed: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// A. Pure computation
// ---------------------------------------------------------------------------

func TestMultiParamLambda(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`
import Prelude
bothTrue := \x y. case x { True -> case y { True -> True; False -> False }; False -> False }
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
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`
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

// ---------------------------------------------------------------------------
// B. Computation
// ---------------------------------------------------------------------------

func TestBindChain(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`main := force (thunk (pure ()))`)
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

// ---------------------------------------------------------------------------
// C. Type errors (compile-time rejection)
// ---------------------------------------------------------------------------

func TestTypeErrorUnboundVar(t *testing.T) {
	eng := NewEngine()
	_, err := eng.NewRuntime(`main := totally_undefined`)
	if err == nil {
		t.Fatal("expected compile error for unbound variable")
	}
	if _, ok := err.(*CompileError); !ok {
		t.Errorf("expected CompileError, got %T", err)
	}
}

func TestTypeErrorNonExhaustive(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.NewRuntime(`
import Prelude
f := \x. case x { True -> () }
main := f True
`)
	if err == nil {
		t.Fatal("expected compile error for non-exhaustive pattern")
	}
	ce, ok := err.(*CompileError)
	if !ok {
		t.Fatalf("expected CompileError, got %T", err)
	}
	if !strings.Contains(ce.Error(), "non-exhaustive") {
		t.Errorf("expected non-exhaustive in error message, got: %s", ce.Error())
	}
}

func TestTypeErrorMismatch(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	// Apply True (a Bool, not a function) to an argument.
	_, err := eng.NewRuntime(`
import Prelude
main := True ()
`)
	if err == nil {
		t.Fatal("expected compile error for type mismatch")
	}
	if _, ok := err.(*CompileError); !ok {
		t.Errorf("expected CompileError, got %T", err)
	}
}

// ---------------------------------------------------------------------------
// D. Context cancellation
// ---------------------------------------------------------------------------

func TestContextCancellation(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(`main := pure ()`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err = rt.RunWith(ctx, nil)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestContextTimeout(t *testing.T) {
	eng := NewEngine()
	eng.SetStepLimit(1) // extremely low step limit
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(`
data Unit := Unit
data Bool := True | False
not := \b. case b { True -> False; False -> True }
main := not (not (not True))
`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err = rt.RunWith(ctx, nil)
	if err == nil {
		t.Error("expected error from step limit or timeout")
	}
}

// ---------------------------------------------------------------------------
// E. Runtime reuse
// ---------------------------------------------------------------------------

func TestRuntimeReuse(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.DeclareBinding("x", ConType("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}

	// First run with value 1.
	r1, err := rt.RunWith(context.Background(), &RunOptions{Bindings: map[string]eval.Value{"x": &eval.HostVal{Inner: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	h1, ok := r1.Value.(*eval.HostVal)
	if !ok || h1.Inner != 1 {
		t.Errorf("first run: expected HostVal(1), got %s", r1.Value)
	}

	// Second run with value 2.
	r2, err := rt.RunWith(context.Background(), &RunOptions{Bindings: map[string]eval.Value{"x": &eval.HostVal{Inner: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	h2, ok := r2.Value.(*eval.HostVal)
	if !ok || h2.Inner != 2 {
		t.Errorf("second run: expected HostVal(2), got %s", r2.Value)
	}
}

// Regression: NewRuntime must snapshot the primitive registry.
// Post-compilation RegisterPrim on the Engine must not affect existing Runtimes.
func TestRuntimePrimRegistryIsolation(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.DeclareAssumption("f", ArrowType(ConType("Bool"), ConType("Bool")))
	eng.RegisterPrim("f", func(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return &eval.ConVal{Con: "True"}, ce, nil
	})

	rt, err := eng.NewRuntime("import Prelude\nf := assumption\nmain := f False")
	if err != nil {
		t.Fatal(err)
	}

	// Overwrite "f" on the Engine after NewRuntime.
	eng.RegisterPrim("f", func(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return &eval.ConVal{Con: "False"}, ce, nil
	})

	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Runtime should use the original "f" (returns True), not the overwritten one.
	assertConVal(t, result.Value, "True")
}

func TestRuntimeConcurrent(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
not := \b. case b { True -> False; False -> True }
main := not False
`)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result, err := rt.RunWith(context.Background(), nil)
			if err != nil {
				errs[idx] = err
				return
			}
			con, ok := result.Value.(*eval.ConVal)
			if !ok || con.Con != "True" {
				errs[idx] = context.DeadlineExceeded // sentinel
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d failed: %v", i, err)
		}
	}
}

// ---------------------------------------------------------------------------
// F. Host API
// ---------------------------------------------------------------------------

func TestDiagnostics(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.NewRuntime(`main := undefined_xyz`)
	if err == nil {
		t.Fatal("expected compile error")
	}
	ce, ok := err.(*CompileError)
	if !ok {
		t.Fatalf("expected CompileError, got %T", err)
	}
	diags := ce.Diagnostics()
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic")
	}
	d := diags[0]
	if d.Phase == "" {
		t.Errorf("expected non-empty phase")
	}
	if d.Message == "" {
		t.Errorf("expected non-empty message")
	}
	if d.Line == 0 {
		t.Errorf("expected non-zero line number")
	}
}

func TestCheckOnly(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	prog, err := eng.Compile(`
import Prelude
main := True
`)
	if err != nil {
		t.Fatal(err)
	}
	if prog == nil {
		t.Error("expected non-nil program from Check")
	}
}

func TestParseOnly(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	if err := eng.Parse(`main := True`); err != nil {
		t.Fatal(err)
	}
}

func TestPrettyProgram(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
main := True
`)
	if err != nil {
		t.Fatal(err)
	}
	pretty := rt.Program().Pretty()
	if pretty == "" {
		t.Error("expected non-empty Program().Pretty() output")
	}
}

func TestRunWith(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`main := pure ()`)
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
	// CapEnv should be present (even if empty).
	_ = result.CapEnv
	_ = result.Stats
}

func TestValueConversion(t *testing.T) {
	// HostVal wraps arbitrary Go values.
	hv := &eval.HostVal{Inner: "hello"}
	if hv.Inner != "hello" {
		t.Errorf("expected HostVal.Inner = hello, got %v", hv.Inner)
	}
	if hv.String() != "HostVal(hello)" {
		t.Errorf("unexpected HostVal.String(): %s", hv.String())
	}

	// ConVal represents constructors.
	cv := &eval.ConVal{Con: "True"}
	if cv.String() != "True" {
		t.Errorf("expected True, got %s", cv.String())
	}

	// ConVal with arguments.
	cv2 := &eval.ConVal{Con: "Just", Args: []eval.Value{&eval.ConVal{Con: "True"}}}
	s := cv2.String()
	if !strings.Contains(s, "Just") || !strings.Contains(s, "True") {
		t.Errorf("expected (Just True), got %s", s)
	}
}

// ---------------------------------------------------------------------------
// G. Type helpers
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// H. Boundary conditions and spec compliance
// ---------------------------------------------------------------------------

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

// Assumption without any type annotation must produce a compile error.
func TestAssumptionNoType(t *testing.T) {
	eng := NewEngine()
	_, err := eng.NewRuntime(`
noType := assumption
main := noType ()
`)
	if err == nil {
		t.Fatal("expected compile error for assumption without type")
	}
	ce, ok := err.(*CompileError)
	if !ok {
		t.Fatalf("expected CompileError, got %T", err)
	}
	if !strings.Contains(ce.Error(), "assumption") {
		t.Errorf("expected mention of assumption in error, got: %s", ce.Error())
	}
}

// _ <- in do blocks must work (was a parser bug: TokUnderscore != TokLower).
func TestDoWildcardBind(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`
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

// Missing runtime binding must produce a runtime error (not panic).
func TestMissingRuntimeBinding(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.DeclareBinding("x", ConType("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}
	// Run without providing binding "x".
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for missing runtime binding")
	}
	if !strings.Contains(err.Error(), "missing binding") {
		t.Errorf("expected 'missing binding' in error, got: %s", err.Error())
	}
}

// PrimVal partial application: binary assumption applied to one arg, then another.
func TestPrimPartialApplication(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.RegisterPrim("add", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		a := MustHost[int](args[0])
		b := MustHost[int](args[1])
		return ToValue(a + b), capEnv, nil
	})
	eng.DeclareBinding("a", ConType("Int"))
	eng.DeclareBinding("b", ConType("Int"))
	rt, err := eng.NewRuntime(`
import Prelude
add :: Int -> Int -> Int
add := assumption
main := add a b
`)
	if err != nil {
		t.Fatal(err)
	}
	bindings := map[string]eval.Value{
		"a": ToValue(10),
		"b": ToValue(32),
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Bindings: bindings})
	if err != nil {
		t.Fatal(err)
	}
	v := MustHost[int](result.Value)
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

// Cyclic type alias must produce a compile error.
func TestCyclicTypeAlias(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.NewRuntime(`
import Prelude
type A := B
type B := A
main := True
`)
	if err == nil {
		t.Fatal("expected compile error for cyclic type alias")
	}
	if !strings.Contains(err.Error(), "cyclic") {
		t.Errorf("expected 'cyclic' in error, got: %s", err.Error())
	}
}

// Non-exhaustive case on Maybe must name the missing constructor.
func TestNonExhaustiveMaybe(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.NewRuntime(`
import Prelude
f := \x. case x { Just y -> y }
main := f (Just True)
`)
	if err == nil {
		t.Fatal("expected compile error for non-exhaustive case on Maybe")
	}
	ce := err.(*CompileError)
	if !strings.Contains(ce.Error(), "Nothing") {
		t.Errorf("expected missing constructor 'Nothing', got: %s", ce.Error())
	}
}

// ToValue and FromBool round-trip.
func TestToValueFromBool(t *testing.T) {
	v := ToValue(true)
	b, ok := FromBool(v)
	if !ok || b != true {
		t.Errorf("ToValue(true) -> FromBool failed")
	}
	v2 := ToValue(false)
	b2, ok := FromBool(v2)
	if !ok || b2 != false {
		t.Errorf("ToValue(false) -> FromBool failed")
	}
}

// ToValue(nil) produces ().
func TestToValueNil(t *testing.T) {
	v := ToValue(nil)
	rv, ok := v.(*eval.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("ToValue(nil) should produce (), got %s", v)
	}
}

// FromHost on non-HostVal returns ok=false.
func TestFromHostNonHost(t *testing.T) {
	v := ToValue(true) // ConVal, not HostVal
	_, ok := FromHost(v)
	if ok {
		t.Error("FromHost on ConVal should return ok=false")
	}
}

// MustHost panics on wrong type.
func TestMustHostPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from MustHost on ConVal")
		}
	}()
	MustHost[int](ToValue(true))
}

func TestRuntimeErrorType(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.Fail)
	rt, err := eng.NewRuntime(`
import Effect.Fail
main := do { _ <- fail; pure True }
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected runtime error from fail")
	}
	var rtErr *eval.RuntimeError
	if !errors.As(err, &rtErr) {
		t.Errorf("expected RuntimeError, got %T: %v", err, err)
	}
}

func TestFromRecord(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
main := { x: 1, y: 2 }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	fields, ok := FromRecord(result.Value)
	if !ok {
		t.Fatal("expected record value")
	}
	if len(fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(fields))
	}
	if _, ok := fields["x"]; !ok {
		t.Error("missing field x")
	}
}

func TestFromRecordNonRecord(t *testing.T) {
	_, ok := FromRecord(ToValue(42))
	if ok {
		t.Error("FromRecord on HostVal should return ok=false")
	}
}

func TestRecordTypeHelper(t *testing.T) {
	rty := RecordType(
		RowField{Label: "x", Type: ConType("Int")},
		RowField{Label: "y", Type: ConType("Int")},
	)
	got := TypePretty(rty)
	if !strings.Contains(got, "Record") {
		t.Errorf("expected Record type, got %s", got)
	}
}

func TestTupleTypeHelper(t *testing.T) {
	tt := TupleType(ConType("Int"), ConType("Bool"))
	got := TypePretty(tt)
	if !strings.Contains(got, "Record") || !strings.Contains(got, "_1") {
		t.Errorf("expected Record{_1, _2} type, got %s", got)
	}
}

func TestNewCapEnvExported(t *testing.T) {
	ce := eval.NewCapEnv(map[string]any{"key": "val"})
	v, ok := ce.Get("key")
	if !ok {
		t.Fatal("expected key in CapEnv")
	}
	if v != "val" {
		t.Errorf("expected val, got %v", v)
	}
}

// Explicit bind syntax: bind comp (\x. body) elaborates to Core.Bind.
func TestExplicitBind(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
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
		_, err := eng.NewRuntime("main := " + name)
		if err == nil {
			t.Errorf("expected compile error for standalone %s", name)
		}
	}
	// pure and bind are first-class functions — standalone use is valid.
	for _, name := range []string{"pure", "bind"} {
		eng := NewEngine()
		_, err := eng.NewRuntime("main := " + name)
		if err != nil {
			t.Errorf("standalone %s should compile, got: %v", name, err)
		}
	}
}

// Thunk/force round-trip in do block.
func TestThunkForceInDo(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
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

// Empty program (no main) should produce a runtime error, not panic.
func TestNoMainEntry(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`
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
	_, err := eng.NewRuntime(`
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

func TestPreludeAsModule(t *testing.T) {
	// Prelude is now loaded as an implicit module — Bool, (), etc. should be available.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
main := True
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

func TestNoPreludeWithModules(t *testing.T) {
	eng := NewEngine()
	// Without prelude, Bool is not defined — need to define it ourselves.
	rt, err := eng.NewRuntime(`
data MyBool := Yes | No
main := Yes
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Yes" {
		t.Errorf("expected Yes, got %s", result.Value)
	}
}

func TestSetPreludeCustom(t *testing.T) {
	// Custom prelude replaces default: only defines MyBool, no standard Bool.
	eng := NewEngine()
	eng.RegisterModule("Prelude", `
data MyBool := Yes | No
`)
	rt, err := eng.NewRuntime("import Prelude\nmain := Yes")
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Yes" {
		t.Errorf("expected Yes, got %s", result.Value)
	}
}

func TestSetPreludeCoreStillAvailable(t *testing.T) {
	// Core definitions (IxMonad, Effect, then) available even with custom Prelude.
	eng := NewEngine()
	eng.RegisterModule("Prelude", `
data Bool := True | False
`)
	// Effect and then come from CoreSource, Bool from custom prelude.
	rt, err := eng.NewRuntime(`
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
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestSetPreludeNoDefaultBool(t *testing.T) {
	// Custom prelude that doesn't define Bool — standard Bool should not be available.
	eng := NewEngine()
	eng.RegisterModule("Prelude", `data Color := Red | Blue`)
	_, err := eng.NewRuntime(`
import Prelude
main := True
`)
	if err == nil {
		t.Error("expected compile error: True not defined with custom prelude")
	}
}

func TestTypeHelpers(t *testing.T) {
	// ConType constructs a type constructor.
	intTy := ConType("Int")
	if TypePretty(intTy) != "Int" {
		t.Errorf("expected Int, got %s", TypePretty(intTy))
	}

	// ArrowType constructs a function type.
	arrTy := ArrowType(ConType("Bool"), ConType("Bool"))
	if got := TypePretty(arrTy); got != "Bool -> Bool" {
		t.Errorf("expected Bool -> Bool, got %s", got)
	}

	// EmptyRowType constructs an empty row.
	row := EmptyRowType()
	if TypePretty(row) != "{}" {
		t.Errorf("expected {}, got %s", TypePretty(row))
	}

	// ClosedRowType constructs a row with fields.
	row2 := ClosedRowType(RowField{Label: "x", Type: ConType("Int")})
	if got := TypePretty(row2); !strings.Contains(got, "x") {
		t.Errorf("expected row with field x, got %s", got)
	}

	// ForallType constructs a quantified type.
	forallTy := ForallType("a", ArrowType(VarType("a"), VarType("a")))
	if got := TypePretty(forallTy); !strings.Contains(got, `\`) {
		t.Errorf(`expected \ in pretty, got %s`, got)
	}

	// CompType constructs a computation type.
	compTy := CompType(EmptyRowType(), EmptyRowType(), ConType("Bool"))
	if got := TypePretty(compTy); !strings.Contains(got, "Bool") {
		t.Errorf("expected Bool in computation type, got %s", got)
	}

	// VarType constructs a type variable.
	tv := VarType("a")
	if TypePretty(tv) != "a" {
		t.Errorf("expected a, got %s", TypePretty(tv))
	}

	// Kind helpers.
	k := KindType()
	if !k.Equal(KindType()) {
		t.Errorf("KType should equal KType")
	}
	kr := KindRow()
	if kr.Equal(KindType()) {
		t.Errorf("KRow should not equal KType")
	}
	ka := KindArrow(KindType(), KindType())
	if !ka.Equal(KindArrow(KindType(), KindType())) {
		t.Errorf("KArrow(Type,Type) should equal itself")
	}

	// TypeEqual
	if !TypeEqual(ConType("Int"), ConType("Int")) {
		t.Errorf("Int should equal Int")
	}
	if TypeEqual(ConType("Int"), ConType("Bool")) {
		t.Errorf("Int should not equal Bool")
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

// helper for v0.5 tests
func assertConName(t *testing.T, v eval.Value, name string) {
	t.Helper()
	con, ok := v.(*eval.ConVal)
	if !ok {
		t.Errorf("expected ConVal(%s), got %T: %s", name, v, v)
		return
	}
	if name != "" && con.Con != name {
		t.Errorf("expected %s, got %s", name, con.Con)
	}
}

// ---------------------------------------------------------------------------
// v0.6 Integration tests
// ---------------------------------------------------------------------------

func TestLiteralWithNumPack(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := 1 + 2 * 3
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if hv := MustHost[int64](result.Value); hv != 7 {
		t.Errorf("expected 7, got %d", hv)
	}
}

func TestCustomPack(t *testing.T) {
	// A user-defined Pack that provides a custom constant.
	customPack := reg.Pack(func(r reg.Registrar) error {
		r.RegisterPrim("myConst", func(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
			return &eval.HostVal{Inner: int64(999)}, ce, nil
		})
		return r.RegisterModule("Custom", `
import Prelude

myConst :: Int
myConst := assumption

main := myConst
`)
	})
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	if err := eng.Use(customPack); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
import Custom
main := myConst
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if hv := MustHost[int64](result.Value); hv != 999 {
		t.Errorf("expected 999, got %d", hv)
	}
}

func TestParseChecksSyntax(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	if err := eng.Parse(`main := True`); err != nil {
		t.Fatal(err)
	}
}

func TestCheckReturnsOpaque(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	core, err := eng.Compile(`
import Prelude
main := True
`)
	if err != nil {
		t.Fatal(err)
	}
	if core == nil {
		t.Fatal("expected non-nil CoreProgram")
	}
	if core.Pretty() == "" {
		t.Error("expected non-empty Pretty output")
	}
}

func TestProgramOpaque(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
main := True
`)
	if err != nil {
		t.Fatal(err)
	}
	prog := rt.Program()
	if prog == nil {
		t.Fatal("expected non-nil CoreProgram from Runtime.Program()")
	}
	if prog.Pretty() == "" {
		t.Error("expected non-empty Pretty output from Runtime.Program()")
	}
}

// --- Phase 7B: Public API Edge Cases ---

func TestSetDepthLimit(t *testing.T) {
	// With TCO, tail-recursive loops keep depth flat (depth oscillates 0-1).
	// The step limit is what prevents infinite tail recursion now.
	// Verify that an infinite tail-recursive loop hits the step limit.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(1000)
	eng.SetDepthLimit(10000) // high enough to not interfere
	rt, err := eng.NewRuntime(`
loop := fix (\self x. self x)
main := loop ()
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected step limit error for infinite tail recursion")
	}
	if !strings.Contains(err.Error(), "step limit") {
		t.Fatalf("expected step limit error, got: %v", err)
	}
}

func TestEngineMultipleRuntimes(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt1, err := eng.NewRuntime(`
import Prelude
main := True
`)
	if err != nil {
		t.Fatal(err)
	}
	rt2, err := eng.NewRuntime(`
import Prelude
main := False
`)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := rt1.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := r1.Value.(*eval.ConVal); !ok || c.Con != "True" {
		t.Errorf("rt1: expected True, got %v", r1.Value)
	}
	if c, ok := r2.Value.(*eval.ConVal); !ok || c.Con != "False" {
		t.Errorf("rt2: expected False, got %v", r2.Value)
	}
}

func TestEngineMutationAfterRuntime(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.DeclareBinding("x", ConType("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}
	// Mutate engine after runtime creation.
	eng.SetStepLimit(1)
	// Runtime should still work with original limits.
	bindings := map[string]eval.Value{"x": &eval.HostVal{Inner: int64(99)}}
	result, err := rt.RunWith(context.Background(), &RunOptions{Bindings: bindings})
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != int64(99) {
		t.Errorf("expected 99, got %v", result.Value)
	}
}

// --- Phase 7C: Effectful PrimOp Deferral ---

func TestEffectfulDeferUntilBind(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	if err := eng.Use(stdlib.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`
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
	rt, err := eng.NewRuntime(`
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

// --- Phase 7D: Error Path Tests ---

func TestDepthLimitError(t *testing.T) {
	eng := NewEngine()
	eng.EnableRecursion()
	eng.SetDepthLimit(5)
	rt, err := eng.NewRuntime(`
deep := fix (\self x. self x)
main := deep ()
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for deep recursion")
	}
}

func TestPrimImplError(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := div 1 0
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for division by zero")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected division by zero error, got: %v", err)
	}
}

func TestMissingBinding(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.DeclareBinding("x", ConType("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}
	// Run without providing the binding.
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for missing binding")
	}
	if !strings.Contains(err.Error(), "missing binding") {
		t.Fatalf("expected missing binding error, got: %v", err)
	}
}

func TestMissingEntryPoint(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
import Prelude
foo := True
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for missing entry point")
	}
	if !strings.Contains(err.Error(), "entry point") {
		t.Fatalf("expected entry point error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// List Instances (Group 1A)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Kinded Class Type Params (Group 2A)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Go Boundary Conversion (Group 1D)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// List Literal Syntax (Group 1C)
// ---------------------------------------------------------------------------

func TestListLiteralBasic(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
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
	assertList(t, result.Value, []int64{1, 2, 3})
}

func TestListLiteralEmpty(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(`
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
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Nil" {
		t.Fatalf("expected Nil, got %v", result.Value)
	}
}

func TestListLiteralFmap(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Prelude
main := fmap (\x. add x 10) [1, 2, 3]
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertList(t, result.Value, []int64{11, 12, 13})
}

// ---------------------------------------------------------------------------
// List Stdlib Pack (Group 1B)
// ---------------------------------------------------------------------------

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

// assertList checks that a Value is a List with the given int64 elements.
func assertList(t *testing.T, v eval.Value, expected []int64) {
	t.Helper()
	for i, want := range expected {
		con, ok := v.(*eval.ConVal)
		if !ok || con.Con != "Cons" {
			t.Fatalf("element %d: expected Cons, got %v", i, v)
		}
		if len(con.Args) != 2 {
			t.Fatalf("element %d: Cons has %d args, expected 2", i, len(con.Args))
		}
		hv, ok := con.Args[0].(*eval.HostVal)
		if !ok {
			t.Fatalf("element %d: expected HostVal, got %T", i, con.Args[0])
		}
		if hv.Inner != want {
			t.Fatalf("element %d: expected %d, got %v", i, want, hv.Inner)
		}
		v = con.Args[1]
	}
	con, ok := v.(*eval.ConVal)
	if !ok || con.Con != "Nil" {
		t.Fatalf("expected Nil at end, got %v", v)
	}
}

// --- Regression tests for defensive-copy and isolation fixes ---

func TestFromConDefensiveCopy(t *testing.T) {
	v := &eval.ConVal{Con: "Pair", Args: []eval.Value{ToValue(1), ToValue(2)}}
	_, args, ok := FromCon(v)
	if !ok {
		t.Fatal("expected ConVal")
	}
	// Mutate the returned slice.
	args[0] = ToValue(999)
	// Original must be unchanged.
	_, orig, _ := FromCon(v)
	if MustHost[int](orig[0]) != 1 {
		t.Error("FromCon returned slice aliases internal Args — mutation leaked")
	}
}

func TestFromRecordDefensiveCopy(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime("import Prelude\nmain := { a: 1, b: 2 }")
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	fields, ok := FromRecord(result.Value)
	if !ok {
		t.Fatal("expected record value")
	}
	// Mutate the returned map.
	fields["injected"] = ToValue(999)
	// Re-extract — original must not contain the injected key.
	fields2, _ := FromRecord(result.Value)
	if _, found := fields2["injected"]; found {
		t.Error("FromRecord returned map aliases internal Fields — mutation leaked")
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

// NOTE: Example API tests (TestExamplesReturnsNonEmpty, TestExampleReturnsSource,
// TestExampleNotFound, TestExampleRejectsPathSeparators) remain in the root
// package since they test gicel.Examples/gicel.Example which use embed.FS
// relative to the root module directory.
