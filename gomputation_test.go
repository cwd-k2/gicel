package gomputation_test

import (
	"context"
	"testing"

	gmp "github.com/cwd-k2/gomputation"
	"github.com/cwd-k2/gomputation/internal/eval"
	"github.com/cwd-k2/gomputation/pkg/types"
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
	rt, err := eng.NewRuntime(`main := pure Unit`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Unit" {
		t.Errorf("expected Unit, got %s", result.Value)
	}
}

func TestHostBinding(t *testing.T) {
	eng := gmp.NewEngine()
	eng.RegisterType("Int", types.KType{})
	eng.DeclareBinding("x", types.Con("Int"))
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
	eng.DeclareAssumption("getUnit", types.MkArrow(types.Con("Unit"), types.Con("Unit")))
	eng.RegisterPrim("getUnit", func(ctx context.Context, capEnv gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		return &eval.ConVal{Con: "Unit"}, capEnv, nil
	})
	rt, err := eng.NewRuntime(`
getUnit := assumption
main := getUnit Unit
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Unit" {
		t.Errorf("expected Unit, got %s", result.Value)
	}
}

func TestCaseExpression(t *testing.T) {
	eng := gmp.NewEngine()
	rt, err := eng.NewRuntime(`
not := \b -> case b of { True -> False; False -> True }
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
	rt, err := eng.NewRuntime(`main := do { pure Unit }`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Unit" {
		t.Errorf("expected Unit, got %s", result.Value)
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
main := Pair True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gmp.ConVal)
	if !ok || con.Con != "Pair" {
		t.Errorf("expected Pair, got %s", result.Value)
	}
	if len(con.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(con.Args))
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
