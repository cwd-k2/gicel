// VM debug tests — bytecode dump helpers and Applier callback correctness.
// Does NOT cover: general execution (vm_test.go), compiler (compiler_test.go).
package vm

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// TestVMFoldlFst reproduces the fibonacci example failure:
// myFst (foldl (\a _. a) (1, 2) [()])
// foldl is a PrimVal (_listFoldl) that uses the Applier callback.
func TestVMFoldlFst(t *testing.T) {
	// Set up a minimal foldl primitive.
	prims := eval.NewPrimRegistry()
	prims.Register("_listFoldl", func(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
		// args: [f, z, list]
		f, z := args[0], args[1]
		list := args[2]
		acc := z
		for {
			cv, ok := list.(*eval.ConVal)
			if !ok || cv.Con == "Nil" {
				return acc, ce, nil
			}
			if cv.Con != "Cons" || len(cv.Args) != 2 {
				return acc, ce, nil
			}
			// acc = f acc x
			partial, ce2, err := apply.Apply(f, acc, ce)
			if err != nil {
				return nil, ce, err
			}
			acc, ce, err = apply.Apply(partial, cv.Args[0], ce2)
			if err != nil {
				return nil, ce, err
			}
			list = cv.Args[1]
		}
	})

	globals := map[ir.VarKey]int{
		ir.LocalKey("foldl"): 0,
		ir.LocalKey("myFst"): 1,
	}
	// Build foldl PrimVal.
	foldlVal := &eval.PrimVal{Name: "_listFoldl", Arity: 3, IsEffectful: false}
	// Build myFst as VMClosure.
	myFstBody := &ir.RecordProj{Record: &ir.Var{Name: "p", Index: 0}, Label: "_1"}
	myFstLam := &ir.Lam{Param: "p", Body: myFstBody}
	c := NewCompiler(globals, nil)
	annotate(c, myFstLam)
	myFstProto := c.CompileBinding(ir.Binding{Name: "myFst", Expr: myFstLam})

	b := budget.New(context.Background(), 100000, 1000)
	b.SetAllocLimit(100 * 1024 * 1024)
	globalArray := []eval.Value{foldlVal, nil}

	machine := NewVM(VMConfig{
		Globals:     globalArray,
		GlobalSlots: globals,
		Prims:       prims,
		Budget:      b,
		Ctx:         context.Background(),
	})

	// Run myFst proto to get VMClosure.
	myFstResult, err := machine.Run(myFstProto, eval.EmptyCapEnv())
	if err != nil {
		t.Fatalf("myFst compile: %v", err)
	}
	globalArray[1] = myFstResult.Value

	// Build: myFst (foldl (\a _. a) (1, 2) [()])
	idLam := &ir.Lam{Param: "a", Body: &ir.Lam{Param: "_", Body: &ir.Var{Name: "a", Index: 1}}}
	tuple12 := &ir.RecordLit{Fields: []ir.Field{
		{Label: "_1", Value: &ir.Lit{Value: int64(1)}},
		{Label: "_2", Value: &ir.Lit{Value: int64(2)}},
	}}
	unitList := &ir.Con{Name: "Cons", Args: []ir.Core{
		&ir.RecordLit{},
		&ir.Con{Name: "Nil"},
	}}
	foldlApp := &ir.App{Fun: &ir.App{Fun: &ir.App{
		Fun: &ir.Var{Name: "foldl", Index: -1},
		Arg: idLam,
	}, Arg: tuple12}, Arg: unitList}
	fullExpr := &ir.App{
		Fun: &ir.Var{Name: "myFst", Index: -1},
		Arg: foldlApp,
	}

	annotate(c, fullExpr)
	entryProto := c.CompileExpr(fullExpr)
	result, err := machine.Run(entryProto, eval.EmptyCapEnv())
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	assertHostVal(t, result.Value, int64(1))
}
