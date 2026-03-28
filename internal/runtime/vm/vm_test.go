// VM execution tests — compile Core IR and execute bytecode.
// Does NOT cover: compiler unit tests (compiler_test.go).
package vm

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

func TestVMRunLit(t *testing.T) {
	result := runExpr(t, &ir.Lit{Value: int64(42)}, nil, nil)
	assertHostVal(t, result, int64(42))
}

func TestVMRunConst(t *testing.T) {
	result := runExpr(t, &ir.Lit{Value: "hello"}, nil, nil)
	assertHostVal(t, result, "hello")
}

func TestVMRunGlobalVar(t *testing.T) {
	globals := map[string]int{"x": 0}
	globalArray := []eval.Value{&eval.HostVal{Inner: int64(99)}}
	result := runExprWithGlobals(t, &ir.Var{Name: "x", Index: -1, Key: "x"}, globals, globalArray)
	assertHostVal(t, result, int64(99))
}

func TestVMRunLamApp(t *testing.T) {
	// (\x. x) 42
	lam := &ir.Lam{Param: "x", Body: &ir.Var{Name: "x", Index: 0}}
	app := &ir.App{Fun: lam, Arg: &ir.Lit{Value: int64(42)}}
	result := runExpr(t, app, nil, nil)
	assertHostVal(t, result, int64(42))
}

func TestVMRunConVal(t *testing.T) {
	// Just 42
	expr := &ir.Con{Name: "Just", Args: []ir.Core{&ir.Lit{Value: int64(42)}}}
	result := runExpr(t, expr, nil, nil)
	cv, ok := result.(*eval.ConVal)
	if !ok {
		t.Fatalf("expected ConVal, got %T", result)
	}
	if cv.Con != "Just" || len(cv.Args) != 1 {
		t.Fatalf("expected Just 42, got %s", cv)
	}
}

func TestVMRunConValCurried(t *testing.T) {
	// Cons applied to two args via partial application
	// Con Cons with 0 args, then apply 1, then apply 2
	con0 := &ir.Con{Name: "Cons", Args: nil}
	app1 := &ir.App{Fun: con0, Arg: &ir.Lit{Value: int64(1)}}
	nilCon := &ir.Con{Name: "Nil", Args: nil}
	app2 := &ir.App{Fun: app1, Arg: nilCon}
	result := runExpr(t, app2, nil, nil)
	cv, ok := result.(*eval.ConVal)
	if !ok {
		t.Fatalf("expected ConVal, got %T", result)
	}
	if cv.Con != "Cons" || len(cv.Args) != 2 {
		t.Fatalf("expected Cons with 2 args, got %s", cv)
	}
}

func TestVMRunCaseSimple(t *testing.T) {
	// case True of { True => 1; False => 0 }
	scrut := &ir.Con{Name: "True", Args: nil}
	cs := &ir.Case{
		Scrutinee: scrut,
		Alts: []ir.Alt{
			{Pattern: &ir.PCon{Con: "True"}, Body: &ir.Lit{Value: int64(1)}},
			{Pattern: &ir.PCon{Con: "False"}, Body: &ir.Lit{Value: int64(0)}},
		},
	}
	result := runExpr(t, cs, nil, nil)
	assertHostVal(t, result, int64(1))
}

func TestVMRunCaseSecondAlt(t *testing.T) {
	scrut := &ir.Con{Name: "False", Args: nil}
	cs := &ir.Case{
		Scrutinee: scrut,
		Alts: []ir.Alt{
			{Pattern: &ir.PCon{Con: "True"}, Body: &ir.Lit{Value: int64(1)}},
			{Pattern: &ir.PCon{Con: "False"}, Body: &ir.Lit{Value: int64(0)}},
		},
	}
	result := runExpr(t, cs, nil, nil)
	assertHostVal(t, result, int64(0))
}

func TestVMRunCaseWithBinding(t *testing.T) {
	// case Just 42 of { Just x => x; Nothing => 0 }
	scrut := &ir.Con{Name: "Just", Args: []ir.Core{&ir.Lit{Value: int64(42)}}}
	cs := &ir.Case{
		Scrutinee: scrut,
		Alts: []ir.Alt{
			{
				Pattern: &ir.PCon{Con: "Just", Args: []ir.Pattern{&ir.PVar{Name: "x"}}},
				Body:    &ir.Var{Name: "x", Index: 0},
			},
			{
				Pattern: &ir.PCon{Con: "Nothing"},
				Body:    &ir.Lit{Value: int64(0)},
			},
		},
	}
	result := runExpr(t, cs, nil, nil)
	assertHostVal(t, result, int64(42))
}

func TestVMRunCaseWildcard(t *testing.T) {
	scrut := &ir.Lit{Value: int64(99)}
	cs := &ir.Case{
		Scrutinee: scrut,
		Alts: []ir.Alt{
			{Pattern: &ir.PWild{}, Body: &ir.Lit{Value: int64(1)}},
		},
	}
	result := runExpr(t, cs, nil, nil)
	assertHostVal(t, result, int64(1))
}

func TestVMRunBind(t *testing.T) {
	// x <- pure 10; pure x
	comp := &ir.Pure{Expr: &ir.Lit{Value: int64(10)}}
	body := &ir.Pure{Expr: &ir.Var{Name: "x", Index: 0}}
	bind := &ir.Bind{Comp: comp, Var: "x", Body: body}
	result := runExpr(t, bind, nil, nil)
	assertHostVal(t, result, int64(10))
}

func TestVMRunThunkForce(t *testing.T) {
	// force (thunk (pure 7))
	thunk := &ir.Thunk{Comp: &ir.Pure{Expr: &ir.Lit{Value: int64(7)}}}
	force := &ir.Force{Expr: thunk}
	result := runExpr(t, force, nil, nil)
	assertHostVal(t, result, int64(7))
}

func TestVMRunRecordLit(t *testing.T) {
	expr := &ir.RecordLit{
		Fields: []ir.RecordField{
			{Label: "x", Value: &ir.Lit{Value: int64(1)}},
			{Label: "y", Value: &ir.Lit{Value: int64(2)}},
		},
	}
	result := runExpr(t, expr, nil, nil)
	rv, ok := result.(*eval.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T", result)
	}
	v, ok := rv.Get("x")
	if !ok {
		t.Fatal("missing field x")
	}
	assertHostVal(t, v, int64(1))
}

func TestVMRunRecordProj(t *testing.T) {
	rec := &ir.RecordLit{
		Fields: []ir.RecordField{
			{Label: "a", Value: &ir.Lit{Value: int64(10)}},
			{Label: "b", Value: &ir.Lit{Value: int64(20)}},
		},
	}
	expr := &ir.RecordProj{Record: rec, Label: "b"}
	result := runExpr(t, expr, nil, nil)
	assertHostVal(t, result, int64(20))
}

func TestVMRunNestedApp(t *testing.T) {
	// (\f. \x. f x) (\y. y) 42
	inner := &ir.Lam{Param: "y", Body: &ir.Var{Name: "y", Index: 0}}
	outer := &ir.Lam{
		Param: "f",
		Body: &ir.Lam{
			Param: "x",
			Body:  &ir.App{Fun: &ir.Var{Name: "f", Index: 1}, Arg: &ir.Var{Name: "x", Index: 0}},
		},
	}
	app1 := &ir.App{Fun: outer, Arg: inner}
	app2 := &ir.App{Fun: app1, Arg: &ir.Lit{Value: int64(42)}}
	result := runExpr(t, app2, nil, nil)
	assertHostVal(t, result, int64(42))
}

func TestVMRunPrim(t *testing.T) {
	// _add 1 2
	prims := eval.NewPrimRegistry()
	prims.Register("_add", func(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
		a := args[0].(*eval.HostVal).Inner.(int64)
		b := args[1].(*eval.HostVal).Inner.(int64)
		return &eval.HostVal{Inner: a + b}, ce, nil
	})
	expr := &ir.PrimOp{
		Name: "_add", Arity: 2,
		Args: []ir.Core{&ir.Lit{Value: int64(1)}, &ir.Lit{Value: int64(2)}},
	}
	result := runExpr(t, expr, prims, nil)
	assertHostVal(t, result, int64(3))
}

func TestVMRunTyAppErased(t *testing.T) {
	inner := &ir.Lit{Value: int64(42)}
	expr := &ir.TyApp{Expr: inner}
	result := runExpr(t, expr, nil, nil)
	assertHostVal(t, result, int64(42))
}

func TestVMRunEmptyRecord(t *testing.T) {
	expr := &ir.RecordLit{}
	result := runExpr(t, expr, nil, nil)
	if result != eval.UnitVal {
		t.Fatalf("expected UnitVal, got %T: %s", result, result)
	}
}

// --- helpers ---

func runExpr(t *testing.T, expr ir.Core, prims *eval.PrimRegistry, globals map[string]int) eval.Value {
	t.Helper()
	return runExprFull(t, expr, prims, globals, nil)
}

func runExprWithGlobals(t *testing.T, expr ir.Core, globalSlots map[string]int, globalArray []eval.Value) eval.Value {
	t.Helper()
	return runExprFull(t, expr, nil, globalSlots, globalArray)
}

func runExprFull(t *testing.T, expr ir.Core, prims *eval.PrimRegistry, globalSlots map[string]int, globalArray []eval.Value) eval.Value {
	t.Helper()
	if globalSlots == nil {
		globalSlots = map[string]int{}
	}
	if prims == nil {
		prims = eval.NewPrimRegistry()
	}
	// Run the same annotation passes the pipeline uses before compilation.
	ir.AnnotateFreeVars(expr)
	ir.AssignIndices(expr)
	c := NewCompiler(globalSlots, nil)
	proto := c.CompileExpr(expr)
	b := budget.New(context.Background(), 100000, 1000)
	b.SetNestingLimit(512)
	b.SetAllocLimit(100 * 1024 * 1024)
	if globalArray == nil {
		globalArray = make([]eval.Value, len(globalSlots))
	}
	vm := NewVM(VMConfig{
		Globals:     globalArray,
		GlobalSlots: globalSlots,
		Prims:       prims,
		Budget:      b,
		Ctx:         context.Background(),
	})
	result, err := vm.Run(proto, eval.EmptyCapEnv())
	if err != nil {
		t.Fatalf("VM execution error: %v", err)
	}
	return result.Value
}

func assertHostVal(t *testing.T, v eval.Value, expected any) {
	t.Helper()
	hv, ok := v.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %s", v, v)
	}
	if hv.Inner != expected {
		t.Fatalf("expected %v, got %v", expected, hv.Inner)
	}
}
