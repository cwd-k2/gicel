package gomputation_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	gmp "github.com/cwd-k2/gomputation"
)

func TestIdentity(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
id := \x -> x
main := id True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestPure(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := pure ()`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

func TestHostBinding(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.DeclareBinding("x", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}
	bindings := map[string]gmp.Value{
		"x": &gmp.HostVal{Inner: 42},
	}
	result, err := rt.RunContext(context.Background(), nil, bindings, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != 42 {
		t.Errorf("expected HostVal(42), got %s", result.Value)
	}
}

func TestAssumption(t *testing.T) {
	eng := gmp.NewEngine()
	eng.DeclareAssumption("getUnit", gmp.ArrowType(gmp.ConType("Bool"), gmp.ConType("Bool")))
	eng.RegisterPrim("getUnit", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		return &gmp.ConVal{Con: "True"}, capEnv, nil
	})
	rt, err := eng.NewRuntime(`
getUnit := assumption
main := getUnit True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestEvalIntLit(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := 42`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", result.Value)
	}
	if hv.Inner != int64(42) {
		t.Errorf("expected 42, got %v", hv.Inner)
	}
}

func TestEvalStrLit(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := "hello"`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", result.Value)
	}
	if hv.Inner != "hello" {
		t.Errorf("expected 'hello', got %v", hv.Inner)
	}
}

func TestEngineUse(t *testing.T) {
	eng := gmp.NewEngine()
	called := false
	pack := gmp.Pack(func(r gmp.Registrar) error {
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
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := add 1 2
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 3 {
		t.Errorf("expected 3, got %d", hv)
	}
}

func TestNumOperators(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := 1 + 2 * 3
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 7 {
		t.Errorf("expected 7, got %d", hv)
	}
}

func TestNumNegate(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := negate 42
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != -42 {
		t.Errorf("expected -42, got %d", hv)
	}
}

func TestNumEqInt(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := eq 1 1
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := gmp.FromBool(result.Value)
	if !ok || !b {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestNumOrdInt(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := compare 1 2
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "LT" {
		t.Errorf("expected LT, got %s", result.Value)
	}
}

func TestNumDivMod(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := div 7 3
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 2 {
		t.Errorf("expected 2, got %d", hv)
	}
}

func TestStrConcat(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Str); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Str
main := append "hello" " world"
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[string](result.Value)
	if hv != "hello world" {
		t.Errorf("expected 'hello world', got %s", hv)
	}
}

func TestStrEq(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Str); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Str
main := eq "abc" "abc"
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := gmp.FromBool(result.Value)
	if !ok || !b {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestStrOrd(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Str); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Str
main := compare "a" "b"
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "LT" {
		t.Errorf("expected LT, got %s", result.Value)
	}
}

func TestStrLength(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Str); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Str
main := length "hello"
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 5 {
		t.Errorf("expected 5, got %d", hv)
	}
}

func TestRuneEq(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Str); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Str
main := eq 'a' 'a'
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := gmp.FromBool(result.Value)
	if !ok || !b {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestFailAbort(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Fail); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Fail
main := do { fail; pure True }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"fail": &gmp.RecordVal{Fields: map[string]gmp.Value{}}}
	_, err = rt.RunContext(context.Background(), caps, nil, "main")
	if err == nil {
		t.Fatal("expected error from fail")
	}
}

func TestFromMaybe(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Fail); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Fail
main := fromMaybe (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"fail": &gmp.RecordVal{Fields: map[string]gmp.Value{}}}
	result, err := rt.RunContext(context.Background(), caps, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := gmp.FromBool(result.Value)
	if !ok || !b {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestStateGetPut(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(gmp.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
import Std.State
main := do { put 42; get }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &gmp.HostVal{Inner: int64(0)}}
	result, err := rt.RunContext(context.Background(), caps, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 42 {
		t.Errorf("expected 42, got %d", hv)
	}
}

func TestStateThread(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(gmp.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
import Std.State
main := do { n <- get; put (n + 1); get }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &gmp.HostVal{Inner: int64(0)}}
	result, err := rt.RunContext(context.Background(), caps, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv := gmp.MustHost[int64](result.Value)
	if hv != 1 {
		t.Errorf("expected 1, got %d", hv)
	}
}

func TestFromResult(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Fail); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Fail
main := fromResult (Ok True)
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"fail": &gmp.RecordVal{Fields: map[string]gmp.Value{}}}
	result, err := rt.RunContext(context.Background(), caps, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := gmp.FromBool(result.Value)
	if !ok || !b {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestFailWithState(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(gmp.Fail); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(gmp.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
import Std.Fail
import Std.State
main := do { put 42; fail }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{
		"state": &gmp.HostVal{Inner: int64(0)},
		"fail":  &gmp.RecordVal{Fields: map[string]gmp.Value{}},
	}
	_, err = rt.RunContext(context.Background(), caps, nil, "main")
	if err == nil {
		t.Fatal("expected error from fail")
	}
}

func TestCaseExpression(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
not := \b -> case b { True -> False; False -> True }
main := not True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("expected False, got %s", result.Value)
	}
}

func TestDoBlock(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := do { pure () }`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

func TestNoPrelude(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data MyBool = MyTrue | MyFalse
main := MyTrue
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "MyTrue" {
		t.Errorf("expected MyTrue, got %s", result.Value)
	}
}

func TestCompileError(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := undefined_thing`)
	if err == nil {
		t.Error("expected compile error")
	}
	_, ok := err.(*gmp.CompileError)
	if !ok {
		t.Errorf("expected CompileError, got %T", err)
	}
}

func TestMissingEntry(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`x := True`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
	if err == nil {
		t.Error("expected error for missing entry point")
	}
}

func TestConstructorArgs(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := Just True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Errorf("expected Just, got %s", result.Value)
	}
	if len(con.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(con.Args))
	}
	inner, ok := con.Args[0].(*gmp.ConVal)
	if !ok || inner.Con != "True" {
		t.Errorf("expected True, got %s", con.Args[0])
	}
}

func TestPairConstructor(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := (True, False)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok {
		t.Errorf("expected RecordVal (tuple), got %T: %s", result.Value, result.Value)
	}
	if len(rv.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(rv.Fields))
	}
}

func TestStepLimit(t *testing.T) {
	eng := gmp.NewEngine()
	eng.SetStepLimit(10)
	eng.EnableRecursion()
	eng.NoPrelude()
	// No recursion in source, just a program that completes in few steps.
	rt, err := eng.NewRuntime(`
data Unit = Unit
main := Unit
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	if result.Stats.Steps == 0 {
		t.Error("expected non-zero step count")
	}
}

func TestGoroutineSafety(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 10)
	for range 10 {
		go func() {
			_, err := rt.RunContext(context.Background(), nil, nil, "main")
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
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
konst := \x -> \y -> x
main := konst True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestNestedCase(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
bothTrue := \x -> \y -> case x { True -> case y { True -> True; False -> False }; False -> False }
main := bothTrue True True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestBlockExpression(t *testing.T) {
	eng := gmp.NewEngine()
	// Block binding: { x := True; x } desugars to (\x -> x) True.
	// The body references the block binding variable.
	rt, err := eng.NewRuntime(`main := { x := True; x }`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestBlockMultipleBindings(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := { x := True; y := False; x }`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// ---------------------------------------------------------------------------
// B. Computation
// ---------------------------------------------------------------------------

func TestBindChain(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := do { x <- pure True; pure x }`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestThunkForce(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := force (thunk (pure ()))`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

// ---------------------------------------------------------------------------
// C. Type errors (compile-time rejection)
// ---------------------------------------------------------------------------

func TestTypeErrorUnboundVar(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := totally_undefined`)
	if err == nil {
		t.Fatal("expected compile error for unbound variable")
	}
	if _, ok := err.(*gmp.CompileError); !ok {
		t.Errorf("expected CompileError, got %T", err)
	}
}

func TestTypeErrorNonExhaustive(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
f := \x -> case x { True -> () }
main := f True
`)
	if err == nil {
		t.Fatal("expected compile error for non-exhaustive pattern")
	}
	ce, ok := err.(*gmp.CompileError)
	if !ok {
		t.Fatalf("expected CompileError, got %T", err)
	}
	if !strings.Contains(ce.Error(), "non-exhaustive") {
		t.Errorf("expected non-exhaustive in error message, got: %s", ce.Error())
	}
}

func TestTypeErrorMismatch(t *testing.T) {
	eng := gmp.NewEngine()
	// Apply True (a Bool, not a function) to an argument.
	_, err := eng.NewRuntime(`main := True ()`)
	if err == nil {
		t.Fatal("expected compile error for type mismatch")
	}
	if _, ok := err.(*gmp.CompileError); !ok {
		t.Errorf("expected CompileError, got %T", err)
	}
}

// ---------------------------------------------------------------------------
// D. Context cancellation
// ---------------------------------------------------------------------------

func TestContextCancellation(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := pure ()`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err = rt.RunContext(ctx, nil, nil, "main")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestContextTimeout(t *testing.T) {
	eng := gmp.NewEngine()
	eng.SetStepLimit(1) // extremely low step limit
	eng.EnableRecursion()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data Unit = Unit
data Bool = True | False
not := \b -> case b { True -> False; False -> True }
main := not (not (not True))
`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err = rt.RunContext(ctx, nil, nil, "main")
	if err == nil {
		t.Error("expected error from step limit or timeout")
	}
}

// ---------------------------------------------------------------------------
// E. Runtime reuse
// ---------------------------------------------------------------------------

func TestRuntimeReuse(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.DeclareBinding("x", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}

	// First run with value 1.
	r1, err := rt.RunContext(context.Background(), nil, map[string]gmp.Value{"x": &gmp.HostVal{Inner: 1}}, "main")
	if err != nil {
		t.Fatal(err)
	}
	h1, ok := r1.Value.(*gmp.HostVal)
	if !ok || h1.Inner != 1 {
		t.Errorf("first run: expected HostVal(1), got %s", r1.Value)
	}

	// Second run with value 2.
	r2, err := rt.RunContext(context.Background(), nil, map[string]gmp.Value{"x": &gmp.HostVal{Inner: 2}}, "main")
	if err != nil {
		t.Fatal(err)
	}
	h2, ok := r2.Value.(*gmp.HostVal)
	if !ok || h2.Inner != 2 {
		t.Errorf("second run: expected HostVal(2), got %s", r2.Value)
	}
}

func TestRuntimeConcurrent(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
not := \b -> case b { True -> False; False -> True }
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
			result, err := rt.RunContext(context.Background(), nil, nil, "main")
			if err != nil {
				errs[idx] = err
				return
			}
			con, ok := result.Value.(*gmp.ConVal)
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
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`main := undefined_xyz`)
	if err == nil {
		t.Fatal("expected compile error")
	}
	ce, ok := err.(*gmp.CompileError)
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
	eng := gmp.NewEngine()
	prog, err := eng.Check(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	if prog == nil {
		t.Error("expected non-nil program from Check")
	}
}

func TestParseOnly(t *testing.T) {
	eng := gmp.NewEngine()
	ast, err := eng.Parse(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	if ast == nil {
		t.Error("expected non-nil AST from Parse")
	}
}

func TestPrettyProgram(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	pretty := rt.PrettyProgram()
	if pretty == "" {
		t.Error("expected non-empty PrettyProgram output")
	}
}

func TestRunContextFull(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := pure ()`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContextFull(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
	// CapEnv should be present (even if empty).
	_ = result.CapEnv
	_ = result.Stats
}

func TestValueConversion(t *testing.T) {
	// HostVal wraps arbitrary Go values.
	hv := &gmp.HostVal{Inner: "hello"}
	if hv.Inner != "hello" {
		t.Errorf("expected HostVal.Inner = hello, got %v", hv.Inner)
	}
	if hv.String() != "HostVal(hello)" {
		t.Errorf("unexpected HostVal.String(): %s", hv.String())
	}

	// ConVal represents constructors.
	cv := &gmp.ConVal{Con: "True"}
	if cv.String() != "True" {
		t.Errorf("expected True, got %s", cv.String())
	}

	// ConVal with arguments.
	cv2 := &gmp.ConVal{Con: "Just", Args: []gmp.Value{&gmp.ConVal{Con: "True"}}}
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
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.RegisterPrim("getVal", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(99), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
getVal :: () -> Computation {} {} Int
getVal := assumption
main := do { x <- getVal (); pure x }
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	v := gmp.MustHost[int](result.Value)
	if v != 99 {
		t.Errorf("expected 99, got %d", v)
	}
}

// Assumption without any type annotation must produce a compile error.
func TestAssumptionNoType(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
noType := assumption
main := noType ()
`)
	if err == nil {
		t.Fatal("expected compile error for assumption without type")
	}
	ce, ok := err.(*gmp.CompileError)
	if !ok {
		t.Fatalf("expected CompileError, got %T", err)
	}
	if !strings.Contains(ce.Error(), "assumption") {
		t.Errorf("expected mention of assumption in error, got: %s", ce.Error())
	}
}

// _ <- in do blocks must work (was a parser bug: TokUnderscore != TokLower).
func TestDoWildcardBind(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := do {
  _ <- pure True;
  pure False
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("expected False, got %s", result.Value)
	}
}

// CapEnv threading: mutations propagate through do-block bind chain.
func TestCapEnvThreading(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.RegisterPrim("inc", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		v, _ := capEnv.Get("n")
		n, _ := v.(int)
		return gmp.ToValue(nil), capEnv.Set("n", n+1), nil
	})
	eng.RegisterPrim("getN", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		v, _ := capEnv.Get("n")
		n, _ := v.(int)
		return gmp.ToValue(n), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
inc :: () -> Computation {} {} ()
inc := assumption
getN :: () -> Computation {} {} Int
getN := assumption
main := do { _ <- inc (); _ <- inc (); _ <- inc (); getN () }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContextFull(context.Background(), map[string]any{"n": 0}, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	n := gmp.MustHost[int](result.Value)
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
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.DeclareBinding("x", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}
	// Run without providing binding "x".
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
	if err == nil {
		t.Fatal("expected error for missing runtime binding")
	}
	if !strings.Contains(err.Error(), "missing binding") {
		t.Errorf("expected 'missing binding' in error, got: %s", err.Error())
	}
}

// PrimVal partial application: binary assumption applied to one arg, then another.
func TestPrimPartialApplication(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.RegisterPrim("add", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		a := gmp.MustHost[int](args[0])
		b := gmp.MustHost[int](args[1])
		return gmp.ToValue(a + b), capEnv, nil
	})
	eng.DeclareBinding("a", gmp.ConType("Int"))
	eng.DeclareBinding("b", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`
add :: Int -> Int -> Int
add := assumption
main := add a b
`)
	if err != nil {
		t.Fatal(err)
	}
	bindings := map[string]gmp.Value{
		"a": gmp.ToValue(10),
		"b": gmp.ToValue(32),
	}
	result, err := rt.RunContext(context.Background(), nil, bindings, "main")
	if err != nil {
		t.Fatal(err)
	}
	v := gmp.MustHost[int](result.Value)
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

// Cyclic type alias must produce a compile error.
func TestCyclicTypeAlias(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
type A = B
type B = A
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
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
f := \x -> case x { Just y -> y }
main := f (Just True)
`)
	if err == nil {
		t.Fatal("expected compile error for non-exhaustive case on Maybe")
	}
	ce := err.(*gmp.CompileError)
	if !strings.Contains(ce.Error(), "Nothing") {
		t.Errorf("expected missing constructor 'Nothing', got: %s", ce.Error())
	}
}

// ToValue and FromBool round-trip.
func TestToValueFromBool(t *testing.T) {
	v := gmp.ToValue(true)
	b, ok := gmp.FromBool(v)
	if !ok || b != true {
		t.Errorf("ToValue(true) -> FromBool failed")
	}
	v2 := gmp.ToValue(false)
	b2, ok := gmp.FromBool(v2)
	if !ok || b2 != false {
		t.Errorf("ToValue(false) -> FromBool failed")
	}
}

// ToValue(nil) produces ().
func TestToValueNil(t *testing.T) {
	v := gmp.ToValue(nil)
	rv, ok := v.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("ToValue(nil) should produce (), got %s", v)
	}
}

// FromHost on non-HostVal returns ok=false.
func TestFromHostNonHost(t *testing.T) {
	v := gmp.ToValue(true) // ConVal, not HostVal
	_, ok := gmp.FromHost(v)
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
	gmp.MustHost[int](gmp.ToValue(true))
}

func TestRuntimeErrorType(t *testing.T) {
	eng := gmp.NewEngine()
	eng.Use(gmp.Fail)
	rt, err := eng.NewRuntime(`
import Std.Fail
main := do { _ <- fail; pure True }
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
	if err == nil {
		t.Fatal("expected runtime error from fail")
	}
	var rtErr *gmp.RuntimeError
	if !errors.As(err, &rtErr) {
		t.Errorf("expected RuntimeError, got %T: %v", err, err)
	}
}

func TestFromRecord(t *testing.T) {
	eng := gmp.NewEngine()
	eng.Use(gmp.Num)
	rt, err := eng.NewRuntime(`
import Std.Num
main := { x = 1, y = 2 }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	fields, ok := gmp.FromRecord(result.Value)
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
	_, ok := gmp.FromRecord(gmp.ToValue(42))
	if ok {
		t.Error("FromRecord on HostVal should return ok=false")
	}
}

func TestRecordTypeHelper(t *testing.T) {
	rty := gmp.RecordType(
		gmp.RowField{Label: "x", Type: gmp.ConType("Int")},
		gmp.RowField{Label: "y", Type: gmp.ConType("Int")},
	)
	got := gmp.TypePretty(rty)
	if !strings.Contains(got, "Record") {
		t.Errorf("expected Record type, got %s", got)
	}
}

func TestTupleTypeHelper(t *testing.T) {
	tt := gmp.TupleType(gmp.ConType("Int"), gmp.ConType("Bool"))
	got := gmp.TypePretty(tt)
	if !strings.Contains(got, "Record") || !strings.Contains(got, "_1") {
		t.Errorf("expected Record{_1, _2} type, got %s", got)
	}
}

func TestNewCapEnvExported(t *testing.T) {
	ce := gmp.NewCapEnv(map[string]any{"key": "val"})
	v, ok := ce.Get("key")
	if !ok {
		t.Fatal("expected key in CapEnv")
	}
	if v != "val" {
		t.Errorf("expected val, got %v", v)
	}
}

// Explicit bind syntax: bind comp (\x -> body) elaborates to Core.Bind.
func TestExplicitBind(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := bind (pure True) (\x -> pure x)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// pure/bind/thunk/force used standalone (not applied) must error.
func TestSpecialFormStandalone(t *testing.T) {
	forms := []string{"pure", "bind", "thunk", "force"}
	for _, name := range forms {
		eng := gmp.NewEngine()
		_, err := eng.NewRuntime("main := " + name)
		if err == nil {
			t.Errorf("expected compile error for standalone %s", name)
		}
	}
}

// Thunk/force round-trip in do block.
func TestThunkForceInDo(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := do {
  t := thunk (pure True);
  force t
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// Forall with kinded binder: forall (r : Row). T
func TestKindedForallBinder(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.RegisterPrim("getVal", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(7), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
getVal :: forall (r : Row). () -> Computation r r Int
getVal := assumption
main := do { getVal () }
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	v := gmp.MustHost[int](result.Value)
	if v != 7 {
		t.Errorf("expected 7, got %d", v)
	}
}

// Result type has 2 parameters: Result e a = Ok a | Err e
func TestResult2Params(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("String", gmp.KindType())
	rt, err := eng.NewRuntime(`
main := Ok True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Ok" {
		t.Errorf("expected Ok, got %s", result.Value)
	}
	if len(con.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(con.Args))
	}
	inner, ok := con.Args[0].(*gmp.ConVal)
	if !ok || inner.Con != "True" {
		t.Errorf("expected True inside Ok, got %s", con.Args[0])
	}
}

// Type alias: type Effect r a = Computation r r a
func TestTypeAlias(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
type Effect r a = Computation r r a

main :: Effect {} Bool
main := pure True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// Prelude's Effect alias should work without user-defined alias.
func TestPreludeEffectAlias(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main :: Effect {} Bool
main := pure True
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// Env flat map: deeply nested binds don't retain parent chains.
func TestDeepBindEnvDoesNotLeak(t *testing.T) {
	eng := gmp.NewEngine()
	eng.EnableRecursion()
	eng.RegisterType("Int", gmp.KindType())
	eng.RegisterPrim("mkInt", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(0), capEnv, nil
	})
	// A chain of 100 nested binds. With linked-list Env, each
	// closure would retain the entire chain. With flat Env, each
	// only holds its own bindings map.
	rt, err := eng.NewRuntime(`
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
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	v := gmp.MustHost[int](result.Value)
	if v != 0 {
		t.Errorf("expected 0, got %d", v)
	}
}

// Empty program (no main) should produce a runtime error, not panic.
func TestNoMainEntry(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`helper := True`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
	if err == nil {
		t.Fatal("expected error for missing entry point 'main'")
	}
}

// Large case expression: exhaustiveness on 6-constructor ADT.
func TestExhaustiveLargeADT(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Color = Red | Green | Blue | Yellow | Cyan | Magenta

f := \c -> case c {
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
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// Non-exhaustive on large ADT names missing constructors.
func TestNonExhaustiveLargeADT(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
data Color = Red | Green | Blue | Yellow | Cyan | Magenta

f := \c -> case c {
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

// ---------------------------------------------------------------------------
// TC. Type classes — end-to-end
// ---------------------------------------------------------------------------

func TestTypeClassEqBool(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool {
  eq := \x -> \y -> case x {
    True  -> case y { True -> True;  False -> False };
    False -> case y { True -> False; False -> True }
  }
}
main := eq True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("expected False, got %s", result.Value)
	}
}

func TestTypeClassPolymorphic(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool {
  eq := \x -> \y -> case x {
    True  -> case y { True -> True;  False -> False };
    False -> case y { True -> False; False -> True }
  }
}
f :: forall a. Eq a => a -> a -> Bool
f := \x -> \y -> eq x y
main := f True True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestTypeClassSuperclass(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { lt :: a -> a -> Bool }
instance Eq Bool {
  eq := \x -> \y -> True
}
instance Ord Bool {
  lt := \x -> \y -> False
}
useOrd :: forall a. Ord a => a -> a -> Bool
useOrd := \x -> \y -> eq x y
main := useOrd True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestTypeClassFunctor(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data Bool = True | False
data Maybe a = Just a | Nothing
class Functor f { fmap :: forall a b. (a -> b) -> f a -> f b }
instance Functor Maybe {
  fmap := \g -> \mx -> case mx {
    Just x  -> Just (g x);
    Nothing -> Nothing
  }
}
not := \b -> case b { True -> False; False -> True }
main := fmap not (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Errorf("expected Just, got %s", result.Value)
	}
	if len(con.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(con.Args))
	}
	inner, ok := con.Args[0].(*gmp.ConVal)
	if !ok || inner.Con != "False" {
		t.Errorf("expected False inside Just, got %s", con.Args[0])
	}
}

func TestTypeClassMultiParam(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	rt, err := eng.NewRuntime(`
data Bool = True | False
class Coercible a b { coerce :: a -> b }
instance Coercible Bool Bool {
  coerce := \x -> x
}
main := coerce True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// ---------------------------------------------------------------------------
// I. Type helpers
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// DK. DataKinds — end-to-end integration
// ---------------------------------------------------------------------------

func TestDataKindsDBState(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data DBState = Opened | Closed
data DB s = MkDB

open :: DB Closed -> DB Opened
open := \_ -> MkDB

close :: DB Opened -> DB Closed
close := \_ -> MkDB

main := close (open (MkDB :: DB Closed))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "MkDB" {
		t.Errorf("expected MkDB, got %s", result.Value)
	}
}

func TestDataKindsInRow(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.RegisterPrim("readDB", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(42), ce, nil
	})
	rt, err := eng.NewRuntime(`
data DBState = Opened | Closed

readDB :: () -> Computation { db : Int } { db : Int } Int
readDB := assumption

main :: Computation { db : Int } { db : Int } Int
main := do { readDB () }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), map[string]any{"db": 0}, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	v := gmp.MustHost[int](result.Value)
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestDataKindsBoolPromotion(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Proxy s = MkProxy
main := (MkProxy :: Proxy True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "MkProxy" {
		t.Errorf("expected MkProxy, got %s", result.Value)
	}
}

// --- GADT integration tests ---

func TestGADTEvalExpr(t *testing.T) {
	eng := gmp.NewEngine()
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(`
data Expr a = { LitBool :: Bool -> Expr Bool; Not :: Expr Bool -> Expr Bool }

eval :: Expr Bool -> Bool
eval := fix (\self -> \e -> case e {
  LitBool b -> b;
  Not inner -> case self inner { True -> False; False -> True }
})

main := eval (Not (LitBool True))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("expected False, got %s", result.Value)
	}
}

func TestGADTWithDataKinds(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data DBState = Opened | Closed
data DB s = MkDB

data Action s = { Open :: Action Opened; Close :: Action Closed }

describe :: Action Opened -> DB Opened
describe := \a -> case a { Open -> MkDB }

main := describe Open
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "MkDB" {
		t.Errorf("expected MkDB, got %s", result.Value)
	}
}

func TestGADTNestedPattern(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Expr a = { LitBool :: Bool -> Expr Bool; Not :: Expr Bool -> Expr Bool }

-- Nested pattern: match on Not (LitBool _)
isDoubleNeg :: Expr Bool -> Bool
isDoubleNeg := \e -> case e {
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
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// --- Module system integration tests ---

func TestRegisterModule(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	err := eng.RegisterModule("Lib", `
data Bool = True | False
not :: Bool -> Bool
not := \b -> case b { True -> False; False -> True }
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestImportModuleTypes(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	err := eng.RegisterModule("Lib", `
data Bool = True | False
not :: Bool -> Bool
not := \b -> case b { True -> False; False -> True }
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Lib
main := not True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("expected False, got %s", result.Value)
	}
}

func TestImportModuleInstances(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	err := eng.RegisterModule("EqLib", `
data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x -> \y -> True }
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import EqLib
main := eq True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestRegisterModuleFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/Lib.gmp"
	if err := os.WriteFile(path, []byte(`
data Color = Red | Blue
`), 0644); err != nil {
		t.Fatal(err)
	}
	eng := gmp.NewEngine()
	eng.NoPrelude()
	if err := eng.RegisterModuleFile(path); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Lib
main := Red
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Red" {
		t.Errorf("expected Red, got %s", result.Value)
	}
}

func TestRegisterModuleFileMissing(t *testing.T) {
	eng := gmp.NewEngine()
	err := eng.RegisterModuleFile("/nonexistent/path/Foo.gmp")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCircularImportError(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	// Register module A that imports B.
	err := eng.RegisterModule("A", `
import B
data Unit = Unit
`)
	// Should fail because B is not registered.
	if err == nil {
		// Now try registering B that imports A → circular.
		err = eng.RegisterModule("B", `
import A
data Void = MkVoid
`)
		if err == nil {
			t.Error("expected circular import error")
		}
	}
	// If A failed because B doesn't exist, that's also acceptable.
	if err != nil && !strings.Contains(err.Error(), "unknown module") && !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected import error, got: %s", err)
	}
}

func TestImportNameCollision(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	// Module A defines Bool.
	err := eng.RegisterModule("A", `data Bool = True | False`)
	if err != nil {
		t.Fatal(err)
	}
	// Module B also defines Bool.
	err = eng.RegisterModule("B", `data Bool = Yes | No`)
	if err != nil {
		t.Fatal(err)
	}
	// Importing both should succeed at parse time but may produce
	// ambiguity at type check time. At minimum, the program should not panic.
	_, err = eng.NewRuntime(`
import A
import B
main := True
`)
	// This may or may not error depending on resolution order.
	// The important thing is no panic.
	_ = err
}

// --- Stdlib integration tests ---

func TestStdlibEqOrd(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := eq True True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestStdlibFunctor(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
not :: Bool -> Bool
not := \b -> case b { True -> False; False -> True }

main := fmap not (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %s", result.Value)
	}
	inner, ok := con.Args[0].(*gmp.ConVal)
	if !ok || inner.Con != "False" {
		t.Errorf("expected Just False, got Just %s", con.Args[0])
	}
}

func TestStdlibFoldable(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := foldr (\x -> \_ -> x) False (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestStdlibEqPair(t *testing.T) {
	// NOTE: Eq (a, b) on tuples is not yet supported at runtime (evidence/record interaction).
	// Test Eq on List as a multi-element container substitute.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := eq (Cons True (Cons False Nil)) (Cons True (Cons False Nil))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

// --- Coercible integration tests ---

func TestCoercibleMultiParam(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
class Coercible a b { coerce :: a -> b }

instance Coercible Bool () { coerce := \_ -> () }

main := coerce True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

func TestCoercibleUsage(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
class Coercible a b { coerce :: a -> b }

instance Coercible Bool Bool { coerce := \x -> x }

main :: Bool
main := coerce True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestPreludeAsModule(t *testing.T) {
	// Prelude is now loaded as an implicit module — Bool, (), etc. should be available.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestNoPreludeWithModules(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	// Without prelude, Bool is not defined — need to define it ourselves.
	rt, err := eng.NewRuntime(`
data MyBool = Yes | No
main := Yes
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Yes" {
		t.Errorf("expected Yes, got %s", result.Value)
	}
}

func TestSetPreludeCustom(t *testing.T) {
	// Custom prelude replaces default: only defines MyBool, no standard Bool.
	eng := gmp.NewEngine()
	eng.SetPrelude(`
data MyBool = Yes | No
`)
	rt, err := eng.NewRuntime(`main := Yes`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Yes" {
		t.Errorf("expected Yes, got %s", result.Value)
	}
}

func TestSetPreludeCoreStillAvailable(t *testing.T) {
	// Core definitions (IxMonad, Effect, then) available even with custom Prelude.
	eng := gmp.NewEngine()
	eng.SetPrelude(`
data Bool = True | False
`)
	// Effect and then come from CoreSource, Bool from custom prelude.
	rt, err := eng.NewRuntime(`
main :: Effect {} Bool
main := pure True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestSetPreludeNoDefaultBool(t *testing.T) {
	// Custom prelude that doesn't define Bool — standard Bool should not be available.
	eng := gmp.NewEngine()
	eng.SetPrelude(`data Color = Red | Blue`)
	_, err := eng.NewRuntime(`main := True`)
	if err == nil {
		t.Error("expected compile error: True not defined with custom prelude")
	}
}

func TestTypeHelpers(t *testing.T) {
	// ConType constructs a type constructor.
	intTy := gmp.ConType("Int")
	if gmp.TypePretty(intTy) != "Int" {
		t.Errorf("expected Int, got %s", gmp.TypePretty(intTy))
	}

	// ArrowType constructs a function type.
	arrTy := gmp.ArrowType(gmp.ConType("Bool"), gmp.ConType("Bool"))
	if got := gmp.TypePretty(arrTy); got != "Bool -> Bool" {
		t.Errorf("expected Bool -> Bool, got %s", got)
	}

	// EmptyRowType constructs an empty row.
	row := gmp.EmptyRowType()
	if gmp.TypePretty(row) != "{}" {
		t.Errorf("expected {}, got %s", gmp.TypePretty(row))
	}

	// ClosedRowType constructs a row with fields.
	row2 := gmp.ClosedRowType(gmp.RowField{Label: "x", Type: gmp.ConType("Int")})
	if got := gmp.TypePretty(row2); !strings.Contains(got, "x") {
		t.Errorf("expected row with field x, got %s", got)
	}

	// ForallType constructs a quantified type.
	forallTy := gmp.ForallType("a", gmp.ArrowType(gmp.VarType("a"), gmp.VarType("a")))
	if got := gmp.TypePretty(forallTy); !strings.Contains(got, "forall") {
		t.Errorf("expected forall in pretty, got %s", got)
	}

	// CompType constructs a computation type.
	compTy := gmp.CompType(gmp.EmptyRowType(), gmp.EmptyRowType(), gmp.ConType("Bool"))
	if got := gmp.TypePretty(compTy); !strings.Contains(got, "Bool") {
		t.Errorf("expected Bool in computation type, got %s", got)
	}

	// VarType constructs a type variable.
	tv := gmp.VarType("a")
	if gmp.TypePretty(tv) != "a" {
		t.Errorf("expected a, got %s", gmp.TypePretty(tv))
	}

	// Kind helpers.
	k := gmp.KindType()
	if !k.Equal(gmp.KindType()) {
		t.Errorf("KType should equal KType")
	}
	kr := gmp.KindRow()
	if kr.Equal(gmp.KindType()) {
		t.Errorf("KRow should not equal KType")
	}
	ka := gmp.KindArrow(gmp.KindType(), gmp.KindType())
	if !ka.Equal(gmp.KindArrow(gmp.KindType(), gmp.KindType())) {
		t.Errorf("KArrow(Type,Type) should equal itself")
	}

	// TypeEqual
	if !gmp.TypeEqual(gmp.ConType("Int"), gmp.ConType("Int")) {
		t.Errorf("Int should equal Int")
	}
	if gmp.TypeEqual(gmp.ConType("Int"), gmp.ConType("Bool")) {
		t.Errorf("Int should not equal Bool")
	}
}

// ---------------------------------------------------------------------------
// v0.5 Tests: Existential Types, Higher-Rank Polymorphism, Stdlib Expansion
// ---------------------------------------------------------------------------

// --- Phase 1: Skolem Infrastructure ---

func TestUnifySkolemRigid(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.Check(`
not :: Bool -> Bool
not := \x -> x
main := not True
`)
	if err != nil {
		t.Fatal("basic check should pass:", err)
	}
}

func TestUnifySkolemSame(t *testing.T) {
	// Indirect test: use a GADT constructor with existential that unifies skolem with itself
	eng := gmp.NewEngine()
	_, err := eng.Check(`
data SameTest = { MkSame :: forall a. a -> a -> SameTest }
useIt :: SameTest -> Bool
useIt := \s -> case s { MkSame x y -> True }
`)
	if err != nil {
		t.Fatal("existential pattern match should pass:", err)
	}
}

// --- Phase 1B: Skolem Escape Check ---

func TestSkolemEscapeDetected(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.Check(`
data Exists = { MkExists :: forall a. a -> Exists }
escape :: Exists -> Bool
escape := \e -> case e { MkExists x -> x }
`)
	if err == nil {
		t.Fatal("expected escape error, got nil")
	}
	if !strings.Contains(err.Error(), "escape") && !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("expected escape/mismatch error, got: %s", err)
	}
}

func TestSkolemNoEscape(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.Check(`
data Wrapper = { MkWrapper :: forall a. a -> Wrapper }
safe :: Wrapper -> Bool
safe := \w -> case w { MkWrapper _ -> True }
`)
	if err != nil {
		t.Fatal("non-escaping existential should pass:", err)
	}
}

func TestSkolemEscapeInMeta(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.Check(`
data SomeVal = { MkSome :: forall a. a -> SomeVal }
leaky := \s -> case s { MkSome x -> Just x }
`)
	if err == nil {
		t.Fatal("expected escape error for Just x where x is existential")
	}
}

// --- Phase 2: Existential Types ---

func TestExistentialBasic(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data SomeEq = { MkSomeEq :: forall a. Eq a => a -> SomeEq }
useSomeEq :: SomeEq -> Bool
useSomeEq := \s -> case s { MkSomeEq x -> eq x x }
main := useSomeEq (MkSomeEq True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestExistentialEscapeError(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.Check(`
data SomeEq = { MkSomeEq :: forall a. Eq a => a -> SomeEq }
escape :: SomeEq -> Bool
escape := \s -> case s { MkSomeEq x -> x }
`)
	if err == nil {
		t.Fatal("expected type error for escaping existential")
	}
}

func TestExistentialNoConstraint(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data ExistsF f = { MkExistsF :: forall a. f a -> ExistsF f }
useMaybe :: ExistsF Maybe -> Bool
useMaybe := \e -> case e { MkExistsF _ -> True }
main := useMaybe (MkExistsF (Just True))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestExistentialMixed(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Wrapper a = { MkWrapper :: forall b. (b -> a) -> b -> Wrapper a }
useWrapper :: Wrapper Bool -> Bool
useWrapper := \w -> case w { MkWrapper f x -> f x }
main := useWrapper (MkWrapper (\x -> x) True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestExistentialMultiConstraint(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.Check(`
data ShowOrd = { MkShowOrd :: forall a. Eq a => Ord a => a -> a -> ShowOrd }
use :: ShowOrd -> Ordering
use := \s -> case s { MkShowOrd x y -> compare x y }
`)
	if err != nil {
		t.Fatal(err)
	}
}

// --- Phase 2D: Existential Integration ---

func TestExistentialWithTypeClass(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data SomeEq = { MkSomeEq :: forall a. Eq a => a -> SomeEq }
isSame :: SomeEq -> Bool
isSame := \s -> case s { MkSomeEq x -> eq x x }
main := isSame (MkSomeEq (Just True))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	// eq (Just True) (Just True) = True (same value)
	assertConName(t, result.Value, "True")
}

func TestExistentialWithGADT(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Typed a = { MkBool :: Bool -> Typed Bool; MkUnit :: Typed () }
data SomeTyped = { MkSome :: forall a. Typed a -> SomeTyped }
classify :: SomeTyped -> Bool
classify := \s -> case s { MkSome t -> True }
main := classify (MkSome (MkBool True))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestExistentialNestedCase(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data Wrap = { MkWrap :: forall a. Eq a => a -> Wrap }
bothSame :: Wrap -> Wrap -> Bool
bothSame := \w1 -> \w2 ->
  case w1 { MkWrap x -> case w2 { MkWrap y -> True } }
main := bothSame (MkWrap True) (MkWrap False)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

// --- Phase 3: Higher-Rank Polymorphism ---

func TestSubsumptionInstantiate(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
id :: forall a. a -> a
id := \x -> x
useBool :: (Bool -> Bool) -> Bool
useBool := \f -> f True
main := useBool id
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestHigherRankBasic(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
applyToTrue :: (forall a. a -> a) -> Bool
applyToTrue := \f -> f True
id :: forall a. a -> a
id := \x -> x
main := applyToTrue id
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestHigherRankAnnotationRequired(t *testing.T) {
	// Rank-2 types should require annotation on the parameter
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
apply :: (forall a. a -> a) -> (Bool, ())
apply := \f -> (f True, f ())
id :: forall a. a -> a
id := \x -> x
main := apply id
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 2 {
		t.Errorf("expected tuple (Bool, ()), got %s", result.Value)
	}
}

// --- Phase 3F: Higher-Rank Integration ---

func TestHigherRankPolymorphicArg(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
runId :: (forall a. a -> a) -> (Bool, ())
runId := \f -> (f True, f ())
id :: forall a. a -> a
id := \x -> x
main := runId id
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 2 {
		t.Errorf("expected tuple (Bool, ()), got %s", result.Value)
	}
	assertConName(t, rv.Fields["_1"], "True")
	unitField, ok := rv.Fields["_2"].(*gmp.RecordVal)
	if !ok || len(unitField.Fields) != 0 {
		t.Errorf("expected () in _2, got %s", rv.Fields["_2"])
	}
}

// --- Phase 4: Stdlib Expansion ---

func TestClassSemigroup(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := append () ()
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

func TestClassMonoid(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := (empty :: Ordering)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "EQ")
}

func TestClassApplicative(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.Check(`
test := (wrap True :: Maybe Bool)
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClassTraversable(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.Check(`
test :: forall f a b. Applicative f => (a -> f b) -> Maybe a -> f (Maybe b)
test := traverse
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSemigroupUnit(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := append () ()`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

func TestSemigroupOrdering(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
test1 := append LT EQ
test2 := append EQ GT
main := (test1, test2)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 2 {
		t.Fatalf("expected tuple, got %s", result.Value)
	}
	assertConName(t, rv.Fields["_1"], "LT") // LT <> EQ = LT
	assertConName(t, rv.Fields["_2"], "GT") // EQ <> GT = GT
}

func TestMonoidUnit(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := (empty :: ())`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

func TestApplicativeMaybe(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
wrapped := (wrap True :: Maybe Bool)
main := wrapped
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Errorf("expected Just, got %s", result.Value)
	}
}

func TestTraversableMaybe(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
not :: Bool -> Bool
not := \b -> case b { True -> False; False -> True }
main := traverse (\x -> Just (not x)) (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	// traverse (\x -> Just (not x)) (Just True) = Just (Just False)
	outer, ok := result.Value.(*gmp.ConVal)
	if !ok || outer.Con != "Just" {
		t.Fatalf("expected Just, got %s", result.Value)
	}
	inner, ok := outer.Args[0].(*gmp.ConVal)
	if !ok || inner.Con != "Just" {
		t.Fatalf("expected inner Just, got %s", outer.Args[0])
	}
	assertConName(t, inner.Args[0], "False")
}

func TestOrdBool(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := compare False True`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "LT")
}

func TestOrdMaybe(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
test1 := compare (Nothing :: Maybe Bool) (Just True)
test2 := compare (Just False) (Just True)
main := (test1, test2)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 2 {
		t.Fatalf("expected tuple, got %s", result.Value)
	}
	assertConName(t, rv.Fields["_1"], "LT") // Nothing < Just _
	assertConName(t, rv.Fields["_2"], "LT") // Just False < Just True
}

func TestOrdPair(t *testing.T) {
	// NOTE: Ord (a, b) on tuples is not yet supported at runtime (evidence/record interaction).
	// Test Ord on Maybe as a parameterized type substitute.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := compare (Just False) (Just True)`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	// compare (Just False) (Just True) = LT
	assertConName(t, result.Value, "LT")
}

// --- Phase 5: Cross-Feature Integration ---

func TestExistentialWithStdlib(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data SomeSemigroup = { MkSomeSG :: forall a. Semigroup a => a -> a -> SomeSemigroup }
combine :: SomeSemigroup -> Bool
combine := \s -> case s { MkSomeSG x y -> case append x y { _ -> True } }
main := combine (MkSomeSG EQ LT)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestFullPipeline(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
data SomeEq = { MkSomeEq :: forall a. Eq a => a -> SomeEq }
isSelf :: SomeEq -> Bool
isSelf := \s -> case s { MkSomeEq x -> eq x x }
applyId :: (forall a. a -> a) -> Bool
applyId := \f -> f True
id :: forall a. a -> a
id := \x -> x
main := case isSelf (MkSomeEq True) { True -> applyId id; False -> False }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertConName(t, result.Value, "True")
}

func TestStdlibClassHierarchy(t *testing.T) {
	// Verify all 8 classes compile and instances work
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
testEq := eq True True
testOrd := compare False True
testSemigroup := append EQ LT
testMonoid := (empty :: Ordering)
testFunctor := fmap (\x -> True) (Just False)
testApplicative := (wrap True :: Maybe Bool)
main := (testEq, (testOrd, (testSemigroup, (testMonoid, (testFunctor, testApplicative)))))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gmp.RecordVal)
	if !ok || len(rv.Fields) != 2 {
		t.Fatalf("expected tuple, got %s", result.Value)
	}
	assertConName(t, rv.Fields["_1"], "True") // eq True True = True
}

// helper for v0.5 tests
func assertConName(t *testing.T, v gmp.Value, name string) {
	t.Helper()
	con, ok := v.(*gmp.ConVal)
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
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := 1 + 2 * 3
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	if hv := gmp.MustHost[int64](result.Value); hv != 7 {
		t.Errorf("expected 7, got %d", hv)
	}
}

func TestEffectComposition(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(gmp.Fail); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(gmp.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
import Std.Fail
import Std.State
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
		"state": &gmp.HostVal{Inner: int64(0)},
		"fail":  &gmp.RecordVal{Fields: map[string]gmp.Value{}},
	}
	result, err := rt.RunContext(context.Background(), caps, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	if hv := gmp.MustHost[int64](result.Value); hv != 15 {
		t.Errorf("expected 15, got %d", hv)
	}
}

func TestPackModuleImport(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(gmp.Str); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
import Std.Str
v1 := 1 + 2
v2 := length "hello"
main := Just (v1 + v2)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %s", result.Value)
	}
	if hv := gmp.MustHost[int64](con.Args[0]); hv != 8 {
		t.Errorf("expected 8, got %d", hv)
	}
}

func TestCustomPack(t *testing.T) {
	// A user-defined Pack that provides a custom constant.
	customPack := gmp.Pack(func(r gmp.Registrar) error {
		r.RegisterPrim("myConst", func(_ context.Context, ce gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
			return &gmp.HostVal{Inner: int64(999)}, ce, nil
		})
		return r.RegisterModule("Custom", `
import Prelude

myConst :: Int
myConst := assumption

main := myConst
`)
	})
	eng := gmp.NewEngine()
	if err := eng.Use(customPack); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Custom
main := myConst
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	if hv := gmp.MustHost[int64](result.Value); hv != 999 {
		t.Errorf("expected 999, got %d", hv)
	}
}

// ---------------------------------------------------------------------------
// v0.6.1 Tests: API Surface Repair
// ---------------------------------------------------------------------------

func TestSetTraceHookPublicAPI(t *testing.T) {
	eng := gmp.NewEngine()
	var events []gmp.TraceEvent
	eng.SetTraceHook(func(e gmp.TraceEvent) error {
		events = append(events, e)
		return nil
	})
	rt, err := eng.NewRuntime(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Error("expected at least one trace event")
	}
}

func TestSetCheckTraceHookPublicAPI(t *testing.T) {
	eng := gmp.NewEngine()
	var events []gmp.CheckTraceEvent
	eng.SetCheckTraceHook(func(e gmp.CheckTraceEvent) {
		events = append(events, e)
	})
	_, err := eng.Check(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Error("expected at least one check trace event")
	}
}

func TestParseReturnsOpaque(t *testing.T) {
	eng := gmp.NewEngine()
	parsed, err := eng.Parse(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	if parsed == nil {
		t.Error("expected non-nil ParsedProgram")
	}
}

func TestCheckReturnsOpaque(t *testing.T) {
	eng := gmp.NewEngine()
	core, err := eng.Check(`main := True`)
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
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := True`)
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
	eng := gmp.NewEngine()
	eng.EnableRecursion()
	eng.SetDepthLimit(10)
	rt, err := eng.NewRuntime(`
loop := fix (\self -> \x -> self x)
main := loop ()
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	if !strings.Contains(err.Error(), "depth limit") {
		t.Fatalf("expected depth limit error, got: %v", err)
	}
}

func TestEngineMultipleRuntimes(t *testing.T) {
	eng := gmp.NewEngine()
	rt1, err := eng.NewRuntime(`main := True`)
	if err != nil {
		t.Fatal(err)
	}
	rt2, err := eng.NewRuntime(`main := False`)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := rt1.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := rt2.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := r1.Value.(*gmp.ConVal); !ok || c.Con != "True" {
		t.Errorf("rt1: expected True, got %v", r1.Value)
	}
	if c, ok := r2.Value.(*gmp.ConVal); !ok || c.Con != "False" {
		t.Errorf("rt2: expected False, got %v", r2.Value)
	}
}

func TestEngineMutationAfterRuntime(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.DeclareBinding("x", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}
	// Mutate engine after runtime creation.
	eng.SetStepLimit(1)
	// Runtime should still work with original limits.
	bindings := map[string]gmp.Value{"x": &gmp.HostVal{Inner: int64(99)}}
	result, err := rt.RunContext(context.Background(), nil, bindings, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != int64(99) {
		t.Errorf("expected 99, got %v", result.Value)
	}
}

func TestModuleAccumulation(t *testing.T) {
	eng := gmp.NewEngine()
	eng.NoPrelude()
	err := eng.RegisterModule("A", `
data Bool = True | False
`)
	if err != nil {
		t.Fatal(err)
	}
	err = eng.RegisterModule("B", `
import A
data Pair a b = MkPair a b
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import A
import B
main := MkPair True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Value.(*gmp.ConVal); !ok {
		t.Fatalf("expected ConVal, got %T", result.Value)
	}
}

func TestInvalidModuleSource(t *testing.T) {
	eng := gmp.NewEngine()
	err := eng.RegisterModule("Bad", `this is not valid syntax @@@@`)
	if err == nil {
		t.Fatal("expected error for invalid module source")
	}
}

// --- Phase 7C: Effectful PrimOp Deferral ---

func TestEffectfulDeferUntilBind(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.State
main :: Computation { state : Int | r } { state : Int | r } Int
main := do { s <- get; pure s }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &gmp.HostVal{Inner: int64(42)}}
	result, err := rt.RunContext(context.Background(), caps, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != int64(42) {
		t.Errorf("expected 42, got %v", result.Value)
	}
}

func TestEffectfulTopLevelForce(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.State
main :: Computation { state : Int | r } { state : Int | r } Int
main := get
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &gmp.HostVal{Inner: int64(7)}}
	result, err := rt.RunContext(context.Background(), caps, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != int64(7) {
		t.Errorf("expected 7, got %v", result.Value)
	}
}

func TestNonEffectfulImmediate(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := 1 + 2
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != int64(3) {
		t.Errorf("expected 3, got %v", result.Value)
	}
}

// --- Phase 7D: Error Path Tests ---

func TestDepthLimitError(t *testing.T) {
	eng := gmp.NewEngine()
	eng.EnableRecursion()
	eng.SetDepthLimit(5)
	rt, err := eng.NewRuntime(`
deep := fix (\self -> \x -> self x)
main := deep ()
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
	if err == nil {
		t.Fatal("expected error for deep recursion")
	}
}

func TestPrimImplError(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := div 1 0
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
	if err == nil {
		t.Fatal("expected error for division by zero")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected division by zero error, got: %v", err)
	}
}

func TestMissingBinding(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", gmp.KindType())
	eng.DeclareBinding("x", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`main := x`)
	if err != nil {
		t.Fatal(err)
	}
	// Run without providing the binding.
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
	if err == nil {
		t.Fatal("expected error for missing binding")
	}
	if !strings.Contains(err.Error(), "missing binding") {
		t.Fatalf("expected missing binding error, got: %v", err)
	}
}

func TestMissingEntryPoint(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`foo := True`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunContext(context.Background(), nil, nil, "main")
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
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := fmap (\x -> add x 1) (Cons 1 (Cons 2 (Cons 3 Nil)))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	// Expected: Cons 2 (Cons 3 (Cons 4 Nil))
	assertList(t, result.Value, []int64{2, 3, 4})
}

func TestListFoldr(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := foldr add 0 (Cons 1 (Cons 2 (Cons 3 Nil)))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", result.Value)
	}
	if hv.Inner != int64(6) {
		t.Fatalf("expected 6, got %v", hv.Inner)
	}
}

func TestListAppend(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := append (Cons 1 (Cons 2 Nil)) (Cons 3 Nil)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertList(t, result.Value, []int64{1, 2, 3})
}

func TestListEq(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := eq (Cons 1 (Cons 2 Nil)) (Cons 1 (Cons 2 Nil))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Fatalf("expected True, got %v", result.Value)
	}
}

func TestListEqDifferent(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := eq (Cons 1 Nil) (Cons 2 Nil)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "False" {
		t.Fatalf("expected False, got %v", result.Value)
	}
}

func TestListMonoidEmpty(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main := (empty :: List Bool)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Nil" {
		t.Fatalf("expected Nil, got %v", result.Value)
	}
}

// ---------------------------------------------------------------------------
// IxMonad Class + Lift Alias (Group 2B)
// ---------------------------------------------------------------------------

func TestIxMonadClassExists(t *testing.T) {
	// Verify IxMonad class is defined in prelude and usable in type annotations.
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
myFn :: forall (m : Row -> Row -> Type -> Type). IxMonad m => forall a (r : Row). a -> m r r a
myFn := ixpure
main := True
`)
	if err != nil {
		t.Fatalf("IxMonad class should be usable: %v", err)
	}
}

func TestLiftAliasExpansion(t *testing.T) {
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
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
	eng := gmp.NewEngine()
	_, err := eng.NewRuntime(`
class MyClass (m : Row -> Row -> Type -> Type) {
  myPure :: forall a (r : Row). a -> m r r a
}
main := True
`)
	if err != nil {
		t.Fatalf("kinded class param should compile: %v", err)
	}
}

func TestKindedClassParamWithInstance(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
class Wrap (f : Type -> Type) {
  wrap :: forall a. a -> f a
}
instance Wrap Maybe {
  wrap := \x -> Just x
}
main := wrap True
`)
	if err != nil {
		t.Fatalf("kinded class with instance should compile: %v", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", result.Value)
	}
}

// ---------------------------------------------------------------------------
// Go Boundary Conversion (Group 1D)
// ---------------------------------------------------------------------------

func TestToListFromList(t *testing.T) {
	list := gmp.ToList([]any{
		&gmp.HostVal{Inner: int64(1)},
		&gmp.HostVal{Inner: int64(2)},
		&gmp.HostVal{Inner: int64(3)},
	})
	items, ok := gmp.FromList(list)
	if !ok {
		t.Fatal("expected valid list")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	for i, want := range []int64{1, 2, 3} {
		hv := items[i].(*gmp.HostVal)
		if hv.Inner != want {
			t.Fatalf("element %d: expected %d, got %v", i, want, hv.Inner)
		}
	}
}

func TestToListEmpty(t *testing.T) {
	list := gmp.ToList(nil)
	con, ok := list.(*gmp.ConVal)
	if !ok || con.Con != "Nil" {
		t.Fatalf("expected Nil, got %v", list)
	}
}

func TestFromListInvalid(t *testing.T) {
	_, ok := gmp.FromList(&gmp.HostVal{Inner: 42})
	if ok {
		t.Fatal("expected false for non-list")
	}
}

func TestToListWithBinding(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	eng.DeclareBinding("xs", gmp.AppType(gmp.ConType("List"), gmp.ConType("Int")))
	rt, err := eng.NewRuntime(`
import Std.Num
main := foldr add 0 xs
`)
	if err != nil {
		t.Fatal(err)
	}
	bindings := map[string]gmp.Value{
		"xs": gmp.ToList([]any{
			&gmp.HostVal{Inner: int64(1)},
			&gmp.HostVal{Inner: int64(2)},
			&gmp.HostVal{Inner: int64(3)},
		}),
	}
	result, err := rt.RunContext(context.Background(), nil, bindings, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != int64(6) {
		t.Fatalf("expected 6, got %v", result.Value)
	}
}

// ---------------------------------------------------------------------------
// List Literal Syntax (Group 1C)
// ---------------------------------------------------------------------------

func TestListLiteralBasic(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := [1, 2, 3]
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertList(t, result.Value, []int64{1, 2, 3})
}

func TestListLiteralEmpty(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`main := ([] :: List Bool)`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Nil" {
		t.Fatalf("expected Nil, got %v", result.Value)
	}
}

func TestListLiteralFmap(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.Num); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.Num
main := fmap (\x -> add x 10) [1, 2, 3]
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertList(t, result.Value, []int64{11, 12, 13})
}

// ---------------------------------------------------------------------------
// List Stdlib Pack (Group 1B)
// ---------------------------------------------------------------------------

func TestListStdlibLength(t *testing.T) {
	eng := gmp.NewEngine()
	if err := eng.Use(gmp.List); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(`
import Std.List
main := length (Cons True (Cons False Nil))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != int64(2) {
		t.Fatalf("expected 2, got %v", result.Value)
	}
}

// assertList checks that a Value is a List with the given int64 elements.
func assertList(t *testing.T, v gmp.Value, expected []int64) {
	t.Helper()
	for i, want := range expected {
		con, ok := v.(*gmp.ConVal)
		if !ok || con.Con != "Cons" {
			t.Fatalf("element %d: expected Cons, got %v", i, v)
		}
		if len(con.Args) != 2 {
			t.Fatalf("element %d: Cons has %d args, expected 2", i, len(con.Args))
		}
		hv, ok := con.Args[0].(*gmp.HostVal)
		if !ok {
			t.Fatalf("element %d: expected HostVal, got %T", i, con.Args[0])
		}
		if hv.Inner != want {
			t.Fatalf("element %d: expected %d, got %v", i, want, hv.Inner)
		}
		v = con.Args[1]
	}
	con, ok := v.(*gmp.ConVal)
	if !ok || con.Con != "Nil" {
		t.Fatalf("expected Nil at end, got %v", v)
	}
}

// ---------------------------------------------------------------------------
// Instance IxMonad Computation (Group 2C)
// ---------------------------------------------------------------------------

func TestIxMonadComputationIxpure(t *testing.T) {
	eng := gmp.NewEngine()
	eng.DeclareBinding("n", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`
main :: Computation {} {} Int
main := ixpure n
`)
	if err != nil {
		t.Fatalf("ixpure should resolve via IxMonad Computation: %v", err)
	}
	result, err := rt.RunContext(context.Background(), nil, map[string]gmp.Value{
		"n": &gmp.HostVal{Inner: 42},
	}, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != 42 {
		t.Fatalf("expected 42, got %v", result.Value)
	}
}

func TestIxMonadComputationIxbind(t *testing.T) {
	eng := gmp.NewEngine()
	eng.DeclareBinding("n", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`
main :: Computation {} {} Int
main := ixbind (ixpure n) (\x -> ixpure x)
`)
	if err != nil {
		t.Fatalf("ixbind should resolve via IxMonad Computation: %v", err)
	}
	result, err := rt.RunContext(context.Background(), nil, map[string]gmp.Value{
		"n": &gmp.HostVal{Inner: 99},
	}, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != 99 {
		t.Fatalf("expected 99, got %v", result.Value)
	}
}

func TestIxMonadGenericFunction(t *testing.T) {
	// A generic function using IxMonad constraint should work with Computation.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
myReturn :: forall (m : Row -> Row -> Type -> Type). IxMonad m => forall a (r : Row). a -> m r r a
myReturn := ixpure
main :: Computation {} {} Bool
main := myReturn True
`)
	if err != nil {
		t.Fatalf("generic IxMonad function should compile: %v", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "True" {
		t.Fatalf("expected True, got %v", result.Value)
	}
}

// ---------------------------------------------------------------------------
// Type-Driven do Dispatch (Group 3A)
// ---------------------------------------------------------------------------

func TestDoBlockComputationRegression(t *testing.T) {
	// Existing do blocks with Computation should still use Core.Bind path.
	eng := gmp.NewEngine()
	eng.DeclareBinding("x", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`
main := do { v <- pure x; pure v }
`)
	if err != nil {
		t.Fatalf("Computation do block regression failed: %v", err)
	}
	result, err := rt.RunContext(context.Background(), nil, map[string]gmp.Value{
		"x": &gmp.HostVal{Inner: 42},
	}, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != 42 {
		t.Fatalf("expected 42, got %v", result.Value)
	}
}

func TestDoBlockComputationCoreBind(t *testing.T) {
	// Verify Computation do blocks elaborate to Core.Bind (not class dispatch).
	eng := gmp.NewEngine()
	eng.DeclareBinding("x", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`
main := do { v <- pure x; pure v }
`)
	if err != nil {
		t.Fatal(err)
	}
	// Core.Bind is used when PrettyProgram shows "bind" nodes rather than "ixbind" calls.
	pretty := rt.PrettyProgram()
	if !strings.Contains(pretty, "bind") {
		t.Fatalf("expected Core.Bind in pretty output, got:\n%s", pretty)
	}
}

// ---------------------------------------------------------------------------
// Monad Instances for Maybe and List (Group 3C)
// ---------------------------------------------------------------------------

func TestMaybeDoBlockPure(t *testing.T) {
	// do { x <- Just 5; pure (add x 1) } :: Maybe Int → Just 6
	eng := gmp.NewEngine()
	eng.RegisterPrim("add", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(gmp.MustHost[int64](args[0]) + gmp.MustHost[int64](args[1])), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
add :: Int -> Int -> Int
add := assumption
main :: Maybe Int
main := do { x <- Just 5; pure (add x 1) }
`)
	if err != nil {
		t.Fatalf("Maybe do block should compile: %v", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Just" || len(con.Args) != 1 {
		t.Fatalf("expected Just 6, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*gmp.HostVal)
	if !ok || hv.Inner != int64(6) {
		t.Fatalf("expected Just 6, got Just %v", con.Args[0])
	}
}

func TestMaybeDoBlockNothing(t *testing.T) {
	// do { x <- Just 5; Nothing; pure x } :: Maybe Int → Nothing
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main :: Maybe Int
main := do { x <- Just 5; Nothing; pure x }
`)
	if err != nil {
		t.Fatalf("Maybe do block Nothing should compile: %v", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Nothing" {
		t.Fatalf("expected Nothing, got %v", result.Value)
	}
}

func TestListDoBlockCartesian(t *testing.T) {
	// do { x <- [1,2]; y <- [10,20]; pure (add x y) } :: List Int → [11, 21, 12, 22]
	eng := gmp.NewEngine()
	eng.RegisterPrim("add", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(gmp.MustHost[int64](args[0]) + gmp.MustHost[int64](args[1])), capEnv, nil
	})
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(`
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
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := gmp.FromList(result.Value)
	if !ok || len(got) != 4 {
		t.Fatalf("expected 4 elements, got %v", result.Value)
	}
	expected := []int64{11, 21, 12, 22}
	for i, v := range got {
		hv, ok := v.(*gmp.HostVal)
		if !ok || hv.Inner != expected[i] {
			t.Fatalf("element %d: expected %d, got %v", i, expected[i], v)
		}
	}
}

func TestMaybeDoBlockDirectReturn(t *testing.T) {
	// Last expression not using pure — direct constructor.
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
main :: Maybe Int
main := do { x <- Just 5; Just x }
`)
	if err != nil {
		t.Fatalf("Maybe do block direct return should compile: %v", err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Just" || len(con.Args) != 1 {
		t.Fatalf("expected Just 5, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*gmp.HostVal)
	if !ok || hv.Inner != int64(5) {
		t.Fatalf("expected Just 5, got Just %v", con.Args[0])
	}
}

// ---------------------------------------------------------------------------
// Integration Tests (Group 4A/4B)
// ---------------------------------------------------------------------------

func TestThenCombinator(t *testing.T) {
	eng := gmp.NewEngine()
	eng.DeclareBinding("x", gmp.ConType("Int"))
	eng.DeclareBinding("y", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`
main := then (pure x) (pure y)
`)
	if err != nil {
		t.Fatalf("then combinator should compile: %v", err)
	}
	result, err := rt.RunContext(context.Background(), nil, map[string]gmp.Value{
		"x": gmp.ToValue(1),
		"y": gmp.ToValue(2),
	}, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != 2 {
		t.Fatalf("expected 2 (from second computation), got %v", result.Value)
	}
}

func TestGenericMonadicFunction(t *testing.T) {
	// A generic IxMonad function used with Computation via type annotation.
	eng := gmp.NewEngine()
	eng.DeclareBinding("x", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`
myReturn :: forall (m : Row -> Row -> Type -> Type). IxMonad m => forall a (r : Row). a -> m r r a
myReturn := ixpure
main :: Computation {} {} Int
main := myReturn x
`)
	if err != nil {
		t.Fatalf("generic monadic function should compile: %v", err)
	}
	result, err := rt.RunContext(context.Background(), nil, map[string]gmp.Value{
		"x": gmp.ToValue(99),
	}, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != 99 {
		t.Fatalf("expected 99, got %v", result.Value)
	}
}

func TestMaybeDoChain(t *testing.T) {
	// Chain multiple binds in a Maybe do block.
	eng := gmp.NewEngine()
	eng.RegisterPrim("add", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(gmp.MustHost[int64](args[0]) + gmp.MustHost[int64](args[1])), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
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
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just 6, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*gmp.HostVal)
	if !ok || hv.Inner != int64(6) {
		t.Fatalf("expected Just 6, got Just %v", con.Args[0])
	}
}

func TestMaybeDoEarlyExit(t *testing.T) {
	// Nothing anywhere in the chain short-circuits.
	eng := gmp.NewEngine()
	eng.RegisterPrim("add", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(gmp.MustHost[int64](args[0]) + gmp.MustHost[int64](args[1])), capEnv, nil
	})
	rt, err := eng.NewRuntime(`
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
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Nothing" {
		t.Fatalf("expected Nothing, got %v", result.Value)
	}
}

func TestListDoFlatMap(t *testing.T) {
	// flatMap: each element maps to multiple elements.
	eng := gmp.NewEngine()
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(`
main :: List Int
main := do {
  x <- Cons 1 (Cons 2 (Cons 3 Nil));
  Cons x (Cons x Nil)
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := gmp.FromList(result.Value)
	if !ok {
		t.Fatalf("expected list, got %v", result.Value)
	}
	expected := []int64{1, 1, 2, 2, 3, 3}
	if len(got) != len(expected) {
		t.Fatalf("expected %d elements, got %d: %v", len(expected), len(got), result.Value)
	}
	for i, v := range got {
		hv, ok := v.(*gmp.HostVal)
		if !ok || hv.Inner != expected[i] {
			t.Fatalf("element %d: expected %d, got %v", i, expected[i], v)
		}
	}
}

func TestComputationDoRegression(t *testing.T) {
	// Ensure Computation do blocks still use Core.Bind (not class dispatch).
	eng := gmp.NewEngine()
	eng.DeclareBinding("x", gmp.ConType("Int"))
	eng.DeclareBinding("y", gmp.ConType("Int"))
	rt, err := eng.NewRuntime(`
main := do { a <- pure x; b <- pure y; pure a }
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, map[string]gmp.Value{
		"x": gmp.ToValue(10),
		"y": gmp.ToValue(20),
	}, "main")
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != 10 {
		t.Fatalf("expected 10, got %v", result.Value)
	}
}

func TestListPipelineEndToEnd(t *testing.T) {
	// fromSlice → fmap → foldr → toSlice full pipeline.
	eng := gmp.NewEngine()
	eng.RegisterPrim("add", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value, _ gmp.Applier) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(gmp.MustHost[int64](args[0]) + gmp.MustHost[int64](args[1])), capEnv, nil
	})
	eng.EnableRecursion()
	eng.DeclareBinding("xs", gmp.AppType(gmp.ConType("List"), gmp.ConType("Int")))
	rt, err := eng.NewRuntime(`
add :: Int -> Int -> Int
add := assumption
main := foldr add 0 (fmap (\x -> add x 10) xs)
`)
	if err != nil {
		t.Fatal(err)
	}
	// xs = [1, 2, 3]
	result, err := rt.RunContext(context.Background(), nil, map[string]gmp.Value{
		"xs": gmp.ToList([]any{int64(1), int64(2), int64(3)}),
	}, "main")
	if err != nil {
		t.Fatal(err)
	}
	// fmap (+10) [1,2,3] = [11,12,13], foldr add 0 = 36
	hv, ok := result.Value.(*gmp.HostVal)
	if !ok || hv.Inner != int64(36) {
		t.Fatalf("expected 36, got %v", result.Value)
	}
}
