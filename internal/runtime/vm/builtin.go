package vm

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// CompileBuiltinGlobals compiles the built-in globals (pure, bind, and
// optionally fix/rec) into VMClosure values. `thunk` and `force` are
// deliberately absent: they are pure syntactic special forms without
// first-class runtime representation. Applied uses elaborate directly
// to ir.Thunk / ir.Force, and indirect uses are covered by the CBPV
// auto-coercion in the type checker.
func CompileBuiltinGlobals(compiler *Compiler, enableFix, enableRec bool) map[string]eval.Value {
	globals := make(map[string]eval.Value, 8)

	globals["pure"] = compileBuiltinLam(compiler, "pure", eval.PureBody())
	globals["bind"] = compileBuiltinLam(compiler, "bind", eval.BindBody())

	if enableFix {
		globals["fix"] = compileBuiltinLam(compiler, "fix",
			&ir.Lam{Param: "_f", Body: eval.FixBody()})
	}
	if enableRec {
		globals["rec"] = compileBuiltinLam(compiler, "rec",
			&ir.Lam{Param: "_f", Body: eval.RecBody()})
	}

	return globals
}

// compileBuiltinLam compiles a Lam IR expression into a VMClosure.
func compileBuiltinLam(compiler *Compiler, name string, lam *ir.Lam) eval.Value {
	annots := ir.AnnotateFreeVars(lam)
	ir.AssignIndices(lam, annots)
	compiler.SetFVAnnots(annots)
	proto := compiler.CompileBinding(ir.Binding{Name: name, Expr: lam})
	if len(proto.Protos) == 1 {
		childProto := proto.Protos[0]
		return &eval.VMClosure{
			Captured: nil,
			Proto:    childProto,
			Name:     name,
		}
	}
	panic(fmt.Sprintf("vm/builtin: expected 1 nested proto for %s, got %d", name, len(proto.Protos)))
}
