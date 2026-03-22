// Engine host API tests — Engine config, diagnostics, runtime reuse, multiple runtimes, packs.
// Does NOT cover: value conversion (convert_test.go), boundary errors (engine_boundary_test.go).

package engine

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

func TestHostBinding(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.DeclareBinding("x", ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `main := x`)
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

func TestEngineUse(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	called := false
	pack := registry.Pack(func(r registry.Registrar) error {
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

func TestNoPrelude(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
data MyBool := { MyTrue: MyBool; MyFalse: MyBool; }
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
	_, err := eng.NewRuntime(context.Background(), `main := undefined_thing`)
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
	rt, err := eng.NewRuntime(context.Background(), `
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

func TestStepLimit(t *testing.T) {
	eng := NewEngine()
	eng.SetStepLimit(100)
	eng.EnableRecursion()
	// No recursion in source, just a program that completes in few steps.
	rt, err := eng.NewRuntime(context.Background(), `
data Unit := { Unit: Unit; }
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
	rt, err := eng.NewRuntime(context.Background(), `
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

func TestRuntimeReuse(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.DeclareBinding("x", ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `main := x`)
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

// Regression: NewRuntime must snapshot the primitive reg.
// Post-compilation RegisterPrim on the Engine must not affect existing Runtimes.
func TestRuntimePrimRegistryIsolation(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.DeclareAssumption("f", ArrowType(ConType("Bool"), ConType("Bool")))
	eng.RegisterPrim("f", func(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		return &eval.ConVal{Con: "True"}, ce, nil
	})

	rt, err := eng.NewRuntime(context.Background(), "import Prelude\nf := assumption\nmain := f False")
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
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
not := \b. case b { True => False; False => True }
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

func TestDiagnostics(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.NewRuntime(context.Background(), `main := undefined_xyz`)
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
	prog, err := eng.Compile(context.Background(), `
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
	rt, err := eng.NewRuntime(context.Background(), `
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
	// CapEnv should be present (even if empty).
	_ = result.CapEnv
	_ = result.Stats
}

// Assumption without any type annotation must produce a compile error.
func TestAssumptionNoType(t *testing.T) {
	eng := NewEngine()
	_, err := eng.NewRuntime(context.Background(), `
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

// Missing runtime binding must produce a runtime error (not panic).
func TestMissingRuntimeBinding(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.DeclareBinding("x", ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `main := x`)
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
	rt, err := eng.NewRuntime(context.Background(), `
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
	_, err := eng.NewRuntime(context.Background(), `
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

func TestEngineMultipleRuntimes(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt1, err := eng.NewRuntime(context.Background(), `
import Prelude
main := True
`)
	if err != nil {
		t.Fatal(err)
	}
	rt2, err := eng.NewRuntime(context.Background(), `
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
	rt, err := eng.NewRuntime(context.Background(), `main := x`)
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

func TestCustomPack(t *testing.T) {
	// A user-defined Pack that provides a custom constant.
	customPack := registry.Pack(func(r registry.Registrar) error {
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
	rt, err := eng.NewRuntime(context.Background(), `
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

func TestSetDepthLimit(t *testing.T) {
	// With TCO, tail-recursive loops keep depth flat (depth oscillates 0-1).
	// The step limit is what prevents infinite tail recursion now.
	// Verify that an infinite tail-recursive loop hits the step limit.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(1000)
	eng.SetDepthLimit(10000) // high enough to not interfere
	rt, err := eng.NewRuntime(context.Background(), `
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
